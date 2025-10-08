// Copyright 2025 JC-Lab
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-FEL

package relay_client

import (
	"fmt"
	"net"
	"time"

	"github.com/flymesh/core/pkg/pb/relay"
	relay_protocol "github.com/flymesh/core/pkg/relay-protocol"
	"github.com/libp2p/go-libp2p/core/peer"
)

func sendHandshake(conn net.Conn, streamID uint64, token []byte, peerID peer.ID) error {
	req := relaypb.HandshakeRequest{
		StreamId: streamID,
	}
	req.SenderPeerId, _ = peerID.MarshalBinary()
	payload, err := req.MarshalVT()
	if err != nil {
		return fmt.Errorf("marshal handshake: %w", err)
	}
	return relay_protocol.WriteRelayFrame(conn, relay_protocol.RelayTypeHandshakeRequest, token, payload)
}

func readHandshakeAck(conn net.Conn, token []byte) error {
	hdr, data, sum, err := relay_protocol.ReadRelayFrameRaw(conn, time.Second*10)
	if err != nil {
		return fmt.Errorf("read relay-server ack: %w", err)
	}
	if err := hdr.VerifyRelayHMAC(token, data, sum); err != nil {
		return fmt.Errorf("ack hmac: %w", err)
	}
	if hdr.Type != relay_protocol.RelayTypeHandshakeAck {
		return fmt.Errorf("unexpected relay-server type: %d", hdr.Type)
	}
	var ack relaypb.HandshakeAck
	if err := ack.UnmarshalVT(data); err != nil {
		return fmt.Errorf("decode ack: %w", err)
	}
	if !ack.GetOk() {
		return fmt.Errorf("relay-server nack: %s", ack.GetError())
	}
	return nil
}
