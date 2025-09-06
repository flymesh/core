// Copyright 2025 JC-Lab
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-FEL

package util

import (
	"crypto/rand"
	"errors"
	"os"

	"fmt"
	"io"
	"log"
	"net"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
)

// LoadOrCreatePrivateKey loads a private key from a file, or creates a new one if it doesn't exist.
func LoadOrCreatePrivateKey(path string) (crypto.PrivKey, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		privKey, _, err := crypto.GenerateEd25519Key(rand.Reader)
		if err != nil {
			return nil, err
		}
		data, err = crypto.MarshalPrivateKey(privKey)
		if err != nil {
			return nil, err
		}
		return privKey, os.WriteFile(path, data, 0600)
	} else if err != nil {
		return nil, err
	}
	priv, err := crypto.UnmarshalPrivateKey(data)
	if err != nil {
		return nil, err
	}
	return priv, nil
}

// TCP throughput test (sender)
func SendAndMeasureTCP(conn net.Conn, duration int) {
	buf := make([]byte, 64*1024)
	_, _ = rand.Read(buf)
	log.Printf("Starting to send data for %d seconds over TCP relay-server...", duration)
	deadline := time.Now().Add(time.Duration(duration) * time.Second)
	total := 0
	for time.Now().Before(deadline) {
		n, err := conn.Write(buf)
		if err != nil {
			log.Printf("Write error: %v", err)
			break
		}
		total += n
	}
	throughput := float64(total) / float64(duration) / (1024 * 1024)
	fmt.Printf("✅ Sent %d bytes in %ds (%.2f MB/s)\n", total, duration, throughput)
}

// TCP throughput test (receiver)
func ReceiveAndMeasureTCP(conn net.Conn, duration int) {
	buf := make([]byte, 64*1024)
	deadline := time.Now().Add(time.Duration(duration) * time.Second)
	total := 0
	log.Printf("Starting to receive data for %d seconds over TCP relay-server...", duration)
	for {
		if time.Now().After(deadline) {
			break
		}
		_ = conn.SetReadDeadline(time.Now().Add(time.Second))
		n, err := conn.Read(buf)
		if err != nil {
			if IsTimeout(err) {
				continue
			}
			if err == io.EOF {
				break
			}
			log.Printf("Read error: %v", err)
			break
		}
		total += n
	}
	throughput := float64(total) / float64(duration) / (1024 * 1024)
	fmt.Printf("✅ Received %d bytes in %ds (%.2f MB/s)\n", total, duration, throughput)
}

func IsTimeout(err error) bool {
	ne, ok := err.(net.Error)
	return ok && ne.Timeout()
}
