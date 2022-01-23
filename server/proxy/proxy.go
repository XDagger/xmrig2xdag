package proxy

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/swordlet/xmrig2xdag/config"
	"github.com/swordlet/xmrig2xdag/logger"
	"github.com/swordlet/xmrig2xdag/stratum"
	"github.com/swordlet/xmrig2xdag/xdag"
	"hash/crc32"
	"math/rand"
	"net"
	"strings"
	"sync"
	"time"
	"unsafe"
)

const (
	// MaxUint protects IDs from overflow if the process runs for thousands of years
	MaxUint = ^uint64(0)

	// TODO adjust - lower means more connections to pool, potentially fewer stales if that is a problem
	maxProxyWorkers = 1024

	retryDelay = 60 * time.Second

	timeout = 60 * time.Second

	xdagAlgo = `rx/xdag`
)

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

	NewJob(*Job)
}

// Proxy manages a group of workers.
type Proxy struct {
	ID       uint64
	Conn     *xdag.Connection
	SS       *stratum.Server
	director *Director

	authID     string // identifies the proxy to the pool
	aliveSince time.Time
	shares     uint64

	workerCount int
	worker      Worker
	submissions chan *share
	notify      chan []byte
	done        chan int
	ready       bool
	currentJob  *Job
	//prevJob     *Job
	jobMu       sync.Mutex
	addressHash [xdag.HashLength]byte
	address     string

	crypt     unsafe.Pointer
	fieldOut  uint64
	fieldIn   uint64
	recvCount int
	recvByte  [2 * xdag.HashLength]byte
}

func init() {
	rand.Seed(time.Now().UnixNano())
}

// NewProxy creates a new proxy, starts the work thread, and returns a pointer to it.
func NewProxy(id uint64) *Proxy {
	p := &Proxy{
		ID:         id,
		aliveSince: time.Now(),
		currentJob: &Job{},
		//prevJob:     &Job{},
		submissions: make(chan *share),
		done:        make(chan int),
		ready:       true,
	}

	ss := stratum.NewServer()
	p.SS = ss
	p.SS.RegisterName("mining", &Mining{})
	logger.Get().Debugln("RPC server is listening on proxy ", p.ID)

	return p
}

func (p *Proxy) Run(minerName string) {
	for {
		err := p.connect(minerName)
		if err == nil {
			break
		}
		logger.Get().Printf("Failed to acquire pool connection.  Retrying in %s.Error: %s\n", retryDelay, err)
		// TODO allow fallback pools here
		<-time.After(retryDelay)
	}

	defer func() {
		p.shutdown()
	}()

	for {
		select {
		// these are from workers
		case s := <-p.submissions:
			err := p.handleSubmit(s) //, p.SC)
			if err != nil {
				logger.Get().Println("share submission error: ", err)
			}
			if err != nil && strings.Contains(strings.ToLower(err.Error()), "banned") {
				logger.Get().Println("Banned IP - killing proxy: ", p.ID)
				return
			}
		case notif := <-p.notify:
			p.handleNotification(notif)
		case <-p.done:
			return

		}
	}
}

func (p *Proxy) handleJob(job *Job) (err error) {
	p.jobMu.Lock()
	//p.prevJob, p.currentJob = p.currentJob, job
	p.currentJob = job
	p.jobMu.Unlock()

	if err != nil {
		// logger.Get().Debugln("Skipping regular job broadcast: ", err)
		return
	}

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

	ptr := unsafe.Pointer(&data[0])

	xdag.DecryptField(p.crypt, ptr, p.fieldIn)
	p.fieldIn += 1
	//xdag.DecryptField(p.crypt, unsafe.Add(ptr, uintptr(32)), p.fieldIn)
	//p.fieldIn += 1
	if xdag.Hash2address(data[:]) == p.address {
		p.recvCount = 0
	} else if p.recvCount == 0 {
		p.recvCount++
		copy(p.recvByte[:32], data[:])
	} else if p.recvCount == 1 {
		p.recvCount = 0
		copy(p.recvByte[32:], data[:])

		job := p.CreateJob(p.recvByte[:])
		err := p.handleJob(job)

		if err != nil {
			// log and wait for the next job?
			logger.Get().Println("error processing job: ", job)
			logger.Get().Println(err)
		}

	}

}

