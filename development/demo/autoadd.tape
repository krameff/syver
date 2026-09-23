# VHS tape: writing a spec from a running host with `syver autoadd`, validating
# it, breaking it, and validating again so the failure output is on screen.
#
# Render from THIS directory, then bake the cards on:
#
#   vhs autoadd.tape
#   cp autoadd.mp4 autoadd.raw.mp4
#   ./bake-card.sh -i card-autoadd.png -o card-outro.png autoadd.mp4
#   mv autoadd.branded.mp4 autoadd.mp4
#
# Keep autoadd.raw.mp4. Baking is cheap and rendering is not, so a change to
# either card should re-bake from the raw file -- and baking a second time onto
# an already-branded recording gives it two of each card, which nothing checks.
#
# NEITHER CARD CAN COME FROM THE TAPE. VHS drives a terminal and has no
# directive for showing an image, so both are concatenated afterwards by
# bake-card.sh, which reads the recording's own resolution, pixel format and
# frame rate rather than assuming them.
#
# The closing card is the call to action: `Get started` over
# syver.readthedocs.io, three seconds rather than the opening card's two,
# because a viewer has to act on it rather than just read it.
#
# MP4 ONLY, DELIBERATELY. GIF encoding is a palettegen/paletteuse two-pass that
# holds frames in memory, so its cost scales with LENGTH, not resolution. It was
# OOM-killed part way on this host while VHS still exited 0, leaving a zero-byte
# .gif beside a good .mp4.
#
# The demo needs a running sshd and `syver` on PATH at /usr/local/bin/syver.

Output autoadd.mp4

Set Width 1080
Set Height 1080
Set FontSize 24
Set Padding 60
Set TypingSpeed 60ms
Set Theme { "background": "#1a1f3a", "foreground": "#f5f6fa", "cursor": "#e91e8c", "selection": "#2f3766", "black": "#1a1f3a", "red": "#e91e8c", "green": "#78c896", "yellow": "#f0c674", "blue": "#8a92b2", "magenta": "#e91e8c", "cyan": "#8ec5d6", "white": "#f5f6fa", "brightBlack": "#8a92b2", "brightRed": "#e91e8c", "brightGreen": "#78c896", "brightYellow": "#f0c674", "brightBlue": "#a9b0cc", "brightMagenta": "#ff4fa8", "brightCyan": "#8ec5d6", "brightWhite": "#ffffff" }

Hide
Type "cd $(mktemp -d) && clear"
Enter
Show

Sleep 800ms
Type "# Write a spec from a host that already works"
Sleep 600ms
Enter
Type "/usr/local/bin/syver autoadd sshd"
Sleep 400ms
Enter
Sleep 2s

Type "cat syver.yaml"
Enter
Sleep 4s

Type "clear"
Enter
Type "# Now validate it"
Enter
Type "/usr/local/bin/syver validate"
Sleep 400ms
Enter
Sleep 4s

Type "clear"
Enter
Type "# Lets Break it and set running to false"
Sleep 400ms
Enter
Type "sed -i 's/running: true/running: false/g' syver.yaml"
Enter

Type "cat syver.yaml"
Enter
Sleep 4s

Type "# Now validate it show the failure"
Enter
Type "/usr/local/bin/syver validate"
Sleep 400ms
Enter
Sleep 4s
