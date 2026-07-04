#!/usr/bin/env bash
set -euo pipefail

echo "Running GA guard..."
go test ./pkg/corpus/ -run TestGAGuard -v
echo "GA guard passed — descriptor-only registration needs no core change."
