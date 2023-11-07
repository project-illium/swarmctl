// Copyright (c) 2022 The illium developers
// Use of this source code is governed by an MIT
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"github.com/multiformats/go-multiaddr"
)

type SwarmConfig struct {
	TxPerMinute int      `json:"TxPerMinute"`
	RPCCert     string   `json:"RPCCert"`
	GenesisNode string   `json:"GenesisNode"`
	Nodes       []string `json:"Nodes"`
}

func (c *SwarmConfig) validate() error {
	_, err := multiaddr.NewMultiaddr(c.GenesisNode)
	if err != nil {
		return fmt.Errorf("invalid genesis node multiaddr: %s", err.Error())
	}
	for _, n := range c.Nodes {
		_, err := multiaddr.NewMultiaddr(n)
		if err != nil {
			return fmt.Errorf("invalid node multiaddr: %s", err.Error())
		}
	}
	return nil
}
