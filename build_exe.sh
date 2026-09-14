#!/usr/bin/env bash
# Önálló Windows .exe: yt-dlp + ffmpeg beágyazva
set -euo pipefail
ROOT="$(cd "$(dirname "$0")" && pwd)"
OUT="$ROOT/release/YouTube_DownloadR"
BUNDLE_DIR="$ROOT/cmd/ytd/bundled"
ZIP="$BUNDLE_DIR/tools_windows.zip"
APP="$ROOT/app"
FFBIN="$APP/ffmpeg-8.0.1-essentials_build/bin"

echo "[1/3] Beágyazandó Windows csomag…"
mkdir -p "$BUNDLE_DIR"
STAGE="$(mktemp -d)"
cleanup() { rm -rf "$STAGE"; }
trap cleanup EXIT

cp -f "$APP/yt-dlp.exe" "$STAGE/yt-dlp.exe"
cp -f "$FFBIN/ffmpeg.exe" "$STAGE/ffmpeg.exe"
cp -f "$FFBIN/ffprobe.exe" "$STAGE/ffprobe.exe"

rm -f "$ZIP"
(
  cd "$STAGE"
  zip -9 -q "$ZIP" yt-dlp.exe ffmpeg.exe ffprobe.exe
)
echo "    tools_windows.zip: $(du -h "$ZIP" | awk '{print $1}')"

echo "[2/3] Go build (windows/amd64)…"
mkdir -p "$OUT"
# Windows buildhez kell egy placeholder darwin zip is? Nem – build tags.
# De go list a darwin fájlt nem fordítja. OK.
# Ha tools_darwin.zip hiányzik, a windows build mégis OK, mert //go:build darwin.
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -C "$ROOT/cmd/ytd" \
  -ldflags="-s -w -H windowsgui" \
  -o "$OUT/YouTube_DownloadR.exe"

echo "[3/3] Kész:"
ls -lh "$OUT/YouTube_DownloadR.exe"
file "$OUT/YouTube_DownloadR.exe"
