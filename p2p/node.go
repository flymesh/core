// Copyright 2025 JC-Lab
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-FEL

package p2p

import (
	"context"
	crand "crypto/rand"
	"fmt"
	"math/rand"
	"time"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/backoff"
	"github.com/libp2p/go-libp2p/p2p/host/autorelay"
	routedhost "github.com/libp2p/go-libp2p/p2p/host/routed"
	"github.com/libp2p/go-libp2p/p2p/protocol/holepunch"
	"github.com/libp2p/go-libp2p/p2p/protocol/ping"
	ma "github.com/multiformats/go-multiaddr"
)

type Node struct {
	Context        context.Context
	PrivKey        crypto.PrivKey
	Host           host.Host
	BootstrapPeers []peer.AddrInfo
	DHT            *dht.IpfsDHT
	PingService    *ping.PingService

	// ListenPort controls libp2p listen port for both TCP and QUIC (UDP).
	// If 0, libp2p.DefaultListenAddrs are used.
	ListenPort int

	Libp2pOptions []libp2p.Option

	ctx      context.Context
	cancel   context.CancelFunc
	peerChan chan peer.AddrInfo
}

func (n *Node) Init() error {
	var err error

	if n.Context == nil {
		n.Context = context.Background()
	}
	n.ctx, n.cancel = context.WithCancel(n.Context)

	if n.PrivKey == nil {
		n.PrivKey, _, err = crypto.GenerateEd25519Key(crand.Reader)
		if err != nil {
			return err
		}
	}

	bootstrapPeers := n.BootstrapPeers
	if len(bootstrapPeers) == 0 {
		bootstrapPeers, err = DefaultBootstrapPeers()
		if err != nil {
			return err
		}
	}

	peerChan := make(chan peer.AddrInfo)
	n.peerChan = peerChan

	opts := []libp2p.Option{
		libp2p.Identity(n.PrivKey),
		libp2p.UserAgent("p2ptest"),
		libp2p.DefaultTransports,
		libp2p.DefaultSecurity,
		libp2p.DefaultMuxers,
		libp2p.NATPortMap(),
		//libp2p.EnableRelayService(relay.WithLimit(nil), relay.WithACL(acl)),
		//libp2p.EnableNATService(),
		libp2p.EnableAutoNATv2(),
		libp2p.EnableAutoRelayWithPeerSource(
			func(ctx context.Context, numPeers int) <-chan peer.AddrInfo {
				r := make(chan peer.AddrInfo)
				go func() {
					defer close(r)
					for ; numPeers != 0; numPeers-- {
						select {
						case v, ok := <-peerChan:
							if !ok {
								return
							}
							select {
							case r <- v:
							case <-ctx.Done():
								return
							}
						case <-ctx.Done():
							return
						}
					}
				}()
				return r
			},
			autorelay.WithNumRelays(2),
			autorelay.WithBootDelay(5*time.Second),
		),
		libp2p.EnableHolePunching(
			holepunch.WithTracer(&simpleTracer{}),
		),
		libp2p.ForceReachabilityPrivate(),
		//libp2p.WithDialTimeout(time.Second*10),
		libp2p.FallbackDefaults,
	}
	opts = append(opts, n.Libp2pOptions...)

	// Listen addrs: if ListenPort specified, use the same port for TCP and QUIC (UDP)
	if n.ListenPort > 0 {
		addrs := []string{
			fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", n.ListenPort),
			fmt.Sprintf("/ip6/::/tcp/%d", n.ListenPort),
			fmt.Sprintf("/ip4/0.0.0.0/udp/%d/quic-v1", n.ListenPort),
			fmt.Sprintf("/ip6/::/udp/%d/quic-v1", n.ListenPort),
		}
		opts = append(opts, libp2p.ListenAddrStrings(addrs...))
	} else {
		opts = append(opts, libp2p.DefaultListenAddrs)
	}

	basicHost, err := libp2p.New(opts...)
	if err != nil {
		return err
	}

	if n.DHT == nil {
		ddht, err := dht.New(
			n.ctx,
			basicHost,
			dht.Mode(dht.ModeClient),
			dht.BootstrapPeers(bootstrapPeers...),
		)
		if err != nil {
			return err
		}
		n.DHT = ddht
	}

	n.Host = routedhost.Wrap(basicHost, n.DHT)

	n.PingService = ping.NewPingService(n.Host)

	// Continuously feed peers into the AutoRelay service
	go n.autoRelayFeeder(peerChan)

	return nil
}

func (n *Node) autoRelayFeeder(peerChan chan peer.AddrInfo) {
	delay := backoff.NewExponentialDecorrelatedJitter(time.Second, time.Second*60, 5.0, rand.NewSource(time.Now().UnixMilli()))()
	for {
		for _, p := range n.Host.Network().Peers() {
			pi := n.Host.Network().Peerstore().PeerInfo(p)
			relayCount := 0
			for _, la := range n.Host.Network().ListenAddresses() {
				for _, proto := range la.Protocols() {
					if proto.Code == ma.P_CIRCUIT {
						relayCount = relayCount + 1
						break
					}
				}
			}
			if relayCount < 2 {
				peerChan <- pi
			}
		}
		time.Sleep(delay.Delay())
	}
}

type simpleTracer struct {
}

func (s *simpleTracer) Trace(evt *holepunch.Event) {
	//log.Printf("HOLEPUNCH EVENT: %+v\n", evt)
}
