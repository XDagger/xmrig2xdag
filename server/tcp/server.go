package tcp

import (
	"crypto/tls"
	"net"
	"strconv"

	"github.com/swordlet/xmrig2xdag/config"
	"github.com/swordlet/xmrig2xdag/logger"
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
		logger.Get().Fatal("Unable to listen for tcp connections on port: ", listener.Addr(),
			" Listen failed with error: ", listenErr)
		return
	}
	for {
		conn, err := listener.Accept()
		if err != nil {
			logger.Get().Println("Unable to accept connection: ", err)
		}
		go SpawnWorker(conn)
	}
}
