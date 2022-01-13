package proxy

import (
	"errors"
	"github.com/swordlet/xmrig2xdag/xdag"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/swordlet/xmrig2xdag/config"
	"github.com/swordlet/xmrig2xdag/logger"
	"github.com/swordlet/xmrig2xdag/stratum"
)

const (
	// MaxUint protects IDs from overflow if the process runs for thousands of years
	MaxUint = ^uint64(0)

	// TODO adjust - lower means more connections to pool, potentially fewer stales if that is a problem
	maxProxyWorkers = 1024

	retryDelay = 60 * time.Second

	timeout = 60 * time.Second
)

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
	ready       bool
	currentJob  *Job
	prevJob     *Job
	jobMu       sync.Mutex
	addressHash [xdag.HashLength]byte
}

// New creates a new proxy, starts the work thread, and returns a pointer to it.
func New(id uint64) *Proxy {
	p := &Proxy{
		ID:          id,
		aliveSince:  time.Now(),
		currentJob:  &Job{},
		prevJob:     &Job{},
		submissions: make(chan *share),
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

		}
	}
}

func (p *Proxy) handleJob(job *Job) (err error) {
	p.jobMu.Lock()
	p.prevJob, p.currentJob = p.currentJob, job
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
	p.worker.NewJob(p.NextJob())
}

func (p *Proxy) handleNotification(notif []byte) {
	job := p.CreateJob(notif)
	err := p.handleJob(job)

	if err != nil {
		// log and wait for the next job?
		logger.Get().Println("error processing job: ", job)
		logger.Get().Println(err)
	}

}

func (p *Proxy) connect(minerName string) error {
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
	p.Conn.SendBuffMsg(block[:])
	if minerName != "" {
		p.Conn.SendBuffMsg([]byte(minerName))
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
	case s.JobID == p.prevJob.ID:
		job = p.prevJob
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

	if err = p.validateShare(s); err != nil {
		logger.Get().Debug("share: ", s)
		logger.Get().Println("rejecting share with: ", err)
		s.Error <- err
		return
	}

	reply := StatusReply{}
	shareBytes := p.CreateShare(s)
	p.Conn.SendBuffMsg(shareBytes)
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
	if s.JobID == p.currentJob.ID || s.JobID == p.prevJob.ID {
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
	var j *Job
	j = p.currentJob.Next()

	return j
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
}

// CreateJob builds a job for distribution to a worker
func (p *Proxy) CreateJob(blobBytes []byte) *Job {
	j := &Job{
		//ID:              id,
		//Target:          target,
		//submittedNonces: make([]string, 0),
	}
	//nonceBytes := make([]byte, nonceLength, nonceLength)
	//binary.BigEndian.PutUint32(nonceBytes, nonce)
	//copy(blobBytes[nonceOffset:nonceOffset+nonceLength], nonceBytes)
	//j.Blob = hex.EncodeToString(blobBytes)

	return j
}

func (p *Proxy) CreateShare(s *share) []byte {
	return []byte{}
}

func (p *Proxy) SetAddress(a string) error {
	addressHash, err := xdag.Address2hash(a)
	if err != nil {
		return err
	}
	p.addressHash = addressHash
	return nil
}
