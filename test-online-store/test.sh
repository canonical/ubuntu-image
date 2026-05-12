#!/usr/bin/env bash
# End-to-end test: build the edge-imx93-uc-vtg image from a manifest using m2cp.
# Runs until it works. Failures are signal; iterate the implementation, not the test.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "$HERE/.." && pwd)"

MANIFEST="$HERE/manifest.yaml"
WORK="$HERE/work"
OUT="$HERE/out"

# Prefer the freshly-built m2cp (which has --url) over the system one
# by prepending its build dir to PATH. Drop once the system binary
# catches up.
DEV_M2CP_DIR="/r/M2CP.2/Tools/m2cp/build/linux-amd64"
if [[ -x "$DEV_M2CP_DIR/m2cp" ]]; then
    export PATH="$DEV_M2CP_DIR:$PATH"
fi

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
