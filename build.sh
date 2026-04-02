#!/bin/bash
set -e
GOPRIVATE='github.com/ML-PA-Consulting-GmbH/*' go build -o bin/ubuntu-image ./cmd/ubuntu-image/
