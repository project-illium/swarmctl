// Copyright (c) 2022 The illium developers
// Use of this source code is governed by an MIT
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"github.com/jessevdk/go-flags"
	"os"
	"os/signal"
	"syscall"
)

type opts struct {
	ConfigFile string `short:"c" long:"config" description:"Path to the configuration file"`
}

func main() {
	o := opts{}
	parser := flags.NewParser(&o, flags.HelpFlag)
	_, err := parser.Parse()
	if err != nil {
		log.Fatal(err)
	}

	f, err := os.Open(o.ConfigFile)
	if err != nil {
		log.Fatal(err)
	}
	var config SwarmConfig
	decoder := json.NewDecoder(f)
	if err := decoder.Decode(&config); err != nil {
		log.Fatal(err)
	}

	if err := config.validate(); err != nil {
		log.Fatal(err)
	}

	setupLogging()

	done := make(chan struct{})
	go run(&config, done)

	// Listen for an exit signal and close.
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	for sig := range c {
		if sig == syscall.SIGINT || sig == syscall.SIGTERM {
			log.Info("swarmctl shutting down")
			close(done)
			os.Exit(1)
		}
	}
}
