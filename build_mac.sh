#!/usr/bin/env bash
# macOS .app build - amd64|arm64|x86_64|Intel|AppleSilicon
set -euo pipefail
ROOT="$(cd "$(dirname "$0")" && pwd)"
ARCH_IN="${1:-$(uname -m)}"

case "$ARCH_IN" in
  x86_64|amd64|Intel)
    GOARCH=amd64
    OUT_LABEL=Intel
    ZIP_ARCH=amd64
    ;;
  arm64|aarch64|AppleSilicon)
    GOARCH=arm64
    OUT_LABEL=AppleSilicon
    ZIP_ARCH=arm64
    ;;
  *)
    echo "Ismeretlen arch: $ARCH_IN"
    exit 1
    ;;
esac

OUT_DIR="$ROOT/release/macOS_${OUT_LABEL}"
APP_NAME="YouTube_DownloadR"
APP="$OUT_DIR/${APP_NAME}.app"
ZIP="$ROOT/cmd/ytd/bundled/tools_darwin_${ZIP_ARCH}.zip"

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "Csak macOS-en."
  exit 1
fi
if [[ ! -f "$ZIP" ]]; then
  echo "Hiányzik: $ZIP"
  echo "Futtasd: ./scripts/prepare_darwin_bundle.sh ${ZIP_ARCH}"
  exit 1
fi

echo "[1/3] Go build darwin/${GOARCH}..."
mkdir -p "$OUT_DIR"
BIN="$OUT_DIR/${APP_NAME}"

HOST="$(uname -m)"
USE_CGO=0
if [[ "$HOST" == "x86_64" && "${GOARCH}" == "amd64" ]]; then
  USE_CGO=1
fi
if [[ "$HOST" == "arm64" && "${GOARCH}" == "arm64" ]]; then
  USE_CGO=1
fi

if [[ "$USE_CGO" == "1" ]]; then
  CGO_ENABLED=1 GOOS=darwin GOARCH="${GOARCH}" go build -C "$ROOT/cmd/ytd" \
    -ldflags="-s -w" -o "$BIN"
else
  echo "  (cross-compile -> CGO_ENABLED=0)"
  CGO_ENABLED=0 GOOS=darwin GOARCH="${GOARCH}" go build -C "$ROOT/cmd/ytd" \
    -ldflags="-s -w" -o "$BIN"
fi

echo "[2/3] .app csomag..."
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
mv "$BIN" "$APP/Contents/MacOS/${APP_NAME}"
chmod +x "$APP/Contents/MacOS/${APP_NAME}"

cat > "$APP/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleDevelopmentRegion</key>
  <string>hu</string>
  <key>CFBundleExecutable</key>
  <string>${APP_NAME}</string>
  <key>CFBundleIdentifier</key>
  <string>com.piranha78.youtubedownloadr</string>
  <key>CFBundleInfoDictionaryVersion</key>
  <string>6.0</string>
  <key>CFBundleName</key>
  <string>YouTube DownloadR</string>
  <key>CFBundleDisplayName</key>
  <string>YouTube DownloadR</string>
  <key>CFBundlePackageType</key>
  <string>APPL</string>
  <key>CFBundleShortVersionString</key>
  <string>1.3.1</string>
  <key>CFBundleVersion</key>
  <string>1.3.1</string>
  <key>LSMinimumSystemVersion</key>
  <string>10.15</string>
  <key>NSHighResolutionCapable</key>
  <true/>
  <key>NSAppTransportSecurity</key>
  <dict>
    <key>NSAllowsLocalNetworking</key>
    <true/>
  </dict>
</dict>
</plist>
EOF

echo "[3/3] Zip..."
(
  cd "$OUT_DIR"
  rm -f "${APP_NAME}_macOS_${OUT_LABEL}.zip"
  ditto -c -k --sequesterRsrc --keepParent "${APP_NAME}.app" "${APP_NAME}_macOS_${OUT_LABEL}.zip"
)

echo
file "$APP/Contents/MacOS/${APP_NAME}"
ls -lh "$APP/Contents/MacOS/${APP_NAME}" "$OUT_DIR/${APP_NAME}_macOS_${OUT_LABEL}.zip"
echo "Kész: $APP"
