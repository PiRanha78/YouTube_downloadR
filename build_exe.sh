#!/usr/bin/env bash
# Önálló Windows .exe: yt-dlp + ffmpeg beágyazva (külön telepítés nélkül)
set -euo pipefail
ROOT="$(cd "$(dirname "$0")" && pwd)"
OUT="$ROOT/release/YouTube_DownloadR"
BUNDLE_DIR="$ROOT/cmd/ytd/bundled"
ZIP="$BUNDLE_DIR/tools.zip"
APP="$ROOT/app"
FFBIN="$APP/ffmpeg-8.0.1-essentials_build/bin"

echo "[1/3] Beágyazandó csomag készítése (yt-dlp + ffmpeg + ffprobe)…"
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
  # zip elérhető macOS-en
  zip -9 -q "$ZIP" yt-dlp.exe ffmpeg.exe ffprobe.exe
)

echo "    tools.zip: $(du -h "$ZIP" | awk '{print $1}')"

echo "[2/3] Go build (windows/amd64, beágyazott csomag)…"
mkdir -p "$OUT"
GOOS=windows GOARCH=amd64 go build -C "$ROOT/cmd/ytd" \
  -ldflags="-s -w -H windowsgui" \
  -o "$OUT/YouTube_DownloadR.exe"

echo "[3/3] Kész csomag (egyetlen .exe, app/ nélkül)…"
# Régi app/ mappa törlése a release-ből, ha volt
rm -rf "$OUT/app"

echo
echo "Kész: $OUT/YouTube_DownloadR.exe"
ls -lh "$OUT/YouTube_DownloadR.exe"
file "$OUT/YouTube_DownloadR.exe"
