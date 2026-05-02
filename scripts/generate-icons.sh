#!/bin/bash
# generate-icons.sh — Generate macOS .icns and Windows .ico from the SVG source.
#
# Requirements:
#   - rsvg-convert (from librsvg: brew install librsvg)
#   - iconutil (macOS built-in)
#   - For .ico: ImageMagick (brew install imagemagick) OR png2ico
#
# Usage:
#   ./scripts/generate-icons.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
SVG="$PROJECT_ROOT/assets/icon.svg"
OUT_DIR="$PROJECT_ROOT/packaging/icons"
ICONSET_DIR="$OUT_DIR/vortex.iconset"

if [[ ! -f "$SVG" ]]; then
    echo "Error: $SVG not found" >&2
    exit 1
fi

# Check for rsvg-convert.
if ! command -v rsvg-convert &>/dev/null; then
    echo "Error: rsvg-convert not found. Install with: brew install librsvg" >&2
    exit 1
fi

mkdir -p "$OUT_DIR" "$ICONSET_DIR"

echo "Generating icon PNGs from $SVG"

# macOS iconset requires specific filenames and sizes.
declare -a SIZES=(16 32 128 256 512)
for size in "${SIZES[@]}"; do
    rsvg-convert -w "$size" -h "$size" "$SVG" -o "$ICONSET_DIR/icon_${size}x${size}.png"
    # Retina (@2x) versions: the previous standard size at 2x resolution.
    double=$((size * 2))
    if [[ $double -le 1024 ]]; then
        rsvg-convert -w "$double" -h "$double" "$SVG" -o "$ICONSET_DIR/icon_${size}x${size}@2x.png"
    fi
done

echo "  Generated iconset PNGs"

# Generate .icns using iconutil (macOS only).
if command -v iconutil &>/dev/null; then
    iconutil --convert icns --output "$OUT_DIR/vortex.icns" "$ICONSET_DIR"
    echo "✓ Generated $OUT_DIR/vortex.icns"
else
    echo "  Warning: iconutil not available (not on macOS?) — skipping .icns"
fi

# Generate .ico using ImageMagick convert.
if command -v magick &>/dev/null; then
    # Windows .ico with standard sizes: 16, 32, 48, 256
    ICON_PNGS=()
    for size in 16 32 48 256; do
        png="$OUT_DIR/icon_${size}.png"
        rsvg-convert -w "$size" -h "$size" "$SVG" -o "$png"
        ICON_PNGS+=("$png")
    done
    magick "${ICON_PNGS[@]}" "$OUT_DIR/vortex.ico"
    # Clean up temp PNGs.
    rm -f "${ICON_PNGS[@]}"
    echo "✓ Generated $OUT_DIR/vortex.ico"
elif command -v convert &>/dev/null; then
    # Older ImageMagick (convert command).
    ICON_PNGS=()
    for size in 16 32 48 256; do
        png="$OUT_DIR/icon_${size}.png"
        rsvg-convert -w "$size" -h "$size" "$SVG" -o "$png"
        ICON_PNGS+=("$png")
    done
    convert "${ICON_PNGS[@]}" "$OUT_DIR/vortex.ico"
    rm -f "${ICON_PNGS[@]}"
    echo "✓ Generated $OUT_DIR/vortex.ico"
else
    echo "  Warning: ImageMagick not found — skipping .ico generation"
    echo "  Install with: brew install imagemagick"
fi

# Clean up iconset directory (the .icns has been created).
rm -rf "$ICONSET_DIR"

echo "Done."
