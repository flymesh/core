// Copyright 2025 JC-Lab
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-FEL

package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/flymesh/core/internal/util"
	"github.com/flymesh/core/p2p"
	relay_client "github.com/flymesh/core/relay-client"

	peer "github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

func main() {
	mode := flag.String("mode", "", "server | client")
	privKeyFile := flag.String("private-key", "", "path to private key file")
	listenPort := flag.Int("listen-port", 0, "listen port")
	remoteAddr := flag.String("remote", "", "remote peer multiaddr (client mode)")
	duration := flag.Int("duration", 10, "throughput test duration in seconds")
	sendMode := flag.Bool("send", true, "client mode: send or receive on relay-server TCP")
	// relay-server config
	relayPeer := flag.String("relay-server-peer", "", "relay-server peer ID (server mode)")
	relayAddr := flag.String("relay-server-addr", "", "relay-server peer multiaddr (server mode, optional)")
	flag.Parse()

	if *mode == "" {
		log.Fatal("missing --mode")
	}
	if *privKeyFile == "" {
		log.Fatal("missing --private-key")
	}

	// Load private key
	priv, err := util.LoadOrCreatePrivateKey(*privKeyFile)
	if err != nil {
		log.Fatalf("load private key failed: %+v", err)
	}

	// Build libp2p node
	node := &p2p.Node{
		PrivKey:    priv,
		ListenPort: *listenPort,
	}
	if err := node.Init(); err != nil {
		log.Fatalf("node initialize failed: %+v", err)
	}
	ctx := context.Background()

	log.Printf("Host ID: %s", node.Host.ID())
	for _, a := range node.Host.Addrs() {
		log.Printf("Listen on: %s/p2p/%s", a, node.Host.ID())
	}

	switch *mode {
	case "server":
		if *relayPeer == "" && *relayAddr == "" {
			log.Fatal("server mode requires --relay-server-peer=<peerID> or --relay-server-addr=<multiaddr>")
		}
		runServerMode(ctx, node, *relayPeer, *relayAddr, *duration)
	case "client":
		if *remoteAddr == "" {
			log.Fatal("client mode requires --remote=<multiaddr>")
		}
		runClientMode(ctx, node, *remoteAddr, *duration, *sendMode)
	default:
		log.Fatalf("unknown --mode: %s", *mode)
	}
	select {}
}

// --------------- server mode -----------------

func runServerMode(ctx context.Context, node *p2p.Node, relayPeerID string, relayMaddr string, duration int) {
	var (
		rpid peer.ID
		err  error
	)

	if relayMaddr != "" {
		maddr, err := ma.NewMultiaddr(relayMaddr)
		if err != nil {
			log.Fatalf("bad --relay-server-addr: %v", err)
		}
		info, err := peer.AddrInfoFromP2pAddr(maddr)
		if err != nil {
			log.Fatalf("bad --relay-server-addr: %v", err)
		}
		rpid = info.ID
		connectCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if err := node.Host.Connect(connectCtx, *info); err != nil {
			log.Fatalf("connect to relay-server failed: %v", err)
		}
	} else {
		rpid, err = peer.Decode(relayPeerID)
		if err != nil {
			log.Fatalf("bad --relay-server-peer: %v", err)
		}
		// Attempt to connect using routed host (DHT) if possible
		connectCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		_ = node.Host.Connect(connectCtx, peer.AddrInfo{ID: rpid})
	}

	serverRole := &relay_client.ServerRole{
		PrivKey:     node.PrivKey,
		RelayPeerId: rpid,
	}

	serverRole.RegisterProtocol(node.Host)

	log.Printf("[server] ready. Waiting for clients...")
}

func runClientMode(ctx context.Context, node *p2p.Node, remote string, duration int, send bool) {
	clientRole := &relay_client.ClientRole{
		PrivKey: node.PrivKey,
	}

	// Parse remote addr
	maddr, err := ma.NewMultiaddr(remote)
	if err != nil {
		log.Fatalf("bad --remote: %v", err)
	}
	info, err := peer.AddrInfoFromP2pAddr(maddr)
	if err != nil {
		log.Fatalf("bad --remote: %v", err)
	}

	// Connect
	connectCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	var success bool
	for i := 0; i < 5; i++ {
		if err := node.Host.Connect(connectCtx, *info); err == nil {
			log.Printf("[client] connected to %s", info.ID)
			success = true
			break
		} else {
			log.Printf("connect failed: %v", err)
		}
		time.Sleep(time.Second * 3)
	}
	if !success {
		return
	}

	conn, err := clientRole.OpenStream(ctx, node.Host, info.ID)
	if err != nil {
		log.Fatalf("open stream failed: %+v", err)
	}

	// Throughput test over bridged TCP
	if send {
		util.SendAndMeasureTCP(conn, duration)
	} else {
		util.ReceiveAndMeasureTCP(conn, duration)
	}
}
