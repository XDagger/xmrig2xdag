package proxy

import (
	"errors"
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

	keepAliveInterval = 5 * time.Minute
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
	SC       *stratum.Client
	DC       *stratum.Client
	SS       *stratum.Server
	director *Director

	authID     string // identifies the proxy to the pool
	aliveSince time.Time
	shares     uint64

	workerCount int

	// workers have to be ID'd so they can be removed when they die
	workerIDs chan uint64
	workers   map[uint64]Worker

	addWorker chan Worker
	delWorker chan Worker

	submissions chan *share

	notify chan stratum.Notification

	ready bool

	currentJob *Job
	prevJob    *Job

	jobMu     sync.Mutex
	jobWaiter *sync.WaitGroup // waits for the first job
}

// New creates a new proxy, starts the work thread, and returns a pointer to it.
func New(id uint64) *Proxy {
	p := &Proxy{
		ID:         id,
		aliveSince: time.Now(),
		workerIDs:  make(chan uint64, 5),
		workers:    make(map[uint64]Worker),

		currentJob: &Job{},
		prevJob:    &Job{},

		addWorker: make(chan Worker),
		delWorker: make(chan Worker, 1),

		submissions: make(chan *share),

		ready:     true,
		jobWaiter: &sync.WaitGroup{},
	}
	p.jobWaiter.Add(1)

	ss := stratum.NewServer()
	p.SS = ss
	p.SS.RegisterName("mining", &Mining{})
	logger.Get().Debugln("RPC server is listening on proxy ", p.ID)

	go p.generateIDs()
	go p.run()
	return p
}

func (p *Proxy) generateIDs() {
	var currentWorkerID uint64

	for {
		currentWorkerID++
		p.workerIDs <- currentWorkerID
	}
}

// nextWorkerID returns the next sequential orderID.  It is safe for concurrent use.
func (p *Proxy) nextWorkerID() uint64 {
	return <-p.workerIDs
}

func (p *Proxy) run() {
	for {
		err := p.login()
		if err == nil {
			break
		}
		logger.Get().Printf("Failed to acquire pool connection.  Retrying in %s.Error: %s\n", retryDelay, err)
		// TODO allow fallback pools here
		<-time.After(retryDelay)
	}

	keepalive := time.NewTicker(keepAliveInterval)

	defer func() {
		keepalive.Stop()
		p.shutdown()
	}()

	for {
		select {
		// these are from workers
		case s := <-p.submissions:
			err := p.handleSubmit(s, p.SC)
			if err != nil {
				logger.Get().Println("share submission error: ", err)
			}
			if err != nil && strings.Contains(strings.ToLower(err.Error()), "banned") {
				logger.Get().Println("Banned IP - killing proxy: ", p.ID)
				return
			}
		case w := <-p.addWorker:
			p.receiveWorker(w)
		case w := <-p.delWorker:
			p.removeWorker(w)

		// this comes from the stratum client
		case notif := <-p.notify:
			p.handleNotification(notif)
		case <-keepalive.C:
			reply := StatusReply{}
			err := p.SC.Call("keepalived", map[string]string{"id": p.authID}, &reply)
			if reply.Error != nil {
				err = reply.Error
			}
			if err != nil {
				logger.Get().Println("Received error from keepalive request: ", err)
				return
			}
			logger.Get().Debugln("Keepalived response: ", reply)
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
	for _, w := range p.workers {
		go w.NewJob(p.NextJob())
	}
}

func (p *Proxy) handleNotification(notif stratum.Notification) {
	switch notif.Method {
	case "job":
		job, err := NewJobFromServer(notif.Params.(map[string]interface{}))
		if err != nil {
			logger.Get().Println("bad job: ", notif.Params)
			break
		}

		err = p.handleJob(job)

		if err != nil {
			// log and wait for the next job?
			logger.Get().Println("error processing job: ", job)
			logger.Get().Println(err)
		}
	default:
		logger.Get().Println("Received notification from server: ",
			"method: ", notif.Method,
			"params: ", notif.Params,
		)
	}
}

func (p *Proxy) login() error {
	sc, err := stratum.Dial("tcp", config.Get().PoolAddr)
	if err != nil {
		return err
	}
	logger.Get().Debugln("Client made pool connection.")
	p.SC = sc

	p.notify = p.SC.Notifications()
	params := map[string]interface{}{
		"login": config.Get().PoolLogin,
		"pass":  config.Get().PoolPassword,
	}
	reply := LoginReply{}
	err = p.SC.Call("login", params, &reply)
	if err != nil {
		return err
	}
	logger.Get().Debugln("Successfully logged into pool.")
	p.authID = reply.ID
	if err = reply.Job.init(); err != nil {
		logger.Get().Println("bad job from login: ", reply.Job, "- err: ", err)
		// still just wait for the next job
	} else if err = p.handleJob(reply.Job); err != nil {
		logger.Get().Println("Error processing job: ", reply.Job)
		// continue and just wait for the next job?
		// this shouldn't happen
	}

	logger.Get().Printf("****    Connected and logged in to pool server: %s \n", config.Get().PoolAddr)
	logger.Get().Println("****    Broadcasting jobs to workers.")

	// now we have a job, so release jobs
	p.jobWaiter.Done()

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

func (p *Proxy) receiveWorker(w Worker) {
	p.workers[w.ID()] = w
	p.workerCount++
}

func (p *Proxy) removeWorker(w Worker) {
	delete(p.workers, w.ID())
	p.workerCount--
	// potentially check for len(workers) == 0, start timer to spin down proxy if empty
	// like apache, we might expire a proxy at some point anyway, just to try and reclaim potential resources
	// in workers map, avert id overflow, etc.
}

func (p *Proxy) shutdown() {
	// kill worker connections - they should connect to a new proxy if active
	p.ready = false
	for _, w := range p.workers {
		w.Disconnect()
	}
	<-time.After(1 * time.Minute)
	p.director.removeProxy(p)
}

func (p *Proxy) isReady() bool {
	// the worker count read is a race TODO
	return p.ready && p.workerCount < maxProxyWorkers
}

func (p *Proxy) handleSubmit(s *share, c *stratum.Client) (err error) {
	defer func() {
		close(s.Response)
		close(s.Error)
	}()
	if c == nil {
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

	s.AuthID = p.authID
	reply := StatusReply{}
	if err = c.Call("submit", s, &reply); err != nil {
		s.Error <- err
		return
	}
	if reply.Status == "OK" {
		p.shares++
	}

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
	p.jobWaiter.Wait() // only waits for first job from login
	p.jobMu.Lock()
	defer p.jobMu.Unlock()
	var j *Job
	//if !p.donating {
	j = p.currentJob.Next()
	//} else {
	//	j = p.donateJob.Next()
	//}

	return j
}

// Add a worker to the proxy - safe for concurrent use.
func (p *Proxy) Add(w Worker) {
	w.SetProxy(p)
	w.SetID(p.nextWorkerID())

	p.addWorker <- w
}

// Remove a worker from the proxy - safe for concurrent use.
func (p *Proxy) Remove(w Worker) {
	p.delWorker <- w
}
