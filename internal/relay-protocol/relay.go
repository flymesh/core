// Copyright 2025 JC-Lab
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-FEL

package relay_protocol

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

var (
	ErrBadMagic     = errors.New("bad magic")
	ErrBadVersion   = errors.New("bad version")
	ErrHMACMismatch = errors.New("hmac mismatch")
)

// Relay data plane framing:
//
// Magic "FLYR" (4B)
// Length (LE16) -- length of Data only (does NOT include header/HMAC)
// Version (1B) -- fixed 0x01
// Type (1B)
// Data (NB) -- protobuf-encoded payload
// HMAC (32B) -- HMAC-SHA256(key=token, msg = Magic||Length||Version||Type||Data)
//
// Types:
//
//	0x01 HandshakeRequest
//	0x02 HandshakeAck
const (
	relayMagic   = "FLYR"
	relayVersion = byte(0x01)

	RelayTypeHandshakeRequest = byte(0x01)
	RelayTypeHandshakeAck     = byte(0x02)
)

type RelayHeader struct {
	Length  uint16
	Version byte
	Type    byte
}

// WriteRelayFrame writes one relay-server frame with computed HMAC.
// token is required to compute HMAC.
func WriteRelayFrame(w io.Writer, typ byte, token []byte, data []byte) error {
	if len(data) > 0xFFFF {
		return fmt.Errorf("relay-server frame too large: %d", len(data))
	}
	hdr := &RelayHeader{
		Length:  uint16(len(data)),
		Version: relayVersion,
		Type:    typ,
	}
	// RelayHeader
	buf := bytes.NewBuffer(nil)
	buf.Grow(4 + 2 + 1 + 1 + len(data) + 32)
	buf.WriteString(relayMagic)
	var le [2]byte
	binary.LittleEndian.PutUint16(le[:], hdr.Length)
	buf.Write(le[:])
	buf.WriteByte(hdr.Version)
	buf.WriteByte(hdr.Type)
	if len(data) > 0 {
		buf.Write(data)
	}
	h := buildRelayHMAC(token, hdr, data)
	buf.Write(h)

	_, err := w.Write(buf.Bytes())
	return err
}

// ReadRelayFrameRaw reads a relay-server frame and returns header, data, and hmac bytes.
// It does not verify HMAC. Caller must validate using the expected token.
func ReadRelayFrameRaw(r net.Conn, timeout time.Duration) (hdr *RelayHeader, data []byte, hmacSum []byte, err error) {
	var magic [4]byte

	_ = r.SetReadDeadline(time.Now().Add(timeout))
	defer func() {
		_ = r.SetReadDeadline(time.Time{})
	}()

	if _, err = io.ReadFull(r, magic[:]); err != nil {
		return
	}
	if string(magic[:]) != relayMagic {
		err = ErrBadMagic
		return
	}

	hdr = &RelayHeader{}

	var le [2]byte
	if _, err = io.ReadFull(r, le[:]); err != nil {
		return
	}
	hdr.Length = binary.LittleEndian.Uint16(le[:])

	var ver [1]byte
	if _, err = io.ReadFull(r, ver[:]); err != nil {
		return
	}
	hdr.Version = ver[0]
	if hdr.Version != relayVersion {
		err = ErrBadVersion
		return
	}
	var typ [1]byte
	if _, err = io.ReadFull(r, typ[:]); err != nil {
		return
	}
	hdr.Type = typ[0]

	if hdr.Length > 0 {
		data = make([]byte, int(hdr.Length))
		if _, err = io.ReadFull(r, data); err != nil {
			return
		}
	}
	hmacSum = make([]byte, 32)
	if _, err = io.ReadFull(r, hmacSum); err != nil {
		return
	}
	return
}

// VerifyRelayHMAC verifies the relay-server HMAC using token. Returns ErrHMACMismatch if invalid.
func (h *RelayHeader) VerifyRelayHMAC(token []byte, data []byte, got []byte) error {
	want := buildRelayHMAC(token, h, data)
	if !hmac.Equal(want, got) {
		return ErrHMACMismatch
	}
	return nil
}

// buildRelayHMAC computes HMAC per spec using token as key.
func buildRelayHMAC(token []byte, hdr *RelayHeader, data []byte) []byte {
	mac := hmac.New(sha256.New, token)
	// Magic
	mac.Write([]byte(relayMagic))
	// Length LE16
	var le [2]byte
	binary.LittleEndian.PutUint16(le[:], hdr.Length)
	mac.Write(le[:])
	// Version, Type
	mac.Write([]byte{hdr.Version, hdr.Type})
	// Data
	mac.Write(data)
	return mac.Sum(nil)
}
