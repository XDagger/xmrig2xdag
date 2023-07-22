package tcp

import (
	"crypto/tls"
	"net"
	"strconv"
	"time"

	"github.com/swordlet/xmrig2xdag/config"
	"github.com/swordlet/xmrig2xdag/logger"
	"golang.org/x/time/rate"
)

func StartServer() {
	tcpPort := config.Get().StratumPort
	// TODO expose bind address?
	portStr := ":" + strconv.Itoa(tcpPort)

	logger.Get().Debug("Starting TCP listener on port: ", portStr)
	var listener net.Listener
	var listenErr error
	if config.Get().Tls {
		cert, err := tls.LoadX509KeyPair(config.Get().CertFile, config.Get().KeyFile)
		if err != nil {
			logger.Get().Fatal("Unable to open cert file ", config.Get().CertFile, " or key file ",
				config.Get().KeyFile)
			return
		}
		tlsConfig := &tls.Config{Certificates: []tls.Certificate{cert}}
		listener, listenErr = tls.Listen("tcp", portStr, tlsConfig)
	} else {
		listener, listenErr = net.Listen("tcp", portStr)
	}

	if listenErr != nil {
		logger.Get().Fatal("Unable to listen for tcp connections on port ", portStr,
			" Listen failed with error: ", listenErr)
		return
	}
	rl := config.Get().RateLimit
	if rl == 0 {
		rl = 1
	}
	ra := rate.Every(25 * time.Millisecond)
	limit := rate.NewLimiter(ra, rl)
	for {
		conn, err := listener.Accept()
		if err != nil {
			logger.Get().Println("Unable to accept connection: ", err)
		}
		// conn.SetDeadline(time.Now().Add(45 * time.Second))

		if !limit.Allow() {
			logger.Get().Println("Out of rate limit:", rl, "per 100 ms.")
			conn.Close()
			continue
		}
		go SpawnWorker(conn)
	}
}
