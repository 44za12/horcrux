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
    python3 -c "
from PIL import Image; import os, subprocess
img = Image.open('logo.png')
if img.mode != 'RGBA': img = img.convert('RGBA')
iconset = '/tmp/horcrux.iconset'
os.makedirs(iconset, exist_ok=True)
for name, sz in [('icon_16x16.png',16),('icon_16x16@2x.png',32),('icon_32x32.png',32),('icon_32x32@2x.png',64),('icon_128x128.png',128),('icon_128x128@2x.png',256),('icon_256x256.png',256),('icon_256x256@2x.png',512),('icon_512x512.png',512),('icon_512x512@2x.png',1024)]:
    img.resize((sz,sz),Image.LANCZOS).save(os.path.join(iconset, name))
subprocess.run(['iconutil','-c','icns',iconset,'-o','$SRC_APP/Contents/Resources/iconfile.icns'],check=True)
print('Icon injected')
"
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
