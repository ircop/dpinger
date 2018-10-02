package main

import (
	"flag"
	"fmt"
	"github.com/ircop/dpinger/cfg"
	"github.com/ircop/dpinger/logger"
	"github.com/ircop/dpinger/nats"
	"github.com/ircop/dpinger/pinger"
	"math/rand"
	_ "net/http/pprof"
	"time"
)

func main() {
	// run ; start listener ; daemonize
	//go http.ListenAndServe(":6060", nil)

	// just in case:
	rand.NewSource(time.Now().UnixNano())

	configPath := flag.String("c", "./dpinger.toml", "Config file location")
	flag.Parse()

	// config
	config, err := cfg.NewCfg(*configPath)
	if err != nil {
		fmt.Printf("[FATAL]: Cannot read config: %s\n", err.Error())
		return
	}

	// logger
	if err := logger.InitLogger(config.LogDebug, config.LogDir); err != nil {
		fmt.Printf("[FATAL]: %s\n", err.Error())
		return
	}

	// Init pinger
	if err = pinger.Pinger.Init(); err != nil {
		logger.Err(err.Error())
		return
	}

	// Init NATS
	if err = nats.Init(config.NatsURL, config.NatsDB, config.NatsPing); err != nil {
		logger.Err(err.Error())
		return
	}
	// request DBD sync
	nats.RequestSync()

	select{}
}
