package tcp

import (
	"context"
	"net"
	"time"

	"github.com/swordlet/xmrig2xdag/logger"
	"github.com/swordlet/xmrig2xdag/proxy"
	"github.com/swordlet/xmrig2xdag/stratum"
)

const (
	workerTimeout  = 1 * time.Minute
	jobSendTimeout = 30 * time.Second
)

// Worker does the work (of mining, well more like accounting)
type Worker struct {
	conn net.Conn
	id   uint64
	p    *proxy.Proxy

	// codec will be used directly for sending jobs
	// this is not ideal, and it would be nice to do this differently
	codec *stratum.DefaultServerCodec

	//jobs chan *proxy.Job
}

// SpawnWorker spawns a new TCP worker and adds it to a proxy
func SpawnWorker(conn net.Conn) {
	w := &Worker{
		conn: conn,
		//jobs: make(chan *proxy.Job),
	}
	ctx := context.WithValue(context.Background(), "worker", w)
	codec := stratum.NewDefaultServerCodecContext(ctx, w.Conn())
	w.codec = codec.(*stratum.DefaultServerCodec)

	p := proxy.GetDirector().NextProxy()
	logger.Get().Debugln("New proxy.", p.ID)
	p.Add(w) // set worker's proxy

	// blocks until disconnect
	w.Proxy().SS.ServeCodec(codec)

	w.Disconnect()
}

func (w *Worker) Conn() net.Conn {
	return w.conn
}

func (w *Worker) SetConn(c net.Conn) {
	w.conn = c
}

// Worker interfaces

func (w *Worker) ID() uint64 {
	return w.id
}

func (w *Worker) SetID(i uint64) {
	w.id = i
}

func (w *Worker) SetProxy(p *proxy.Proxy) {
	w.p = p
}

func (w *Worker) Proxy() *proxy.Proxy {
	return w.p
}

func (w *Worker) Disconnect() {
	w.Conn().Close()
	if w.p != nil {
		w.p.Remove(w)
		w.p = nil
	}

}

func (w *Worker) RemoveProxy() {
	if w.p != nil {
		logger.Get().Printf("proxy [%d] shutdown by login error\n", w.p.ID)
		w.p.Close()
		w.p = nil
	}
}

func (w *Worker) Close() {
	w.p = nil
	w.Conn().Close()

}

func (w *Worker) NewJob(j *proxy.Job) {
	err := w.codec.Notify("job", j)
	if err != nil {
		// logger.Get().Debugln("Error sending job to worker: ", err)
		w.Disconnect()
	}
	// other actions? shut down worker?
}

func (w *Worker) expectedHashes() uint32 {
	// this is a complete unknown at this time.
	return 0x7a120
}
