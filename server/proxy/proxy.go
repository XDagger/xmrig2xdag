package proxy

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"hash/crc32"
	"math"
	"math/rand"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/swordlet/xmrig2xdag/config"
	"github.com/swordlet/xmrig2xdag/logger"
	"github.com/swordlet/xmrig2xdag/stratum"
	"github.com/swordlet/xmrig2xdag/xdag"
	"golang.org/x/net/proxy"
)

const (
	// MaxUint protects IDs from overflow if the process runs for thousands of years
	MaxUint = ^uint64(0)

	// TODO adjust - lower means more connections to pool, potentially fewer stales if that is a problem
	//maxProxyWorkers = 1024

	retryDelay = 5 * time.Second

	timeout = 10 * time.Second

	xdagAlgo = `rx/xdag`

	initDiffCount = 16

	//refreshDiffCount uint64 = 128 // count of shares to refresh target
	refreshDiffInterval = 10 * time.Minute

	submitInterval = 5

	isCrypto = true

	eofLimit = 3

	detectProxy = "XDAG_POOL_RESTART_DETECT_PROXY"
)

var poolIsDown atomic.Uint64
var eofCount atomic.Uint64

var crc32table = crc32.MakeTable(0xEDB88320)

var (
	ErrBadJobID       = errors.New("invalid job id")
	ErrDuplicateShare = errors.New("duplicate share")
	ErrMalformedShare = errors.New("malformed share")
)

// Worker does the work for the proxy.  It exposes methods that allow interface with the proxy.
type Worker interface {
	// ID is used to distinguish this worker from other workers on the proxy.
	ID() uint64
	// SetID allows proxies to assign this value when a connection is established.
	SetID(uint64)

	//SetProxy Workers must implement this method to establish communication with their assigned
	// proxy.  The proxy connection should be stored in order to 1. Submit Shares and 2. Disconnect Cleanly
	SetProxy(*Proxy)
	Proxy() *Proxy

	// Disconnect closes the connection to the proxy from the worker.
	// Ideally it sets up the worker to try and reconnect to a new proxy through the director.
	Disconnect()

	Close()

	NewJob(*Job)

	RemoveProxy()
}

// Proxy manages a group of workers.
type Proxy struct {
	ID       uint64
	Conn     *xdag.Connection
	SS       *stratum.Server
	director *Director

	//authID     string // identifies the proxy to the pool
	aliveSince time.Time
	shares     uint64

	//workerCount int
	worker                Worker
	submissions           chan *share
	notify                chan []byte
	done                  chan int
	ready                 bool
	currentJob            *Job
	PrevJobID             string
	BeforePrevJobID       string
	BeforeBeforePrevJobID string
	jobMu                 sync.Mutex
	connMu                sync.RWMutex
	isClosed              bool

	addressHash [xdag.HashLength]byte
	address     string

	fieldOut uint64
	fieldIn  uint64

	recvCount int
	recvByte  [2 * xdag.HashLength]byte

	target      string
	targetSince time.Time
	lastSend    time.Time
	miniResult  uint64
	miniNonce   uint32
	targetShare uint64
}

func init() {
	rand.Seed(time.Now().UnixNano())
	if isCrypto {
		ok := xdag.InitCrypto()
		if ok != 0 {
			panic(errors.New("initialize crypto error"))
		}
	}
}

// NewProxy creates a new proxy, starts the work thread, and returns a pointer to it.
func NewProxy(id uint64) *Proxy {
	p := &Proxy{
		ID:          id,
		aliveSince:  time.Now(),
		currentJob:  &Job{},
		PrevJobID:   NewLen(28),
		submissions: make(chan *share),
		done:        make(chan int),
		ready:       true,
		lastSend:    time.Now(),
		miniResult:  math.MaxUint64,
		notify:      make(chan []byte, 2),
	}

	p.SS = stratum.NewServer()
	p.SS.RegisterName("mining", &Mining{})
	return p
}

