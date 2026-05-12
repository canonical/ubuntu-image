#!/usr/bin/env bash
# End-to-end test: build the edge-imx93-uc-vtg image from a manifest using m2cp.
# Runs until it works. Failures are signal; iterate the implementation, not the test.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "$HERE/.." && pwd)"

MANIFEST="$HERE/manifest.yaml"
WORK="$HERE/work"
OUT="$HERE/out"

rm -rf "$WORK" "$OUT"
mkdir -p "$WORK" "$OUT"

echo "==> Sanity: m2cp session"
m2cp user status | grep -E "Status:|Store:" | head -4

echo "==> Build ubuntu-image"
( cd "$REPO" && go build -o "$HERE/ubuntu-image" ./cmd/ubuntu-image )

echo "==> Run ubuntu-image with manifest"
"$HERE/ubuntu-image" snap \
    --manifest "$MANIFEST" \
    --workdir "$WORK" \
    -O "$OUT"

echo "==> Output"
ls -la "$OUT"
if [[ -f "$OUT/seed.manifest" ]]; then
    echo "==> seed.manifest:"
    cat "$OUT/seed.manifest"
fi
