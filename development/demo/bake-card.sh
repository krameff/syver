#!/bin/bash
# Bake a title card and a closing call-to-action card onto a VHS recording.
#
#   ./bake-card.sh -i card-autoadd.png -o card-outro.png ../autoadd/autoadd.mp4
#
# Writes <recording>.branded.mp4 beside the recording. It deliberately does NOT
# replace the input, so the raw render stays available to re-bake from. Baking
# a second time onto an already-branded file gives you two cards.
#
# WHY THIS IS NOT A TAPE DIRECTIVE. VHS drives a terminal and has no directive
# for showing an image, so neither card can come from the tape. They are
# concatenated afterwards, and concat with `-c copy` only works when each card
# matches the recording exactly.
#
# THE CARDS ARE ENCODED FROM THE RECORDING'S OWN PARAMETERS, never from
# constants. The tapes README recommends a hard-coded 24 fps and an anullsrc
# audio track. Both are wrong for what VHS writes here -- 25 fps, and no audio
# stream at all -- and a mismatch does not fail loudly: concat either refuses,
# or stitches a card in at the wrong rate and the result merely looks odd.
set -euo pipefail

INTRO=""
OUTRO=""
INTRO_SECS=2
# Longer than the intro on purpose: a viewer reads the opening card knowing the
# demo follows, but has to act on the closing one.
OUTRO_SECS=3

usage() {
    echo "usage: bake-card.sh [-i intro.png] [-o outro.png] [-s secs] [-e secs] <recording.mp4>" >&2
    exit 1
}

while getopts ":i:o:s:e:" opt; do
    case "$opt" in
        i) INTRO="$OPTARG" ;;
        o) OUTRO="$OPTARG" ;;
        s) INTRO_SECS="$OPTARG" ;;
        e) OUTRO_SECS="$OPTARG" ;;
        *) usage ;;
    esac
done
shift $((OPTIND - 1))

REC="${1:-}"
[[ -n "$REC" ]] || usage
[[ -n "$INTRO$OUTRO" ]] || { echo "bake-card.sh: nothing to bake; give -i, -o or both." >&2; exit 1; }
[[ -r "$REC" ]] || { echo "bake-card.sh: cannot read $REC" >&2; exit 1; }
for c in "$INTRO" "$OUTRO"; do
    [[ -z "$c" || -r "$c" ]] || { echo "bake-card.sh: cannot read $c" >&2; exit 1; }
done

OUT="${REC%.mp4}.branded.mp4"

read -r W H PIX FPS < <(ffprobe -v error -select_streams v:0 \
    -show_entries stream=width,height,pix_fmt,r_frame_rate \
    -of csv=p=0 "$REC" | tr ',' ' ')

if ffprobe -v error -select_streams a -show_entries stream=index -of csv=p=0 "$REC" | grep -q .; then
    echo "bake-card.sh: $REC carries an audio stream; this script assumes none." >&2
    exit 1
fi

before="$(ffprobe -v error -show_entries format=duration -of csv=p=0 "$REC")"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

encode_card() { # <png> <seconds> <out.mp4>
    ffmpeg -v error -y -loop 1 -t "$2" -i "$1" \
        -c:v libx264 -profile:v high -pix_fmt "$PIX" -r "$FPS" \
        -vf "scale=${W}:${H}" "$3"
}

: > "$work/concat.txt"
added=0
if [[ -n "$INTRO" ]]; then
    encode_card "$INTRO" "$INTRO_SECS" "$work/intro.mp4"
    printf "file '%s'\n" "$work/intro.mp4" >> "$work/concat.txt"
    added="$(awk -v a="$added" -v b="$INTRO_SECS" 'BEGIN{print a + b}')"
fi
printf "file '%s'\n" "$(cd "$(dirname "$REC")" && pwd)/$(basename "$REC")" >> "$work/concat.txt"
if [[ -n "$OUTRO" ]]; then
    encode_card "$OUTRO" "$OUTRO_SECS" "$work/outro.mp4"
    printf "file '%s'\n" "$work/outro.mp4" >> "$work/concat.txt"
    added="$(awk -v a="$added" -v b="$OUTRO_SECS" 'BEGIN{print a + b}')"
fi

ffmpeg -v error -y -f concat -safe 0 -i "$work/concat.txt" -c copy "$OUT"

# Prove it did something. ffmpeg exits 0 on plenty of outputs nobody wants, and
# a concat that silently dropped a segment still produces a playable file.
[[ -s "$OUT" ]] || { echo "bake-card.sh: $OUT is empty" >&2; exit 1; }
after="$(ffprobe -v error -show_entries format=duration -of csv=p=0 "$OUT")"
ok="$(awk -v a="$after" -v b="$before" -v s="$added" 'BEGIN{print (a - b > s * 0.75) ? "yes" : "no"}')"
if [[ "$ok" != "yes" ]]; then
    echo "bake-card.sh: $OUT is ${after}s against ${before}s in, expected about ${added}s more." >&2
    exit 1
fi
echo "wrote ${OUT}: ${before}s -> ${after}s (${W}x${H} ${PIX} ${FPS}${INTRO:+, intro ${INTRO_SECS}s}${OUTRO:+, outro ${OUTRO_SECS}s})"
