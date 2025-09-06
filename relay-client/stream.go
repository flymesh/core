// Copyright 2025 JC-Lab
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-FEL

package relay_client

import (
	"context"
	"net"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/sec"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
)

type StreamInfo struct {
	RelayEndpoint string
	StreamID      uint64
	Token         []byte
	IsServer      bool
	LocalPeerID   peer.ID
	RemotePeerID  peer.ID
}

type commonRole struct {
	LocalPeerId peer.ID
}

var dialer net.Dialer

func DialRelayStream(ctx context.Context, privateKey crypto.PrivKey, info *StreamInfo) (sec.SecureConn, error) {
	var success bool

	conn, err := dialer.DialContext(ctx, "tcp", info.RelayEndpoint)
	if err != nil {
		return nil, err
	}
	defer func() {
		if !success {
			_ = conn.Close()
		}
	}()

	// send handshake for this data conn as well
	if err := sendHandshake(conn, info.StreamID, info.Token, info.LocalPeerID); err != nil {
		return nil, err
	}

	// read ack
	if err := readHandshakeAck(conn, info.Token); err != nil {
		return nil, err
	}

	tpt, err := noise.New(noise.ID, privateKey, nil)
	if err != nil {
		return nil, err
	}

	var sconn sec.SecureConn
	if info.IsServer {
		sconn, err = tpt.SecureInbound(ctx, conn, info.RemotePeerID)
	} else {
		sconn, err = tpt.SecureOutbound(ctx, conn, info.RemotePeerID)
	}
	success = err == nil
	return sconn, err
}
