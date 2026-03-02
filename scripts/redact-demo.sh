#!/usr/bin/env bash
# redact-demo.sh — Blur the WRDS username on the login screen of a VHS-recorded GIF.
#
# Usage:
#   ./scripts/redact-demo.sh                                        # default paths
#   ./scripts/redact-demo.sh raw.gif clean.gif                      # explicit paths
#
# All blur parameters can be overridden via environment variables.
# Record once, inspect the raw GIF, then tweak BLUR_X/Y/W/H if needed.
#
# Requires: ffmpeg

set -euo pipefail

INPUT="${1:-demo-wrds-download-raw.gif}"
OUTPUT="${2:-demo-wrds-download.gif}"

if [[ ! -f "$INPUT" ]]; then
  echo "Error: $INPUT not found. Run 'vhs demo-wrds-download.tape' first." >&2
  exit 1
fi

if ! command -v ffmpeg &>/dev/null; then
  echo "Error: ffmpeg is required. Install with: brew install ffmpeg" >&2
  exit 1
fi

# ── Blur region (pixels) ──
# These cover the username text inside the "Login as <user>  [enter]" button.
# Calibrated from the login frame at t≈2s (1400x700, FontSize 18, Padding 20).
BLUR_X="${BLUR_X:-490}"
BLUR_Y="${BLUR_Y:-210}"
BLUR_W="${BLUR_W:-100}"
BLUR_H="${BLUR_H:-20}"

# ── Time window (seconds) ──
# The login screen is visible from launch until schemas load.
BLUR_START="${BLUR_START:-0}"
BLUR_END="${BLUR_END:-5}"

# ── Pixelation factor (higher = more pixelated, less readable) ──
PIXEL_FACTOR="${PIXEL_FACTOR:-10}"

TEMP=$(mktemp -d)
trap 'rm -rf "$TEMP"' EXIT

echo "Pixelating region (${BLUR_X},${BLUR_Y}) ${BLUR_W}x${BLUR_H} during t=${BLUR_START}s-${BLUR_END}s (factor=${PIXEL_FACTOR})..."

ffmpeg -y -loglevel error -i "$INPUT" -filter_complex \
  "[0:v]crop=${BLUR_W}:${BLUR_H}:${BLUR_X}:${BLUR_Y}, \
   scale=iw/${PIXEL_FACTOR}:ih/${PIXEL_FACTOR}, \
   scale=${BLUR_W}:${BLUR_H}:flags=neighbor[px]; \
   [0:v][px]overlay=${BLUR_X}:${BLUR_Y}:enable='between(t,${BLUR_START},${BLUR_END})'[v]; \
   [v]split[s0][s1]; \
   [s0]palettegen=max_colors=256:stats_mode=diff[p]; \
   [s1][p]paletteuse=dither=bayer:bayer_scale=3" \
  "$TEMP/out.gif"

mv "$TEMP/out.gif" "$OUTPUT"
echo "Done: $INPUT -> $OUTPUT"
