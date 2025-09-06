// Copyright 2025 JC-Lab
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-FEL

package relay_manager

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/flymesh/core/internal/pb/relay"
	relay_protocol "github.com/flymesh/core/internal/relay-protocol"
	"github.com/libp2p/go-libp2p/core/peer"

	"google.golang.org/protobuf/proto"
)

var (
	ErrAllocationNotFound = errors.New("allocation not found")
	ErrBadPeer            = errors.New("bad peer")
)

type allocation struct {
	streamID     uint64
	token        []byte // 32 bytes
	serverPeerID peer.ID
	clientPeerID peer.ID

	// connection sides
	mu      sync.Mutex
	sideS   net.Conn
	sideC   net.Conn
	created time.Time
	ttl     time.Duration
}

func (a *allocation) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.sideS != nil {
		_ = a.sideS.Close()
	}

	if a.sideC != nil {
		_ = a.sideC.Close()
	}

	return nil
}

type RelayManager struct {
	listenAddr string

	mu          sync.Mutex
	allocations map[uint64]*allocation
	wg          sync.WaitGroup
	lis         net.Listener
	ctx         context.Context
	cancel      context.CancelFunc
}

// New constructs a manager listening on listenAddr (e.g. ":24002")
func New(listenAddr string) *RelayManager {
	return &RelayManager{
		listenAddr:  listenAddr,
		allocations: make(map[uint64]*allocation),
	}
}

// Start begins accepting TCP connections and handling handshakes.
func (m *RelayManager) Start(ctx context.Context) error {
	if m.cancel != nil {
		return errors.New("already started")
	}
	m.ctx, m.cancel = context.WithCancel(ctx)
	ln, err := net.Listen("tcp", m.listenAddr)
	if err != nil {
		return err
	}
	m.lis = ln
	log.Printf("[relay-server] listening on %s", ln.Addr().String())

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.acceptLoop()
	}()
	// GC loop for TTL
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		t := time.NewTicker(time.Second * 5)
		defer t.Stop()
		for {
			select {
			case <-m.ctx.Done():
				return
			case <-t.C:
				m.gc()
			}
		}
	}()
	return nil
}

// Stop gracefully stops the server.
func (m *RelayManager) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
	if m.lis != nil {
		_ = m.lis.Close()
	}
	m.wg.Wait()
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range m.allocations {
		_ = a.Close()
	}
	m.allocations = make(map[uint64]*allocation)
}

// CreateStream allocates a new stream with TTL and returns (streamID, token, tcpEndpoint)
func (m *RelayManager) CreateStream(serverPeerID peer.ID, clientPeerID peer.ID, ttl time.Duration) (uint64, []byte, string, error) {
	streamID := randomUint64()
	token := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, token); err != nil {
		return 0, nil, "", err
	}

	a := &allocation{
		streamID:     streamID,
		token:        token,
		serverPeerID: serverPeerID,
		clientPeerID: clientPeerID,
		created:      time.Now(),
		ttl:          ttl,
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.allocations[streamID] = a

	return streamID, token, m.listenAddr, nil
}

// acceptLoop handles incoming TCP connections and handshake frames.
func (m *RelayManager) acceptLoop() {
	for {
		conn, err := m.lis.Accept()
		if err != nil {
			select {
			case <-m.ctx.Done():
				return
			default:
			}
			log.Printf("[relay-server] accept error: %v", err)
			continue
		}
		m.wg.Add(1)
		go func(c net.Conn) {
			defer m.wg.Done()
			if err := m.handleConn(c); err != nil {
				log.Printf("[relay-server] conn error: %v", err)
				_ = c.Close()
			}
		}(conn)
	}
}

