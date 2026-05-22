#!/bin/bash
set -e
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo dev)
go build -ldflags "-X main.Version=$VERSION" -o bin/liot-image ./cmd/ubuntu-image/
