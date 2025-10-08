// Copyright 2025 JC-Lab
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-FEL

package relay_client

import (
	"context"
	"fmt"
	"log"
	"time"

	controlpb "github.com/flymesh/core/pkg/pb/control"
	"github.com/flymesh/core/pkg/protocol"
	relay_protocol "github.com/flymesh/core/pkg/relay-protocol"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/sec"
)

type ClientRole struct {
	PrivKey crypto.PrivKey
}

func (r *ClientRole) OpenStream(ctx context.Context, h host.Host, serverPeerId peer.ID) (sec.SecureConn, error) {
	streamInfo, err := r.RequestStream(ctx, h, serverPeerId)
	if err != nil {
		return nil, err
	}
	return DialRelayStream(ctx, r.PrivKey, streamInfo)
}

func (r *ClientRole) RequestStream(ctx context.Context, h host.Host, serverPeerId peer.ID) (*StreamInfo, error) {
	stream, err := h.NewStream(network.WithAllowLimitedConn(ctx, ""), serverPeerId, protocol.ProtoServerStartRelay)
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	// Send empty StartRelayStreamRequest
	if err := relay_protocol.WriteControlFrame(stream, relay_protocol.ControlTypeStartRelayStreamRequest, nil); err != nil {
		return nil, err
	}

	// Read StartRelayStreamResponse
	typ, data, err := relay_protocol.ReadControlFrame(stream, time.Second*10)
	if err != nil {
		return nil, err
	}
	if typ != relay_protocol.ControlTypeStartRelayStreamResponse {
		return nil, fmt.Errorf("unknown type: 0x%x", typ)
	}
	var resp controlpb.StartRelayStreamResponse
	if err := resp.UnmarshalVT(data); err != nil {
		return nil, err
	}
	if !resp.GetOk() {
		return nil, fmt.Errorf("server error: %s", resp.GetError())
	}

	log.Printf("[client] relay-server endpoint: %s, streamID=%d", resp.GetRelayEndpoint(), resp.GetStreamId())

	return &StreamInfo{
		RelayEndpoint: resp.GetRelayEndpoint(),
		StreamID:      resp.GetStreamId(),
		Token:         resp.GetToken(),
		IsServer:      false,
		LocalPeerID:   h.ID(),
		RemotePeerID:  serverPeerId,
	}, nil
}
