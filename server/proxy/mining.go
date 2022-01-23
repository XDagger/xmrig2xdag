package proxy

import (
	"context"
	"errors"
	"strconv"

	"github.com/sourcegraph/jsonrpc2"
	"github.com/swordlet/xmrig2xdag/logger"
)

// PassThruParams is a generic type for handling RPC requests.  It can (should) contain the context
// of the request in order to be handled correctly.  Other than the context, everything else should
// be shipped off to the pool as is.  If that is not the correct behavior, use another type for params.
type PassThruParams map[string]interface{}

// Context implements jsonrpc2.WithContext
func (p PassThruParams) Context() context.Context {
	if ctx, ok := p["ctx"]; ok {
		return ctx.(context.Context)
	}
	logger.Get().Println("Failed to get context on request with params: ", p)
	return nil
}

// SetContext implements jsonrpc2.WithContext
func (p *PassThruParams) SetContext(ctx context.Context) {
	if *p == nil {
		*p = make(PassThruParams)
	}
	params := *p
	params["ctx"] = ctx
}

// structures for non-passthru objects, and replies

type AuthReply struct {
	Token  string `json:"token"`
	Hashes string `json:"hashes"`
}

type LoginReply struct {
	ID     string          `json:"id"`
	Job    *Job            `json:"job"`
	Status string          `json:"status"`
	Error  *jsonrpc2.Error `json:"error,omitempty"`
}

type StatusReply struct {
	Status string          `json:"status"`
	Error  *jsonrpc2.Error `json:"error,omitempty"`
}

// RPC proxy service
type Mining struct{}

func (m *Mining) getWorker(ctx context.Context) Worker {
	return ctx.Value("worker").(Worker)
}

func (m *Mining) Login(p PassThruParams, resp *LoginReply) error {
	var err error
	var minerName string
	worker := m.getWorker(p.Context())
	if address, ok := p["login"]; ok {
		err = worker.Proxy().SetAddress(address.(string))
	} else {
		err = errors.New("no login param")
	}
	if err != nil {
		resp.ID = strconv.Itoa(int(worker.ID()))
		resp.Error = &jsonrpc2.Error{
			Code:    -1,
			Message: err.Error(),
		}
		return err
	}

	if name, ok := p["rig-id"]; ok {
		minerName = name.(string)
	}

	go worker.Proxy().Run(minerName)
	//resp.Job = worker.Proxy().NextJob()
	resp.ID = strconv.Itoa(int(worker.ID()))
	resp.Status = "OK"
	resp.Job = &Job{
		ID:       "FFFFFFFFFF" + NewLen(18),
		Target:   "3f8d0600",
		Algo:     xdagAlgo,
		Blob:     "070780e6b9d60586ba419a0c224e3c6c3e134cc45c4fa04d8ee2d91c2595463c57eef0a4f0796c000000002fcc4d62fa6c77e76c30017c768be5c61d83ec9d3a",
		SeedHash: "e1364b8782719d7683e2ccd3d8f724bc59dfa780a9e960e7c0e0046acdb40100",
	}

	return nil
}

func (m *Mining) Getjob(p PassThruParams, resp *Job) error {
	worker := m.getWorker(p.Context())
	*resp = *worker.Proxy().NextJob()

	return nil
}

// Submit accepts shares from a worker and passes them through to the pool.
// This does NOT currently recognize which worker or even what type of worker
// is doing the submiting, and does not return a Coinhive friendly response.
// But the coinhive miner doesn't care, it just doesn't keep up with submissions.
func (m *Mining) Submit(p PassThruParams, resp *StatusReply) error {
	worker := m.getWorker(p.Context())
	status, err := worker.Proxy().Submit(p)
	if err != nil {
		return err
	}
	*resp = *status

	return nil
}

// Keepalived lets the client tell you they're still there, and you get to say "I'm still here too"
// Right now, we don't keep track of idle connections, so this doesn't really matter.
func (m *Mining) Keepalived(p PassThruParams, resp *StatusReply) error {
	resp.Status = "KEEPALIVED"
	return nil
}
