#!/bin/bash
# create-dmg.sh — Creates a macOS .dmg with Vortex.app and an Applications symlink.
#
# Usage:
#   ./scripts/create-dmg.sh [--version VERSION] [--app-dir PATH] [--output PATH]
#
# Defaults:
#   VERSION = dev
#   APP_DIR = ./dist/Vortex.app
#   OUTPUT  = ./dist/Vortex-VERSION.dmg

set -euo pipefail

VERSION="dev"
APP_DIR="./dist/Vortex.app"
OUTPUT=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --version)  VERSION="$2"; shift 2 ;;
        --app-dir)  APP_DIR="$2"; shift 2 ;;
        --output)   OUTPUT="$2"; shift 2 ;;
        *) echo "Unknown option: $1" >&2; exit 1 ;;
    esac
done

if [[ -z "$OUTPUT" ]]; then
    OUTPUT="./dist/Vortex-${VERSION}.dmg"
fi

if [[ ! -d "$APP_DIR" ]]; then
    echo "Error: $APP_DIR does not exist. Run create-app-bundle.sh first." >&2
    exit 1
fi

echo "Creating DMG: $OUTPUT (version: $VERSION)"

# Create a temporary directory for the DMG contents.
DMG_STAGING=$(mktemp -d)
trap "rm -rf '$DMG_STAGING'" EXIT

# Copy the app bundle.
cp -R "$APP_DIR" "$DMG_STAGING/Vortex.app"

# Create Applications symlink.
ln -s /Applications "$DMG_STAGING/Applications"

# Create the DMG.
# Use UDZO (zlib) compression for broad compatibility.
mkdir -p "$(dirname "$OUTPUT")"
rm -f "$OUTPUT"

hdiutil create \
    -volname "Vortex" \
    -srcfolder "$DMG_STAGING" \
    -ov \
    -format UDZO \
    -imagekey zlib-level=9 \
    "$OUTPUT"

echo "✓ Created $OUTPUT"
echo ""
echo "To verify: hdiutil verify $OUTPUT"