func (p *Proxy) Run(minerName string) {
	var retryCount = 0
	for {
		if poolIsDown.Load() > 0 && minerName != detectProxy {
			p.shutdown(2)
			return
		}
		err := p.connect(minerName)
		if err == nil {
			break
		}
		retryCount += 1
		if retryCount > 3 {
			if minerName != detectProxy {
				p.shutdown(2)
			} else {
				p.shutdown(-1)
			}
			return
		}
		logger.Get().Printf("Proxy[%d] Failed to acquire pool connection %d times.  Retrying in %s.Error: %s\n",
			p.ID, retryCount, retryDelay, err)
		// TODO allow fallback pools here
		<-time.After(retryDelay)
	}

	for {
		if minerName == detectProxy && poolIsDown.Load() == 0 { // pool restart , quit detect proxy
			return
		}
		select {
		// these are from workers
		case s := <-p.submissions:
			err := p.handleSubmit(s) //, p.SC)
			if err != nil {
				logger.Get().Println("share submission error: ", err)
			}
			if err != nil && strings.Contains(strings.ToLower(err.Error()), "banned") {
				logger.Get().Println("Banned IP - killing proxy: ", p.ID)
				p.shutdown(-1)
				return
			}
		case notif := <-p.notify:
			p.handleNotification(notif)
		case cl := <-p.done:
			p.shutdown(cl)
			return
		}
	}

}

func (p *Proxy) handleJob(job *Job) (err error) {
	p.jobMu.Lock()
	//p.prevJob, p.currentJob = p.currentJob, job
	p.BeforeBeforePrevJobID = p.BeforePrevJobID
	p.BeforePrevJobID = p.PrevJobID
	p.PrevJobID = p.currentJob.ID
	p.currentJob = job

	p.miniResult = math.MaxUint64
	p.lastSend = time.Now()

	p.jobMu.Unlock()

	//if err != nil {
	//	// logger.Get().Debugln("Skipping regular job broadcast: ", err)
	//	return
	//}

	// logger.Get().Debugln("Broadcasting new regular job: ", job.ID)
	p.broadcastJob()
	return
}

// broadcast a job to all workers
func (p *Proxy) broadcastJob() {
	logger.Get().Debugln("Broadcasting new job to connected workers.")
	//for _, w := range p.workers {
	//	go w.NewJob(p.NextJob())
	//}
	p.worker.NewJob(p.currentJob)
}

func (p *Proxy) handleNotification(notif []byte) {
	var data [32]byte
	copy(data[:], notif[:])
	if isCrypto {
		ptr := unsafe.Pointer(&data[0])
		xdag.DecryptField(ptr, p.fieldIn)
		p.fieldIn += 1
	}

	//xdag.DecryptField(p.crypt, unsafe.Add(ptr, uintptr(32)), p.fieldIn)
	//p.fieldIn += 1
	if xdag.Hash2address(data[:]) == p.address { // ignore 32 bytes: address with balance
		p.recvCount = 0
	} else if p.recvCount == 0 { // ignore the balance of fake block and the seed ,both ended with 8 bytes of zero
		seedZero := binary.BigEndian.Uint64(data[24:])
		if seedZero == 0 {
			return
		}
		p.recvCount++
		copy(p.recvByte[:32], data[:])
	} else if p.recvCount == 1 {
		p.recvCount = 0
		if p.address == detectProxy { // pool restart, close detect proxy, restore miners connection
			poolIsDown.Store(0)
			p.shutdown(-1)
			return
		}
		copy(p.recvByte[32:], data[:])

		if p.shares > initDiffCount+1 && time.Since(p.targetSince) >= refreshDiffInterval {
			p.setTarget(p.shares)
		}

		job := p.CreateJob(p.recvByte[:])
		if p.currentJob.Blob == "" || p.currentJob.Blob[:32] != job.Blob[:32] {
			err := p.handleJob(job)

			if err != nil {
				// log and wait for the next job?
				logger.Get().Println("error processing job: ", job)
				logger.Get().Println(err)
			}
		}

	}

}