func (p *Proxy) connect(minerName string) error {
	p.crypt = xdag.InitCrypto()
	if p.crypt == nil {
		return errors.New("initialize crypto error")
	}
	conn, err := net.DialTimeout("tcp", config.Get().PoolAddr, timeout)
	if err != nil {
		return err
	}

	logger.Get().Debugln("Client made pool connection.")
	//p.SC = sc
	p.notify = make(chan []byte, 2)
	p.Conn = xdag.NewConnection(conn, p.ID, p.notify)
	p.Conn.Start()

	block := xdag.GenerateFakeBlock()
	binary.LittleEndian.PutUint64(block[0:8], 0)
	crc := crc32.Checksum(block[:], crc32table)
	newHeader := xdag.BLOCK_HEADER_WORD | (uint64(crc))<<32
	binary.LittleEndian.PutUint64(block[0:8], newHeader)
	ptr := unsafe.Pointer(&block[0])
	for i := 0; i < xdag.XDAG_BLOCK_FIELDS; i++ {
		xdag.EncryptField(p.crypt, unsafe.Add(ptr, uintptr(i)*xdag.FieldSize), p.fieldOut)
		p.fieldOut += 1
	}
	p.Conn.SendBuffMsg(block[:])

	time.Sleep(2 * time.Second)
	if minerName != "" {
		var field [32]byte
		binary.LittleEndian.PutUint32(field[0:4], xdag.WORKERNAME_HEADER_WORD)
		copy(field[4:32], minerName[:])
		xdag.EncryptField(p.crypt, unsafe.Pointer(&field[0]), p.fieldOut)
		p.fieldOut += 1
		p.Conn.SendBuffMsg(field[:])
	}

	logger.Get().Debugln("Successfully logged into pool.")

	logger.Get().Printf("****    Connected and logged in to pool server: %s \n", config.Get().PoolAddr)
	logger.Get().Println("****    Broadcasting jobs to workers.")

	return nil
}

func (p *Proxy) validateShare(s *share) error {
	var job *Job
	switch {
	case s.JobID == p.currentJob.ID:
		job = p.currentJob
	//case s.JobID == p.prevJob.ID:
	//	job = p.prevJob
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

func (p *Proxy) shutdown() {
	// kill worker connections - they should connect to a new proxy if active
	p.ready = false
	//for _, w := range p.workers {
	//	w.Disconnect()
	//}
	p.worker.Disconnect()

	xdag.FreeCrypto(p.crypt)

	<-time.After(10 * time.Second) //(1 * time.Minute)
	p.director.removeProxy(p)
}

func (p *Proxy) isReady() bool {
	// the worker count read is a race TODO
	return p.ready && p.workerCount == 1 // < maxProxyWorkers
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
	if !strings.HasPrefix(s.JobID, "FFFFFFFFFF") {
		if err = p.validateShare(s); err != nil {
			logger.Get().Debug("share: ", s)
			logger.Get().Println("rejecting share with: ", err)
			s.Error <- err
			return
		}

		shareBytes := p.CreateShare(s)
		xdag.EncryptField(p.crypt, unsafe.Pointer(&shareBytes[0]), p.fieldOut)
		p.fieldOut += 1
		p.Conn.SendBuffMsg(shareBytes)
	}
	reply.Status = "OK"
	p.shares++

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
	if s.JobID == p.currentJob.ID || strings.HasPrefix(s.JobID, "FFFFFFFFFF") {
		p.submissions <- s
		//} else if s.JobID == p.donateJob.ID || s.JobID == p.prevDonateJob.ID {
		//	p.donations <- s
	} else {
		return nil, ErrBadJobID
	}

	return <-s.Response, <-s.Error
}

// NextJob gets gets the next job (on the current block) and increments the nonce
func (p *Proxy) NextJob() *Job {
	p.jobMu.Lock()
	defer p.jobMu.Unlock()
	p.currentJob = p.currentJob.Next()

	return p.currentJob
}

// Add a worker to the proxy - safe for concurrent use.
func (p *Proxy) Add(w Worker) {
	w.SetProxy(p)
	//w.SetID(p.nextWorkerID())	//
	//p.addWorker <- w
	w.SetID(p.ID)
	p.worker = w
	p.workerCount = 1
}

// Remove a worker from the proxy - safe for concurrent use.
func (p *Proxy) Remove(w Worker) {
	//p.delWorker <- w
	p.director.removeProxy(p)
	p.done <- 0
}

// CreateJob builds a job for distribution to a worker
func (p *Proxy) CreateJob(blobBytes []byte) *Job {

	fmt.Println("read: ", hex.EncodeToString(blobBytes[:]))

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
	nonceBytes := make([]byte, initNonceLength, initNonceLength)
	binary.BigEndian.PutUint64(nonceBytes, nonce) // last 8 bytes for nonce
	copy(j.currentBlob[initNonceOffset:initNonceOffset+initNonceLength], nonceBytes)
	j.Blob = hex.EncodeToString(j.currentBlob)
	fmt.Println("job blob: ", j.Blob)
	fmt.Println("seed: ", j.SeedHash)
	return j
}

func (p *Proxy) CreateShare(s *share) []byte {
	nonceBytes, _ := hex.DecodeString(s.Nonce)

	var result [32]byte
	copy(result[:28], p.currentJob.currentBlob[32:60])
	copy(result[28:32], nonceBytes[:])
	fmt.Println("nonce: ", s.Nonce, ", share result: ", s.Result)
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
	return "3f8d0600"
}