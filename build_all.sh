#!/usr/bin/env bash
# Összes platform release artifact
set -euo pipefail
ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT"

echo "===== Darwin tool bundle-ök ====="
./scripts/prepare_darwin_bundle.sh all

echo "===== Windows .exe ====="
./build_exe.sh

echo "===== macOS Intel ====="
./build_mac.sh amd64

echo "===== macOS Apple Silicon ====="
./build_mac.sh arm64

echo
echo "===== Kész fájlok ====="
ls -lh release/YouTube_DownloadR/YouTube_DownloadR.exe \
  release/macOS_Intel/YouTube_DownloadR_macOS_Intel.zip \
  release/macOS_AppleSilicon/YouTube_DownloadR_macOS_AppleSilicon.zip