func (p *Proxy) connect(minerName string) error {

	var conn net.Conn
	var socks5Dialer proxy.Dialer
	var err error

	if len(config.Get().Socks5) > 11 {
		socks5Dialer, err = proxy.SOCKS5("tcp", config.Get().Socks5, nil, proxy.Direct)
		if err != nil {
			return err
		}
		conn, err = socks5Dialer.Dial("tcp", config.Get().PoolAddr)
	} else {
		conn, err = net.DialTimeout("tcp", config.Get().PoolAddr, timeout)
	}

	if err != nil {
		return err
	}

	logger.Get().Debugln("Client made pool connection.")
	//p.SC = sc

	p.Conn = xdag.NewConnection(conn, p.ID, p.notify, p.done)
	p.Conn.Start()

	block := xdag.GenerateFakeBlock()
	binary.LittleEndian.PutUint64(block[0:8], xdag.BLOCK_HEADER_WORD)
	crc := crc32.Checksum(block[:], crc32table)
	newHeader := xdag.BLOCK_HEADER_WORD | (uint64(crc))<<32
	binary.LittleEndian.PutUint64(block[0:8], newHeader)
	if isCrypto {
		ptr := unsafe.Pointer(&block[0])
		for i := 0; i < xdag.XDAG_BLOCK_FIELDS; i++ {
			xdag.EncryptField(unsafe.Add(ptr, uintptr(i)*xdag.FieldSize), p.fieldOut)
			p.fieldOut += 1
		}
	}

	var bytesWithHeader [516]byte
	binary.LittleEndian.PutUint32(bytesWithHeader[0:4], 512)
	copy(bytesWithHeader[4:], block[:])
	p.Conn.SendBuffMsg(bytesWithHeader[:])

	time.Sleep(2 * time.Second)
	if minerName != "" {
		var field [32]byte
		binary.LittleEndian.PutUint32(field[0:4], xdag.WORKERNAME_HEADER_WORD)
		copy(field[4:32], minerName[:])
		if isCrypto {
			xdag.EncryptField(unsafe.Pointer(&field[0]), p.fieldOut)
			p.fieldOut += 1
		}

		var nameWithHeader [36]byte
		binary.LittleEndian.PutUint32(nameWithHeader[0:4], 32)
		copy(nameWithHeader[4:], field[:])
		p.Conn.SendBuffMsg(nameWithHeader[:])
	}

	logger.Get().Debugln(p.address, "--Successfully logged into pool.")

	logger.Get().Printf("****    Proxy [%d] Connected to pool server: %s \n", p.ID, config.Get().PoolAddr)
	//logger.Get().Println("****    Broadcasting jobs to workers.")

	return nil
}

func (p *Proxy) validateShare(s *share) error {
	var job *Job
	switch {
	case s.JobID == p.currentJob.ID:
		job = p.currentJob
	case s.JobID == p.PrevJobID:
		job = p.currentJob
	default:
		return ErrBadJobID
	}
	// for _, n := range job.submittedNonces {
	// 	if n == s.Nonce {
	// 		return ErrDuplicateShare
	// 	}
	// }
	return s.validate(job)
}

// proxy disconnected, cl:0 by worker, cl:1 by pool , cl:-1 byself
func (p *Proxy) shutdown(cl int) {
	p.connMu.Lock()
	defer p.connMu.Unlock()

	if p.isClosed {
		return
	}

	if cl == 0 {
		logger.Get().Printf("proxy [%d] shutdown by worker <%s>\n", p.ID, p.address)
		if p.Conn != nil {
			p.Conn.Close()
		}
	} else if cl == 1 {
		logger.Get().Printf("proxy [%d] shutdown by pool <%s>\n", p.ID, p.address)
		if p.worker != nil {
			p.worker.Close()
		}
		if p.fieldIn == 0 && p.fieldOut < 18 && p.Conn.EOFcount.Load() > 0 {
			eofCount.Add(1)
			if eofCount.Load() > eofLimit { // connection eof immediately after connect
				poolIsDown.Add(1)
				logger.Get().Println("*** Pool is down. Please wait for pool recovery.")
			}
		}
	} else if cl == 2 {
		poolIsDown.Add(1)
		logger.Get().Printf("proxy [%d] pool is down <%s>\n", p.ID, p.address)
		logger.Get().Println("*** Pool is down. Please wait for pool recovery.")
		if p.worker != nil {
			p.worker.Close()
		}
	} else if cl == -1 {
		logger.Get().Printf("proxy [%d] shutdown, <%s>\n", p.ID, p.address)
		if p.Conn != nil {
			p.Conn.Close()
		}
		if p.worker != nil {
			p.worker.Close()
		}
	}

	if p.ID > 0 {
		close(p.done)
		p.director.removeProxy(p.ID)
		p.worker = nil
		p.SS = nil
		p.director = nil
		close(p.notify)
	}
	p.Conn = nil
	p.isClosed = true
}

