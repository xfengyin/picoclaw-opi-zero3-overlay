#!/usr/bin/env bash
# Apply this overlay onto a pristine sipeed/picoclaw checkout at the base commit.
set -euo pipefail

if [ $# -ne 1 ]; then echo "usage: ./apply.sh /path/to/sipeed/picoclaw"; exit 2; fi
TARGET=$1
SOURCE_DIR=$(cd $(dirname "$0") && pwd)

cd "$TARGET"
echo "applying picoclaw-opi-zero3-overlay onto $TARGET"

files=("cmd/opi-info/main.go" "cmd/opi-info/main_test.go" "cmd/picoclaw/internal/agent/opi_zero3.go" "cmd/picoclaw/internal/agent/opi_zero3_stub.go" "deploy/orangepi-zero3/99-picoclaw-led.rules" "deploy/orangepi-zero3/config.opi-zero3.example.json" "deploy/orangepi-zero3/install.sh" "deploy/orangepi-zero3/journald-picoclaw.conf" "deploy/orangepi-zero3/picoclaw.service" "deploy/orangepi-zero3/zramswap.service" "docs/orangepi-zero3.md" "pkg/board/opi_zero3/board.go" "pkg/board/opi_zero3/board_test.go" "pkg/board/opi_zero3/led.go" "pkg/board/opi_zero3/led_test.go" "pkg/board/opi_zero3/notifier.go" "pkg/board/opi_zero3/notifier_test.go" "pkg/board/opi_zero3/thermal.go" "pkg/board/opi_zero3/thermal_test.go" "pkg/gateway/opi_zero3.go" "pkg/gateway/opi_zero3_stub.go")
for f in "${files[@]}"; do
  mkdir -p "$(dirname "$f")"
  cp "$SOURCE_DIR/$f" "$f"
done

git apply "$SOURCE_DIR/hooks.patch"

echo "done. Build with: go build -tags goolm,stdjson,opi_zero3 ./pkg/board/... ./pkg/gateway/... ./cmd/opi-info/..."
