#!/bin/bash
# create-app-bundle.sh — Assembles the Vortex.app bundle from template files.
#
# Usage:
#   ./scripts/create-app-bundle.sh [--version VERSION] [--output DIR] [--bootstrap BINARY] [--launcher BINARY]
#
# Defaults:
#   VERSION   = dev
#   OUTPUT    = ./dist
#   BOOTSTRAP = ./bin/vortex-bootstrap (if exists)
#   LAUNCHER  = (built from cmd/vortex-launcher if not provided)

set -euo pipefail

VERSION="dev"
OUTPUT="./dist"
BOOTSTRAP=""
LAUNCHER=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --version)  VERSION="$2"; shift 2 ;;
        --output)   OUTPUT="$2"; shift 2 ;;
        --bootstrap) BOOTSTRAP="$2"; shift 2 ;;
        --launcher) LAUNCHER="$2"; shift 2 ;;
        *) echo "Unknown option: $1" >&2; exit 1 ;;
    esac
done

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
PACKAGING_DIR="$PROJECT_ROOT/packaging/macos"

APP_DIR="$OUTPUT/Vortex.app"
CONTENTS="$APP_DIR/Contents"
MACOS_DIR="$CONTENTS/MacOS"
RESOURCES_DIR="$CONTENTS/Resources"

echo "Creating Vortex.app bundle (version: $VERSION)"

# Clean and create structure.
rm -rf "$APP_DIR"
mkdir -p "$MACOS_DIR" "$RESOURCES_DIR"

# Info.plist — substitute version placeholder.
sed "s/__VERSION__/$VERSION/g" "$PACKAGING_DIR/Info.plist" > "$CONTENTS/Info.plist"

# Launcher binary (Mach-O executable required by LaunchServices).
if [[ -n "$LAUNCHER" && -f "$LAUNCHER" ]]; then
    cp "$LAUNCHER" "$MACOS_DIR/vortex-launcher"
else
    # Fallback: use the shell script (works for direct execution, not via `open`).
    cp "$PACKAGING_DIR/vortex-launcher" "$MACOS_DIR/vortex-launcher"
fi
chmod +x "$MACOS_DIR/vortex-launcher"

# Bootstrap binary (if provided).
if [[ -n "$BOOTSTRAP" && -f "$BOOTSTRAP" ]]; then
    cp "$BOOTSTRAP" "$MACOS_DIR/vortex-bootstrap"
    chmod +x "$MACOS_DIR/vortex-bootstrap"
fi

# Icon (if generated).
ICNS="$PROJECT_ROOT/packaging/icons/vortex.icns"
if [[ -f "$ICNS" ]]; then
    cp "$ICNS" "$RESOURCES_DIR/vortex.icns"
else
    echo "  Warning: $ICNS not found — app bundle will have no icon"
fi

echo "✓ Created $APP_DIR"
