package main

import (
	"flag"
	"fmt"
	"github.com/swordlet/xmrig2xdag/config"
	"github.com/swordlet/xmrig2xdag/logger"
	"github.com/swordlet/xmrig2xdag/tcp"
	"log"
	"os"
)

var (
	version = "1.0.0"

	// cmd line options
	configFile *string
)

func printWelcomeMessage() {
	logger.Get().Println("************************************************************************")
	logger.Get().Printf("*    XMR Stratum to XDAG Proxy \t\t\t\t v%s \n", version)

	if !config.Get().DisableTCP {
		port := config.Get().StratumPort
		logger.Get().Printf("*    Accepting XMRig Connections on port: \t\t\t\t %v\n", port)
	}
	statInterval := config.Get().StatInterval
	logger.Get().Printf("*    Printing stats every: \t\t\t\t %v seconds\n", statInterval)
	logger.Get().Println("************************************************************************")
}

func usage() {
	fmt.Printf("Usage: %s [-c CONFIG_PATH] \n", os.Args[0])
	flag.PrintDefaults()
}

func setOptions() {
	configFile = flag.String("c", "", "JSON file from which to read configuration values")
	flag.Parse()

	config.File = *configFile
}

func setupLogger() {
	lc := &logger.Config{W: nil}
	c := config.Get()
	if c.Debug {
		lc.Level = logger.Debug
	}
	if c.LogFile != "" {
		f, err := os.Create(c.LogFile)
		if err != nil {
			log.Fatal("could not open log file for writing: ", err)
		}
		lc.W = f
	}
	if c.DiscardLog {
		lc.Discard = true
	}
	logger.Configure(lc)
	logger.Get().Debugln("logger is configured")
}

func main() {
	//setOptions()
	//setupLogger()

	flag.Usage = usage

	flag.Parse()
	if args := flag.Args(); len(args) > 1 && (args[1] == "help" || args[1] == "-h") {
		flag.Usage()
		return
	}
	config.File = *configFile

	holdOpen := make(chan bool, 1)

	if config.Get().DisableWebsocket && config.Get().DisableTCP {
		logger.Get().Fatal("No servers configured for listening.  Bye!")
	}

	go tcp.StartServer()

	printWelcomeMessage()

	<-holdOpen
}
