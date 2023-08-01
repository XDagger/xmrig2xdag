package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/swordlet/xmrig2xdag/config"
	"github.com/swordlet/xmrig2xdag/logger"
	"github.com/swordlet/xmrig2xdag/proxy"
	"github.com/swordlet/xmrig2xdag/tcp"
)

var (
	version = "2.0.2"

	// cmd line options
	configFile *string
	logFile    *os.File
)

func printWelcomeMessage() {
	logger.Get().Println("************************************************************************")
	logger.Get().Printf("*    XMR Stratum to XDAG Proxy \t\t\t v%s \t*\n", version)

	port := config.Get().StratumPort
	//var tls string
	//if config.Get().Tls {
	//	tls = "tls"
	//}
	logger.Get().Printf("*    Accepting XMRig Connections on port: \t\t %v\t\t*\n", port)

	statInterval := config.Get().StatInterval
	logger.Get().Printf("*    Printing stats every: \t\t\t\t %v seconds\t*\n", statInterval)
	logger.Get().Println("************************************************************************")
}

func usage() {
	fmt.Printf("Usage: %s [-c CONFIG_PATH] \n", os.Args[0])
	flag.PrintDefaults()
}

func setOptions() {
	configFile = flag.String("c", "config.json", "JSON file from which to read configuration values")
	flag.Parse()

	config.File = *configFile
}

func setupLogger() {
	lc := &logger.Config{W: nil}
	c := config.Get()
	if c.Debug {
		lc.Level = logger.Debug
	}
	var err error
	if c.LogFile != "" {
		if logger.CheckFileExist(c.LogFile) {
			logFile, err = os.OpenFile(c.LogFile, os.O_APPEND|os.O_RDWR, 0644)
			if err != nil {
				log.Fatal("could not open log file for writing: ", err)
			}
		} else {
			logFile, err = os.Create(c.LogFile)
			if err != nil {
				log.Fatal("could not create log file for writing: ", err)
			}
		}
		lc.W = io.MultiWriter(os.Stdout, logFile)
	}
	if c.DiscardLog {
		lc.Discard = true
	}
	logger.Configure(lc)
	logger.Get().Debugln("logger is configured")
}

func main() {
	setOptions()
	setupLogger()
	defer func() {
		if logFile != nil {
			logFile.Close()
		}
	}()

	flag.Usage = usage

	flag.Parse()
	if args := flag.Args(); len(args) > 1 && (args[1] == "help" || args[1] == "-h") {
		flag.Usage()
		return
	}
	config.File = *configFile

	holdOpen := make(chan bool, 1)

	go tcp.StartServer()
	go proxy.PoolDetect()

	printWelcomeMessage()

	<-holdOpen
}
