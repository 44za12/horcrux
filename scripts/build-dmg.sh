#!/bin/bash
set -euo pipefail

APP_NAME="Horcrux"
DMG_NAME="Horcrux-Installer"
SRC_APP="gui/build/bin/Horcrux.app"
DMG_DIR="dist"
STAGING="dist/staging"

if [ ! -d "$SRC_APP" ]; then
    echo "Building $APP_NAME..."
    cd gui && rm -rf build && CGO_ENABLED=1 wails build -skipbindings
    cd ..
fi

if [ -f "logo.png" ]; then
    echo "Injecting icon from logo.png..."
    ICONSET="/tmp/horcrux.iconset"
    rm -rf "$ICONSET"
    mkdir -p "$ICONSET"

    sips -z 16 16   logo.png --out "$ICONSET/icon_16x16.png"
    sips -z 32 32   logo.png --out "$ICONSET/icon_16x16@2x.png"
    sips -z 32 32   logo.png --out "$ICONSET/icon_32x32.png"
    sips -z 64 64   logo.png --out "$ICONSET/icon_32x32@2x.png"
    sips -z 128 128 logo.png --out "$ICONSET/icon_128x128.png"
    sips -z 256 256 logo.png --out "$ICONSET/icon_128x128@2x.png"
    sips -z 256 256 logo.png --out "$ICONSET/icon_256x256.png"
    sips -z 512 512 logo.png --out "$ICONSET/icon_256x256@2x.png"
    sips -z 512 512 logo.png --out "$ICONSET/icon_512x512.png"
    sips -z 1024 1024 logo.png --out "$ICONSET/icon_512x512@2x.png"

    iconutil -c icns "$ICONSET" -o "$SRC_APP/Contents/Resources/iconfile.icns"
    rm -rf "$ICONSET"
    echo "Icon injected"
fi

rm -rf "$DMG_DIR"
mkdir -p "$STAGING"

cp -R "$SRC_APP" "$STAGING/$APP_NAME.app"
ln -s /Applications "$STAGING/Applications"

dot_clean -m "$STAGING/$APP_NAME.app"
xattr -cr "$STAGING/$APP_NAME.app"
codesign --force --deep --sign - "$STAGING/$APP_NAME.app"

echo "Creating DMG..."

hdiutil create -volname "$APP_NAME" \
    -srcfolder "$STAGING" \
    -ov -format UDZO \
    "$DMG_DIR/$DMG_NAME.dmg"

rm -rf "$STAGING"

echo "Created $DMG_DIR/$DMG_NAME.dmg"
ls -lh "$DMG_DIR/$DMG_NAME.dmg"