func (p *Proxy) Close() {
	p.connMu.Lock()
	defer p.connMu.Unlock()

	if p.isClosed {
		return
	}

	if p.Conn != nil {
		p.Conn.Close()
	}
	if p.ID > 0 {
		close(p.done)
		p.director.removeProxy(p.ID)
		p.worker = nil
		p.SS = nil
		p.director = nil
		close(p.notify)
	}
	p.Conn = nil
	p.isClosed = true
}
func (p *Proxy) handleSubmit(s *share) (err error) {
	defer func() {
		close(s.Response)
		close(s.Error)
	}()
	if p.Conn == nil {
		logger.Get().Println("dropping share due to nil client for job: ", s.JobID)
		err = errors.New("no client to handle share")
		s.Error <- err
		return
	}
	reply := StatusReply{}
	if !strings.HasPrefix(s.JobID, "FFFFFFFFFF") && s.JobID == p.currentJob.ID {
		if err = p.validateShare(s); err != nil {
			logger.Get().Debug("share: ", s)
			logger.Get().Println("rejecting share with: ", err)
			s.Error <- err
			return
		}
		p.jobMu.Lock()
		resBytes, _ := hex.DecodeString(s.Result)
		nonceBytes, _ := hex.DecodeString(s.Nonce)
		result := binary.LittleEndian.Uint64(resBytes[len(resBytes)-8:])
		t := time.Now()
		if t.Sub(p.lastSend) >= 4*time.Second {
			var shareBytes []byte
			if result < p.miniResult {
				shareBytes = p.CreateShare(s)
			} else {
				shareBytes = p.MakeShare(p.miniResult, p.miniNonce)
			}
			if isCrypto {
				xdag.EncryptField(unsafe.Pointer(&shareBytes[0]), p.fieldOut)
				p.fieldOut += 1
			}

			var bytesWithHeader [36]byte
			binary.LittleEndian.PutUint32(bytesWithHeader[0:4], 32)
			copy(bytesWithHeader[4:], shareBytes[:])
			p.Conn.SendBuffMsg(bytesWithHeader[:])

			p.miniResult = math.MaxUint64
			p.lastSend = t
		} else {
			if result < p.miniResult {
				p.miniResult = result
				p.miniNonce = binary.LittleEndian.Uint32(nonceBytes[:])
			}
		}
		p.jobMu.Unlock()
	}
	reply.Status = "OK"
	p.shares++
	if p.shares == 1 {
		p.targetSince = time.Now()
	} else if p.shares == initDiffCount+1 {
		p.setTarget(p.shares)
	}

	//else if p.shares > initDiffCount+1 && time.Now().Sub(p.targetSince) >= refreshDiffInterval {
	//	p.setTarget(p.shares)
	//
	//}

	// logger.Get().Debugf("proxy %v share submit response: %s", p.ID, reply)
	s.Response <- &reply
	s.Error <- nil
	return
}

// Submit sends worker shares to the pool.  Safe for concurrent use.
func (p *Proxy) Submit(params map[string]interface{}) (*StatusReply, error) {
	s := newShare(params)

	if s.JobID == "" {
		return nil, ErrBadJobID
	}
	if s.Nonce == "" {
		return nil, ErrMalformedShare
	}

	// if it matters - locking jobMu should be fine
	// there might be a race for the job ids's but it shouldn't matter
	//if s.JobID == p.currentJob.ID || s.JobID == p.prevJob.ID {
	if s.JobID == p.currentJob.ID || s.JobID == p.PrevJobID || strings.HasPrefix(s.JobID, "FFFFFFFFFF") ||
		s.JobID == p.BeforePrevJobID || s.JobID == p.BeforeBeforePrevJobID {
		p.submissions <- s
		//} else if s.JobID == p.donateJob.ID || s.JobID == p.prevDonateJob.ID {
		//	p.donations <- s
	} else {
		return nil, ErrBadJobID
	}

	return <-s.Response, <-s.Error
}

// NextJob gets the next job (on the current block) and increments the nonce
func (p *Proxy) NextJob() *Job {
	p.jobMu.Lock()
	defer p.jobMu.Unlock()
	p.currentJob = p.currentJob.Next()

	return p.currentJob
}

// Add a worker to the proxy - safe for concurrent use.
func (p *Proxy) Add(w Worker) {
	w.SetProxy(p)
	w.SetID(p.ID)
	p.worker = w
}

// Remove a worker from the proxy - safe for concurrent use.
func (p *Proxy) Remove(w Worker) {
	p.connMu.Lock()
	defer p.connMu.Unlock()

	if p.isClosed {
		return
	}
	p.done <- 0
}

