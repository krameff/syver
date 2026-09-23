# VHS tape: checking a container image with dsyver.
#
# Render from THIS directory, then bake the cards on. The result is a build
# output, not repository content: it is gitignored and published as a release
# asset. See README.md.
#
#   vhs dsyver-demo.tape
#   cp dsyver-demo.mp4 dsyver-demo.raw.mp4
#   ./bake-card.sh -i card-dsyver.png -o card-outro.png dsyver-demo.mp4
#   mv dsyver-demo.branded.mp4 dsyver-demo.mp4
#
# Keep dsyver-demo.raw.mp4. Baking is seconds and rendering is a minute and a
# half, so a change to either card should re-bake from the raw file -- and
# baking a second time onto an already-branded recording gives it two of each
# card, which nothing checks.
#
# NEITHER CARD CAN COME FROM THE TAPE. VHS drives a terminal and has no
# directive for showing an image, so both are concatenated afterwards by
# bake-card.sh, which reads the recording's own resolution, pixel format and
# frame rate rather than assuming them. The closing card is the call to action,
# three seconds against the opening card's two.
#
# Prerequisites and what to check in the result: README.md next to this file.
#
# WHY A TAPE AND NOT A SCREENCAST. This file IS the recording: reviewable in a
# diff, re-renders identically anywhere, and when
# docs/containers/testing-images.md changes, the mismatch shows up here as a
# line to edit rather than as a video nobody can correct without re-filming.
#
# MP4 ONLY, DELIBERATELY. GIF encoding is a palettegen/paletteuse two-pass that
# holds frames in memory, so its cost scales with LENGTH, not resolution. It was
# OOM-killed part way on the render host while VHS still exited 0, leaving a
# zero-byte .gif beside a good .mp4 -- and it still fails at 3.4GB.

Output dsyver-demo.mp4

Set Shell bash
Set Width 1080
Set Height 1080
# 24, not 30. MEASURED from frames of a 30pt render: 1080 wide at 30pt gives
# ~36 columns against syver output lines of ~60 characters, so they wrapped
# mid-word AND the failing run scrolled off the top of the frame. At 24pt the
# whole run fits on one screen. Removing wrapping entirely needs ~16pt, too
# small for this format, so this is a trade rather than a fix.
Set FontSize 24
Set Padding 60
Set TypingSpeed 60ms
# 24fps rather than the 50 default: a terminal recording loses nothing by it.
# NOTE, measured 2026-09-23 on VHS v0.11.0: the MP4 comes out at 25 fps with
# this line and at 25 fps without it, so whatever it steers, it is not the
# output frame rate. autoadd.tape sets nothing and renders at 25 too. Anything
# that has to match the recording must MEASURE it -- bake-card.sh does.
Set Framerate 24
Set Theme { "background": "#1a1f3a", "foreground": "#f5f6fa", "cursor": "#e91e8c", "selection": "#2f3766", "black": "#1a1f3a", "red": "#e91e8c", "green": "#78c896", "yellow": "#f0c674", "blue": "#8a92b2", "magenta": "#e91e8c", "cyan": "#8ec5d6", "white": "#f5f6fa", "brightBlack": "#8a92b2", "brightRed": "#e91e8c", "brightGreen": "#78c896", "brightYellow": "#f0c674", "brightBlue": "#a9b0cc", "brightMagenta": "#ff4fa8", "brightCyan": "#8ec5d6", "brightWhite": "#ffffff" }

# --- Setup, deliberately off camera --------------------------------------
# Only environment is hidden: no assertion, no result, nothing the viewer is
# asked to take on trust. The image is pulled ON camera below, so which image is
# being checked is never left implicit.
#
# THREE VHS TRAPS, each of which cost a render:
#  * `Type` takes a SINGLE-LINE string. A heredoc spanning lines is parsed as
#    tape commands and fails with dozens of "Invalid command" errors, which is
#    why the spec below is written with a one-line printf.
#  * The lexer does not handle an escaped double quote inside a double-quoted
#    string. Use single quotes inside double quotes instead.
#  * `Output` rejects an UNQUOTED absolute path. Relative paths are fine.
#
# AND THE SPEC ORDER MATTERS: `file:` is LAST because the failing beat appends
# two lines, and an append lands under whichever block is last. With `command:`
# last the new entry becomes a command attribute and syver rejects it with
# `invalid Attribute for Command`, derailing the demo for an unrelated reason.
Hide
Type "cd $(mktemp -d)" Enter
Type "export PATH=$HOME/bin:$PATH" Enter
Type "export SYVER_PATH=$(command -v syver)" Enter
Type "printf 'package:\n  nginx:\n    installed: true\nuser:\n  nginx:\n    exists: true\ncommand:\n  nginx -v:\n    exit-status: 0\nfile:\n  /etc/nginx/nginx.conf:\n    exists: true\n' > syver.yaml" Enter
Type "clear" Enter
Show

# --- 1. The image being checked -------------------------------------------
Type "# The image we are going to check" Enter
Sleep 800ms
Type "docker pull nginx:alpine" Enter
Sleep 4s

# --- 2. What we expect to be inside it ------------------------------------
Type "clear" Enter
Type "# What should be inside it?" Enter
Sleep 800ms
Type "cat syver.yaml" Enter
Sleep 4s

# --- 3. Check it ----------------------------------------------------------
Type "clear" Enter
Type "# Check it from the inside" Enter
Sleep 800ms
Type "dsyver run nginx:alpine" Enter
Sleep 9s

# --- 4. Break it ----------------------------------------------------------
# The failing beat is the point. A passing test proves the tool runs; a failing
# one proves it CATCHES something, and shows the exit code that makes it a gate.
Type "clear" Enter
Type "# Now expect a file the image does not have" Enter
Sleep 800ms
Type "printf '  /etc/nginx/conf.d/tls.conf:\n    exists: true\n' >> syver.yaml" Enter
Sleep 1s
Type "dsyver run nginx:alpine; echo 'exit status:' $?" Enter
Sleep 9s
