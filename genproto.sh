# Copyright 2025 JC-Lab
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-FEL

mkdir -p ./pkg/pb/{control,relay,throughtput}
protoc --proto_path=./proto/ \
    --go_out=./pkg/pb/control/ --go_opt=paths=source_relative \
    --go-vtproto_out=./pkg/pb/control/ --go-vtproto_opt=features=all,paths=source_relative \
    control.proto

protoc --proto_path=./proto/ \
    --go_out=./pkg/pb/relay/ --go_opt=paths=source_relative \
    --go-vtproto_out=./pkg/pb/relay/ --go-vtproto_opt=features=all,paths=source_relative \
    relay.proto

protoc --proto_path=./proto/ \
    --go_out=./pkg/pb/throughtput/ --go_opt=paths=source_relative \
    --go-vtproto_out=./pkg/pb/throughtput/ --go-vtproto_opt=features=all,paths=source_relative \
    throughput.proto
