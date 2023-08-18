package proxy

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/swordlet/xmrig2xdag/config"
	"github.com/swordlet/xmrig2xdag/logger"
	"github.com/swordlet/xmrig2xdag/utils"
)

var (
	directorInstance      *Director
	directorInstantiation = sync.Once{}
)

type Director struct {
	aliveSince   time.Time
	statInterval time.Duration

	currentProxyID atomic.Uint64
	proxies        *utils.SafeMap

	// stat tracking only
	lastTotalShares uint64
	deleteShares    atomic.Uint64
}

func GetDirector() *Director {
	directorInstantiation.Do(func() {
		directorInstance = newDirector()
	})
	return directorInstance
}

func newDirector() *Director {
	d := &Director{
		aliveSince:   time.Now(),
		statInterval: time.Duration(config.Get().StatInterval) * time.Second,
		proxies:      utils.NewSafeMap(),
	}
	go d.run()

	return d
}

// Stats is a struct containing information about server uptime and activity, generated on demand
type Stats struct {
	Timestamp time.Time
	Alive     time.Duration
	Proxies   int
	Workers   int
	Shares    uint64
	NewShares uint64
}

func (d *Director) addProxy() *Proxy {
	p := NewProxy(d.nextProxyID())
	p.director = d
	d.proxies.Set(p.ID, p)
	return p
}

func (d *Director) run() {
	statPrinter := time.NewTicker(d.statInterval)
	defer statPrinter.Stop()
	for {
		<-statPrinter.C
		d.printStats()
	}
}

func (d *Director) printStats() {
	if poolIsDown.Load() > 0 {
		return
	}

	stats := d.GetStats()
	logger.Get().Printf("  uptime:%s  \t proxies:%v \t workers:%v \t shares:%v(+%v)\n",
		stats.Alive, stats.Proxies, stats.Workers, stats.Shares, stats.NewShares)
}

func (d *Director) removeProxy(id uint64) {
	p, ok := d.proxies.Get(id)
	if ok {
		d.deleteShares.Add(p.(*Proxy).shares)
	}
	d.proxies.Del(id)
}

func (d *Director) nextProxyID() uint64 {
	return d.currentProxyID.Add(1)
}

func (d *Director) NextProxy() *Proxy {
	var pr *Proxy = d.addProxy()
	return pr
}

func (d *Director) GetStats() *Stats {
	totalProxies := 0
	totalWorkers := 0
	var totalSharesSubmitted uint64

	d.proxies.Range(func(key interface{}, p interface{}) bool {
		totalProxies++
		totalWorkers++
		if p != nil {
			totalSharesSubmitted += p.(*Proxy).shares
		}
		return true
	})
	totalSharesSubmitted += d.deleteShares.Load()

	recentShares := totalSharesSubmitted - d.lastTotalShares
	d.lastTotalShares = totalSharesSubmitted
	duration := time.Since(d.aliveSince).Truncate(1 * time.Second)

	stats := &Stats{
		Timestamp: time.Now(),
		Alive:     duration,
		Proxies:   totalProxies,
		Workers:   totalWorkers,
		Shares:    totalSharesSubmitted,
		NewShares: recentShares,
	}

	return stats
}