func (m *RelayManager) handleConn(c net.Conn) error {
	// Read one relay-server frame (HandshakeRequest) + verify HMAC
	hdr, data, sum, err := relay_protocol.ReadRelayFrameRaw(c, time.Second*10)
	if err != nil {
		return fmt.Errorf("read relay-server frame: %w", err)
	}
	if hdr.Type != relay_protocol.RelayTypeHandshakeRequest {
		return fmt.Errorf("unexpected relay-server frame type: %d", hdr.Type)
	}
	var req relaypb.HandshakeRequest
	if err := proto.Unmarshal(data, &req); err != nil {
		return fmt.Errorf("bad handshake payload: %w", err)
	}

	m.mu.Lock()
	a := m.allocations[req.StreamId]
	m.mu.Unlock()
	if a == nil {
		// Ack false
		ack := &relaypb.HandshakeAck{Ok: false, Error: "no such stream"}
		ackBytes, _ := proto.Marshal(ack)
		_ = relay_protocol.WriteRelayFrame(c, relay_protocol.RelayTypeHandshakeAck, make([]byte, 32), ackBytes) // bogus token; conn will close
		return ErrAllocationNotFound
	}

	// Verify HMAC with token
	if err := hdr.VerifyRelayHMAC(a.token, data, sum); err != nil {
		ack := &relaypb.HandshakeAck{Ok: false, Error: "hmac mismatch"}
		ackBytes, _ := proto.Marshal(ack)
		_ = relay_protocol.WriteRelayFrame(c, relay_protocol.RelayTypeHandshakeAck, a.token, ackBytes)
		return err
	}

	senderPeerId, err := peer.IDFromBytes(req.SenderPeerId)
	if err != nil {
		return err
	}
	isServerPeer := a.serverPeerID == senderPeerId
	isClientPeer := a.clientPeerID == senderPeerId

	if !isServerPeer && !isClientPeer {
		log.Printf("[relay-server] warning: sender_peer_id mismatch alloc=(%s, %s) got=%s", a.serverPeerID.String(), a.clientPeerID.String(), senderPeerId.String())
		return ErrBadPeer
	}

	// Ack OK
	ack := &relaypb.HandshakeAck{Ok: true}
	ackBytes, _ := proto.Marshal(ack)
	if err := relay_protocol.WriteRelayFrame(c, relay_protocol.RelayTypeHandshakeAck, a.token, ackBytes); err != nil {
		return fmt.Errorf("write ack: %w", err)
	}

	// Store connection and attempt to bridge
	a.mu.Lock()
	defer a.mu.Unlock()
	if isServerPeer {
		if a.sideS != nil {
			return errors.New("server already bridged")
		}
		a.sideS = c
	} else {
		if a.sideC != nil {
			return errors.New("client already bridged")
		}
		a.sideC = c
	}

	if a.sideS != nil && a.sideC != nil {
		// Bridge and remove allocation when both sides finish.
		go m.startBridge(req.StreamId, a)
	}

	return nil
}

// startBridge runs bidirectional piping and removes the allocation after both directions finish.
func (m *RelayManager) startBridge(id uint64, a *allocation) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		defer a.Close()
		_, _ = io.Copy(a.sideS, a.sideC)
	}()
	go func() {
		defer wg.Done()
		defer a.Close()
		_, _ = io.Copy(a.sideC, a.sideS)
	}()
	wg.Wait()

	// remove allocation after bridge ends
	m.mu.Lock()
	delete(m.allocations, id)
	m.mu.Unlock()
}

// gc removes expired allocations (TTL since creation).
// IMPORTANT: Do NOT close bridged connections during GC.
// Only clean up unbridged (Allocated/HalfConnected) entries when TTL expires.
func (m *RelayManager) gc() {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, a := range m.allocations {
		if now.Sub(a.created) > a.ttl {
			// If not fully bridged, close any half-connected sides and delete.
			if a.sideS == nil || a.sideC == nil {
				a.Close()
				delete(m.allocations, id)
			}
			// If fully bridged (both sides present), keep the allocation as-is.
			// The bridge will close itself when either side ends, or on Stop().
		}
	}
}

// randomUint64 returns a random uint64 using crypto/rand.
func randomUint64() uint64 {
	var b [8]byte
	_, _ = io.ReadFull(rand.Reader, b[:])
	return binary.LittleEndian.Uint64(b[:])
}
