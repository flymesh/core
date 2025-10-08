// Copyright 2025 JC-Lab
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-FEL

package protocol

const (
	// For server to ask relay-server to create a stream
	ProtoRelayCreate = "/flymesh/1.0/relay-server/create-stream"
	// For client to ask server to start a relay-server stream
	ProtoServerStartRelay = "/flymesh/1.0/server/start-relay-server-stream"
)
