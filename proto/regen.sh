#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"

export PATH="$PATH:$(go env GOPATH)/bin"

for dir in core ui dslr; do
    echo "==> proto/$dir"
    protoc \
        --go_out="$ROOT_DIR" \
        --go_opt=paths=source_relative \
        --go-grpc_out="$ROOT_DIR" \
        --go-grpc_opt=paths=source_relative \
        "$SCRIPT_DIR/$dir/$dir.proto"
done
