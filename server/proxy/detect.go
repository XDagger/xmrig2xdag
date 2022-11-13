package proxy

import (
	"math"
	"time"

	"github.com/swordlet/xmrig2xdag/xdag"
)

func PoolDetect() {
	p := &Proxy{
		ID:          0,
		aliveSince:  time.Now(),
		currentJob:  &Job{},
		PrevJobID:   NewLen(28),
		submissions: make(chan *share),
		done:        make(chan int),
		ready:       true,
		lastSend:    time.Now(),
		miniResult:  math.MaxUint64,
		notify:      make(chan []byte, 2),
		address:     detectProxy,
	}
	timer := time.NewTicker(10 * time.Minute)
	for {
		<-timer.C
		if poolIsDown.Load() > 0 {
			p.fieldIn = 0
			p.fieldOut = 0
			p.recvCount = 0
			p.isClosed = false
			for len(p.done) > 0 {
				<-p.done
			}
			for len(p.notify) > 0 {
				<-p.notify
			}
			eofCount.Store(0)
			xdag.PoolDown.Store(0)
			go p.Run(detectProxy)
		}

	}
}
