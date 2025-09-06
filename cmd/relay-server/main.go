// Copyright 2025 JC-Lab
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-FEL

package main

import (
	"context"
	"flag"
	"log"

	"github.com/flymesh/core/internal/relay-server"
	"github.com/flymesh/core/internal/util"
	"github.com/flymesh/core/p2p"
	"github.com/libp2p/go-libp2p"
)

func main() {
	privKeyFile := flag.String("private-key", "", "path to private key file")
	listenPort := flag.Int("listen-port", 0, "listen port")
	relayListen := flag.String("relay-server-listen", ":24002", "relay-server TCP listen address")
	flag.Parse()

	if *privKeyFile == "" {
		log.Fatal("missing --private-key")
	}

	priv, err := util.LoadOrCreatePrivateKey(*privKeyFile)
	if err != nil {
		log.Fatalf("load private key failed: %+v", err)
	}

	node := &p2p.Node{
		PrivKey:    priv,
		ListenPort: *listenPort,
		Libp2pOptions: []libp2p.Option{
			libp2p.EnableRelayService(),
		},
	}
	if err := node.Init(); err != nil {
		log.Fatalf("node initialize failed: %+v", err)
	}
	ctx := context.Background()

	log.Printf("Host ID: %s", node.Host.ID())
	for _, a := range node.Host.Addrs() {
		log.Printf("Listen on: %s/p2p/%s", a, node.Host.ID())
	}

	relay_server.Run(ctx, node, *relayListen)

	select {}
}
