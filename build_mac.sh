#!/usr/bin/env bash
# macOS Intel (.app) build – yt-dlp + ffmpeg beágyazva
set -euo pipefail
ROOT="$(cd "$(dirname "$0")" && pwd)"
OUT_DIR="$ROOT/release/macOS"
APP_NAME="YouTube_DownloadR"
APP="$OUT_DIR/${APP_NAME}.app"
BUNDLE_DIR="$ROOT/cmd/ytd/bundled"
ZIP="$BUNDLE_DIR/tools_darwin.zip"

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "Ez a script csak macOS-en futtatható."
  exit 1
fi

if [[ ! -f "$ZIP" ]]; then
  echo "Hiányzik: $ZIP"
  echo "Futtasd előbb: ./scripts/prepare_darwin_bundle.sh"
  exit 1
fi

echo "[1/3] Go build (darwin/amd64, WebKit)…"
mkdir -p "$OUT_DIR"
BIN="$OUT_DIR/${APP_NAME}"
CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 go build -C "$ROOT/cmd/ytd" \
  -ldflags="-s -w" \
  -o "$BIN"

echo "[2/3] .app csomag összeállítása…"
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
  <string>1.1.0</string>
  <key>CFBundleVersion</key>
  <string>1.1.0</string>
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

echo "[3/3] Kész:"
ls -lh "$APP/Contents/MacOS/${APP_NAME}"
file "$APP/Contents/MacOS/${APP_NAME}"
echo
echo "App: $APP"
echo "Futtatás: open \"$APP\""
