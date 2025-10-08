// Copyright 2025 JC-Lab
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-FEL

package relay_server

import (
	"context"
	"log"
	"time"

	"github.com/flymesh/core/p2p"
	"github.com/flymesh/core/pkg/pb/control"
	"github.com/flymesh/core/pkg/protocol"
	relay_manager "github.com/flymesh/core/pkg/relay-manager"
	relay_protocol "github.com/flymesh/core/pkg/relay-protocol"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/libp2p/go-libp2p/core/network"
)

// Run starts the relay-server mode handlers on the given node.
func Run(ctx context.Context, node *p2p.Node, listen string) {
	// Start TCP RelayManager
	rm := relay_manager.New(listen)
	if err := rm.Start(ctx); err != nil {
		log.Fatalf("relay-server manager start failed: %+v", err)
	}
	log.Printf("RelayManager started on %s", listen)

	// Handle /flymesh/1.0/relay-server/create-stream
	node.Host.SetStreamHandler(protocol.ProtoRelayCreate, func(s network.Stream) {
		defer s.Close()

		remotePeer := s.Conn().RemotePeer()
		log.Printf("[relay-server] create-stream from %s", s.Conn().RemotePeer())

		// Read one control frame (CreateStreamRequest)
		typ, data, err := relay_protocol.ReadControlFrame(s, time.Second*10)
		if err != nil {
			log.Printf("[relay-server] read control frame failed: %v", err)
			return
		}
		if typ != relay_protocol.ControlTypeCreateStreamRequest {
			log.Printf("[relay-server] unexpected type: 0x%04x", typ)
			return
		}
		var req controlpb.CreateStreamRequest
		if data != nil {
			if err := req.UnmarshalVT(data); err != nil {
				log.Printf("[relay-server] bad CreateStreamRequest: %v", err)
				return
			}
		}

		clientPeerId, err := peer.IDFromBytes(req.GetClientPeerId())
		if err != nil {
			log.Printf("[relay-server] invalid peer id: %+v", err)
			return
		}

		streamID, token, tcpEndpoint, err := rm.CreateStream(remotePeer, clientPeerId, time.Minute)
		resp := controlpb.CreateStreamResponse{
			Ok:            err == nil,
			Error:         "",
			StreamId:      streamID,
			Token:         token,
			RelayEndpoint: tcpEndpoint,
		}
		if err != nil {
			resp.Error = err.Error()
		}
		payload, err := resp.MarshalVT()
		if err != nil {
			log.Printf("[relay-server] marshal CreateStreamResponse failed: %v", err)
			return
		}
		if err := relay_protocol.WriteControlFrame(s, relay_protocol.ControlTypeCreateStreamResponse, payload); err != nil {
			log.Printf("[relay-server] write CreateStreamResponse failed: %v", err)
			return
		}
	})
}
