# Copyright 2025 JC-Lab
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-FEL

mkdir -p ./internal/pb/{control,relay,throughtput}
protoc --proto_path=./internal/proto/ \
    --go_out=./internal/pb/control/ --go_opt=paths=source_relative \
    --go-vtproto_out=./internal/pb/control/ --go-vtproto_opt=features=all,paths=source_relative \
    control.proto

protoc --proto_path=./internal/proto/ \
    --go_out=./internal/pb/relay/ --go_opt=paths=source_relative \
    --go-vtproto_out=./internal/pb/relay/ --go-vtproto_opt=features=all,paths=source_relative \
    relay.proto

protoc --proto_path=./internal/proto/ \
    --go_out=./internal/pb/throughtput/ --go_opt=paths=source_relative \
    --go-vtproto_out=./internal/pb/throughtput/ --go-vtproto_opt=features=all,paths=source_relative \
    throughput.proto
