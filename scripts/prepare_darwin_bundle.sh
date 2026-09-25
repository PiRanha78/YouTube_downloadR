#!/usr/bin/env bash
# Beágyazandó macOS tool zip-ek: amd64 (Intel) + arm64 (Apple Silicon)
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BUNDLE="$ROOT/cmd/ytd/bundled"
STAGE="$ROOT/tmp_mac_tools"
mkdir -p "$BUNDLE" "$STAGE"

YT_URL="https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp_macos"
FF_STATIC="https://github.com/eugeneware/ffmpeg-static/releases/download/b6.1.1"

prepare_arch() {
  local arch="$1"
  local zipname="tools_darwin_${arch}.zip"
  local dir="$STAGE/$arch"
  rm -rf "$dir"
  mkdir -p "$dir"
  echo "==> $arch"

  echo "  yt-dlp (universal)..."
  curl -fsSL -o "$dir/yt-dlp" "$YT_URL"
  chmod +x "$dir/yt-dlp"

  if [[ "$arch" == "amd64" ]]; then
    echo "  ffmpeg/ffprobe (evermeet.cx static Intel)..."
    curl -fsSL -o "$dir/ffmpeg.zip" "https://evermeet.cx/ffmpeg/getrelease/ffmpeg/zip"
    curl -fsSL -o "$dir/ffprobe.zip" "https://evermeet.cx/ffmpeg/getrelease/ffprobe/zip"
    (cd "$dir" && unzip -o -q ffmpeg.zip && unzip -o -q ffprobe.zip && rm -f ffmpeg.zip ffprobe.zip)
  else
    echo "  ffmpeg/ffprobe (eugeneware static arm64)..."
    curl -fsSL -o "$dir/ffmpeg.gz" "$FF_STATIC/ffmpeg-darwin-arm64.gz"
    curl -fsSL -o "$dir/ffprobe.gz" "$FF_STATIC/ffprobe-darwin-arm64.gz"
    gunzip -f "$dir/ffmpeg.gz"
    gunzip -f "$dir/ffprobe.gz"
    # gunzip megtartja az alapnevet: ffmpeg / ffprobe (ha .gz a kiterjesztés)
    # A letöltött név ffmpeg.gz -> ffmpeg
    if [[ ! -f "$dir/ffmpeg" && -f "$dir/ffmpeg-darwin-arm64" ]]; then
      mv "$dir/ffmpeg-darwin-arm64" "$dir/ffmpeg"
    fi
    if [[ ! -f "$dir/ffprobe" && -f "$dir/ffprobe-darwin-arm64" ]]; then
      mv "$dir/ffprobe-darwin-arm64" "$dir/ffprobe"
    fi
  fi

  echo "  deno (EJS / YouTube challenge)..."
  local deno_zip deno_url
  if [[ "$arch" == "amd64" ]]; then
    deno_url="https://github.com/denoland/deno/releases/latest/download/deno-x86_64-apple-darwin.zip"
  else
    deno_url="https://github.com/denoland/deno/releases/latest/download/deno-aarch64-apple-darwin.zip"
  fi
  deno_zip="$dir/deno.zip"
  curl -fsSL -o "$deno_zip" "$deno_url"
  (cd "$dir" && unzip -o -q deno.zip && rm -f deno.zip)
  chmod +x "$dir/deno"

  chmod +x "$dir/yt-dlp" "$dir/ffmpeg" "$dir/ffprobe" "$dir/deno"
  file "$dir/yt-dlp" "$dir/ffmpeg" "$dir/ffprobe" "$dir/deno"
  # Csak natív archon futtassuk a verzióchecket
  local host
  host="$(uname -m)"
  if { [[ "$arch" == "amd64" && "$host" == "x86_64" ]] || [[ "$arch" == "arm64" && "$host" == "arm64" ]]; }; then
    "$dir/ffmpeg" -version | head -1
    "$dir/yt-dlp" --version
    "$dir/deno" --version | head -1
  else
    echo "  (verziócheck kihagyva - cross-arch)"
  fi

  rm -f "$BUNDLE/$zipname"
  (cd "$dir" && zip -9 -q "$BUNDLE/$zipname" yt-dlp ffmpeg ffprobe deno)
  ls -lh "$BUNDLE/$zipname"
}

if [[ -f "$BUNDLE/tools_darwin.zip" && ! -f "$BUNDLE/tools_darwin_amd64.zip" ]]; then
  mv "$BUNDLE/tools_darwin.zip" "$BUNDLE/tools_darwin_amd64.zip"
fi

ARCH_FILTER="${1:-all}"
if [[ "$ARCH_FILTER" == "all" || "$ARCH_FILTER" == "amd64" ]]; then
  prepare_arch amd64
fi
if [[ "$ARCH_FILTER" == "all" || "$ARCH_FILTER" == "arm64" ]]; then
  prepare_arch arm64
fi

echo "Kész."
