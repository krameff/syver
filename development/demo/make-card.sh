#!/bin/bash
# Build a branded 1080x1080 title card from the Syver logo.
#
#   ./make-card.sh "Testing container images" card-dsyver.png
#
# A second line, for a closing card with a call to action, comes from SUBTITLE:
#
#   SUBTITLE=syver.readthedocs.io ./make-card.sh "Get started" card-outro.png
#
# Needs: ffmpeg, rsvg-convert (librsvg2-bin), and images/syver-logo.svg.
#
# The logo already carries the wordmark, the tagline and the company line, so the
# only text added here is the demo title, and the call to action on a closing
# card. Each line is its own drawtext so each is centred on its own width; one
# multi-line textfile would left-align the shorter line inside the block.
#
# TITLES ARE PASSED VIA textfile=, NOT text=. ffmpeg's filtergraph parser eats
# spaces in an inline `text=` value: the card renders with the logo and NO title,
# silently, exit 0. A single-word title works, which is exactly how this hides --
# it looks like a quoting problem you have already fixed.
set -euo pipefail

TITLE="${1:?usage: make-card.sh <title> [out.png]}"
OUT="${2:-card.png}"
SUBTITLE="${SUBTITLE:-}"

BG=0x1a1f3a
FG=0xf5f6fa
# The tape theme's muted blue-grey, so the second line reads as secondary
# without dropping to a grey that disappears against this background.
SUBFG=0x8a92b2
BOLD="$(fc-match -f '%{file}' 'DejaVu Sans:bold')"
REGULAR="$(fc-match -f '%{file}' 'DejaVu Sans')"

# Find the logo in either layout this script gets used from, rather than
# assuming one. In the repo it sits two levels up in images/; on a render host
# the script is usually copied into a working directory with the SVG beside it.
# LOGO_SVG overrides both.
#
# This probing exists because the single hard-coded repo-relative default failed
# on the render host with rsvg's own message naming a path that does not exist,
# which reads as a missing file rather than a wrong assumption. It was never
# caught because every test run passed LOGO_SVG explicitly.
here="$(cd "$(dirname "$0")" && pwd)"
if [[ -z "${LOGO_SVG:-}" ]]; then
    for candidate in "$here/syver-logo.svg" "$here/../../images/syver-logo.svg"; do
        if [[ -r "$candidate" ]]; then
            LOGO_SVG="$candidate"
            break
        fi
    done
fi
if [[ -z "${LOGO_SVG:-}" || ! -r "${LOGO_SVG}" ]]; then
    echo "make-card.sh: cannot find syver-logo.svg." >&2
    echo "  looked for: $here/syver-logo.svg" >&2
    echo "              $here/../../images/syver-logo.svg" >&2
    echo "  set LOGO_SVG=/path/to/syver-logo.svg to override." >&2
    exit 1
fi

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# The SVG is 500x560; render at 520 wide so it is crisp at this canvas size.
rsvg-convert -w 520 -h 582 "$LOGO_SVG" -o "$work/logo.png"
printf '%s' "$TITLE" > "$work/title.txt"

filter="[0][1]overlay=(W-w)/2:180[bg];[bg]drawtext=fontfile=${BOLD}:textfile=${work}/title.txt:fontcolor=${FG}:fontsize=54:x=(w-text_w)/2:y=830"
if [[ -n "$SUBTITLE" ]]; then
    # textfile= here too, for the same reason as the title above.
    printf '%s' "$SUBTITLE" > "$work/subtitle.txt"
    filter+="[t];[t]drawtext=fontfile=${REGULAR}:textfile=${work}/subtitle.txt:fontcolor=${SUBFG}:fontsize=40:x=(w-text_w)/2:y=912"
fi

ffmpeg -v error -y \
  -f lavfi -i "color=c=${BG}:s=1080x1080:d=1" \
  -i "$work/logo.png" \
  -filter_complex "$filter" \
  -frames:v 1 -update 1 "$OUT"

echo "wrote ${OUT}: ${TITLE}${SUBTITLE:+ / $SUBTITLE}"
