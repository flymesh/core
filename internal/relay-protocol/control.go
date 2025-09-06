// Copyright 2025 JC-Lab
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-FEL

package relay_protocol

import (
	"encoding/binary"
	"io"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/pkg/errors"
)

// Control framing: Length (LE16) + Type (LE16) + Data(Protobuf)
// We will carry Data as raw protobuf-encoded bytes prepared by caller.

var (
	ErrTooLarge          = errors.New("payload too large")
	ErrShortRead         = errors.New("short read")
	ErrUnsupportedLength = errors.New("unsupported length")
	ErrUnknownType       = errors.New("unsupported type")
)

const (
	ControlTypeStartRelayStreamRequest  uint16 = 0x0101
	ControlTypeStartRelayStreamResponse uint16 = 0x0102
	ControlTypeCreateStreamRequest      uint16 = 0x0201
	ControlTypeCreateStreamResponse     uint16 = 0x0202
)

// WriteControlFrame writes LE16 length + LE16 type + data to w.
// data must be the protobuf-encoded payload for the control message.
func WriteControlFrame(w io.Writer, typ uint16, data []byte) error {
	if len(data) > 0xFFFF {
		return ErrTooLarge
	}
	var hdr [4]byte
	binary.LittleEndian.PutUint16(hdr[0:2], uint16(len(data)))
	binary.LittleEndian.PutUint16(hdr[2:4], typ)
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	if len(data) > 0 {
		_, err := w.Write(data)
		return err
	}
	return nil
}

// ReadControlFrame reads one control frame and returns type and data bytes.
func ReadControlFrame(r network.Stream, timeout time.Duration) (typ uint16, data []byte, err error) {
	var hdr [4]byte

	_ = r.SetReadDeadline(time.Now().Add(timeout))
	defer func() {
		_ = r.SetReadDeadline(time.Time{})
	}()

	if _, err = io.ReadFull(r, hdr[:]); err != nil {
		return 0, nil, err
	}
	length := binary.LittleEndian.Uint16(hdr[0:2])
	typ = binary.LittleEndian.Uint16(hdr[2:4])
	if length == 0 {
		return typ, nil, nil
	}
	data = make([]byte, int(length))
	_, err = io.ReadFull(r, data)
	return typ, data, err
}