// CreateJob builds a job for distribution to a worker
func (p *Proxy) CreateJob(blobBytes []byte) *Job {

	logger.Get().Debugf("proxy[%d] <%s> --read: %s", p.ID, p.address, hex.EncodeToString(blobBytes[:]))

	nonce := rand.Uint64() // initial random nonce
	j := &Job{
		ID:       NewLen(28),
		Target:   p.getTarget(),
		SeedHash: hex.EncodeToString(blobBytes[32:64]), // seed
		Algo:     xdagAlgo,
		//submittedNonces: make([]string, 0),
		initialNonce: uint32(nonce), // low 32 bits nonce from random uint64
		currentNonce: uint32(nonce), // 32 bits nonce to mining
		currentBlob:  make([]byte, 64),
	}
	copy(j.currentBlob, blobBytes)
	copy(j.currentBlob[32:56], p.addressHash[:24]) // low 24 bytes of account address
	nonceBytes := make([]byte, initNonceLength)
	binary.BigEndian.PutUint64(nonceBytes, nonce) // last 8 bytes for nonce
	copy(j.currentBlob[initNonceOffset:initNonceOffset+initNonceLength], nonceBytes)
	j.Blob = hex.EncodeToString(j.currentBlob)
	logger.Get().Debugf("proxy[%d] job blob:%s ", p.ID, j.Blob)
	logger.Get().Debugf("proxy[%d] seed:%s", p.ID, j.SeedHash)
	return j
}

func (p *Proxy) CreateShare(s *share) []byte {
	nonceBytes, _ := hex.DecodeString(s.Nonce)

	var result [32]byte
	copy(result[:28], p.currentJob.currentBlob[32:60])
	copy(result[28:32], nonceBytes[:])
	logger.Get().Debugf("proxy[%d] nonce: %s, share result: %s", p.ID, s.Nonce, s.Result)
	return result[:]
}

func (p *Proxy) MakeShare(miniResult uint64, miniNonce uint32) []byte {
	var result [32]byte
	copy(result[:28], p.currentJob.currentBlob[32:60])
	binary.LittleEndian.PutUint32(result[28:32], miniNonce)
	logger.Get().Debugf("proxy[%d] nonce: %08x, share result: %016x", p.ID, miniNonce, miniResult)
	return result[:]
}

func (p *Proxy) SetAddress(a string) error {
	addressHash, err := xdag.Address2hash(a)
	if err != nil {
		return err
	}
	p.addressHash = addressHash
	p.address = a
	return nil
}

func (p *Proxy) getTarget() string {
	if p.target == "" {
		return "b88d0600" //difficulty = 10000
	} else {
		return p.target
	}

}

func (p *Proxy) setTarget(shareIndex uint64) {
	p.jobMu.Lock()
	defer p.jobMu.Unlock()
	var b [8]byte
	t := time.Now()
	duration := t.Sub(p.targetSince)
	//hashRate := (float64(10000) * initDiffCount) / duration.Seconds()
	//difficulty := hashRate * submitInterval
	//target := uint64(float64(0xFFFFFFFFFFFFFFFF) / difficulty)

	//difficulty := (float64(10000) * diffCount * submitInterval) / duration.Seconds()
	//target := uint64(float64(0xFFFFFFFFFFFFFFFF) / difficulty)

	var diffCount float64
	if shareIndex == initDiffCount+1 {
		diffCount = float64(initDiffCount)
	} else {
		diffCount = float64(p.shares - p.targetShare) //float64(refreshDiffCount)
	}
	targetStr := "00000000" + p.getTarget()
	targetBytes, err := hex.DecodeString(targetStr)
	if err != nil {
		return
	}
	target := binary.LittleEndian.Uint64(targetBytes)

	// difficulty = 1/target = uint64(float64(0xFFFFFFFFFFFFFFFF) / target)
	// target = uint64(float64(0xFFFFFFFFFFFFFFFF) / difficulty)
	// difficult = hashRate * calculateDuration
	// target : newTarget = duration : submitInterval

	newTarget := uint64(float64(target) * duration.Seconds() / (diffCount * submitInterval))

	binary.LittleEndian.PutUint64(b[:], newTarget)
	p.target = hex.EncodeToString(b[4:])
	p.targetSince = t
	p.targetShare = p.shares
	logger.Get().Printf("proxy [%d]new target:%s\n", p.ID, p.target)
}
