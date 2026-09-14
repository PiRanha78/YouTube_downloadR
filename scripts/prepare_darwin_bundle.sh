#!/usr/bin/env bash
# Beágyazandó macOS (Intel) yt-dlp + statikus ffmpeg/ffprobe letöltése
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
STAGE="$ROOT/tmp_mac_tools"
BUNDLE="$ROOT/cmd/ytd/bundled"
ZIP="$BUNDLE/tools_darwin.zip"

mkdir -p "$STAGE" "$BUNDLE"
rm -rf "$STAGE"/*
cd "$STAGE"

echo "yt-dlp (macOS)…"
curl -fsSL -o yt-dlp "https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp_macos"

echo "ffmpeg / ffprobe (evermeet.cx static, Intel)…"
curl -fsSL -o ffmpeg.zip "https://evermeet.cx/ffmpeg/getrelease/ffmpeg/zip"
curl -fsSL -o ffprobe.zip "https://evermeet.cx/ffmpeg/getrelease/ffprobe/zip"
unzip -o -q ffmpeg.zip
unzip -o -q ffprobe.zip
rm -f ffmpeg.zip ffprobe.zip

chmod +x yt-dlp ffmpeg ffprobe
file yt-dlp ffmpeg ffprobe
./ffmpeg -version | head -1
./yt-dlp --version

rm -f "$ZIP"
zip -9 -q "$ZIP" yt-dlp ffmpeg ffprobe
ls -lh "$ZIP"
echo "Kész: $ZIP"
