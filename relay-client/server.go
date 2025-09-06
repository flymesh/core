// Copyright 2025 JC-Lab
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-FEL

package relay_client

import (
	"context"
	"fmt"
	"log"
	"time"

	controlpb "github.com/flymesh/core/internal/pb/control"
	"github.com/flymesh/core/internal/protocol"
	relay_protocol "github.com/flymesh/core/internal/relay-protocol"
	"github.com/flymesh/core/internal/util"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

type ServerRole struct {
	PrivKey     crypto.PrivKey
	RelayPeerId peer.ID
}

func (r *ServerRole) CreateStream(ctx context.Context, h host.Host, relayPeerId peer.ID, clientPeerId peer.ID) (*StreamInfo, error) {
	stream, err := h.NewStream(network.WithAllowLimitedConn(ctx, ""), relayPeerId, protocol.ProtoRelayCreate)
	if err != nil {
		return nil, fmt.Errorf("open relay-server create-stream: %w", err)
	}
	defer stream.Close()

	req := controlpb.CreateStreamRequest{}
	req.ClientPeerId, err = clientPeerId.Marshal()

	payload, err := req.MarshalVT()
	if err != nil {
		return nil, fmt.Errorf("marshal CreateStreamRequest: %w", err)
	}
	if err := relay_protocol.WriteControlFrame(stream, relay_protocol.ControlTypeCreateStreamRequest, payload); err != nil {
		return nil, fmt.Errorf("write CreateStreamRequest: %w", err)
	}

	typ, data, err := relay_protocol.ReadControlFrame(stream, time.Second*10)
	if err != nil {
		return nil, fmt.Errorf("read CreateStreamResponse: %w", err)
	}
	if typ != relay_protocol.ControlTypeCreateStreamResponse {
		return nil, fmt.Errorf("unexpected type 0x%04x", typ)
	}
	var resp controlpb.CreateStreamResponse
	if err := resp.UnmarshalVT(data); err != nil {
		return nil, fmt.Errorf("decode CreateStreamResponse: %w", err)
	}
	if !resp.GetOk() {
		return nil, fmt.Errorf("relay-server error: %s", resp.GetError())
	}

	log.Printf("[server] relay-server endpoint: %s, streamID=%d", resp.GetRelayEndpoint(), resp.GetStreamId())

	return &StreamInfo{
		RelayEndpoint: resp.GetRelayEndpoint(),
		StreamID:      resp.GetStreamId(),
		Token:         resp.GetToken(),
		IsServer:      true,
		LocalPeerID:   h.ID(),
		RemotePeerID:  clientPeerId,
	}, nil
}

func (r *ServerRole) RegisterProtocol(h host.Host) {
	h.SetStreamHandler(protocol.ProtoServerStartRelay, func(stream network.Stream) {
		r.HandleStartRelay(h, stream)
	})
}

func (r *ServerRole) HandleStartRelay(h host.Host, s network.Stream) {
	defer s.Close()
	clientPeerID := s.Conn().RemotePeer()

	ctx := context.Background()

	log.Printf("[server] start-relay-server-stream from %s", clientPeerID)

	typ, _, err := relay_protocol.ReadControlFrame(s, time.Second*10)
	if err != nil {
		log.Printf("[server] read StartRelayStreamRequest failed: %v", err)
		return
	}
	if typ != relay_protocol.ControlTypeStartRelayStreamRequest {
		log.Printf("[server] unexpected type: 0x%04x", typ)
		return
	}

	streamInfo, err := r.CreateStream(ctx, h, r.RelayPeerId, clientPeerID)
	if err != nil {
		log.Printf("[server] create stream failed: %v", err)
		return
	}

	go func() {
		conn, err := DialRelayStream(ctx, r.PrivKey, streamInfo)
		if err != nil {
			log.Printf("[server] Stream[%d] dial relay failed: %+v", streamInfo.StreamID, err)
			return
		}
		defer conn.Close()

		util.ReceiveAndMeasureTCP(conn, 10)
	}()

	// Return StartRelayStreamResponse to the client
	_ = writeStartRelayResponse(s, true, "", streamInfo)
}

func writeStartRelayResponse(s network.Stream, ok bool, errStr string, streamInfo *StreamInfo) error {
	resp := controlpb.StartRelayStreamResponse{
		Ok:            ok,
		Error:         errStr,
		RelayEndpoint: streamInfo.RelayEndpoint,
		StreamId:      streamInfo.StreamID,
		Token:         streamInfo.Token,
	}
	payload, err := resp.MarshalVT()
	if err != nil {
		return err
	}
	if err := relay_protocol.WriteControlFrame(s, relay_protocol.ControlTypeStartRelayStreamResponse, payload); err != nil {
		return err
	}

	return nil
}
