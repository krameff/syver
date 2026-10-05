# VHS tape: checking the Docker container a coding agent runs in.
#
# Render from a working directory holding the profile from
# examples/agent-sandbox/, then bake the cards on. The result is a build
# output, not repository content: it is gitignored and published as a release
# asset. See README.md.
#
#   vhs agent-sandbox-demo.tape
#   cp agent-sandbox-demo.mp4 agent-sandbox-demo.raw.mp4
#   ./bake-card.sh -i card-sandbox.png -o card-outro.png agent-sandbox-demo.mp4
#   mv agent-sandbox-demo.branded.mp4 agent-sandbox-demo.mp4
#
# The working directory needs, beside this tape:
#  * syver.yaml from examples/agent-sandbox/, unchanged.
#  * vars.yaml from the same place, WITH example.com:443 REMOVED from
#    egress_deny. A render host has ordinary network access, so that deny check
#    fails on the locked-down run too and the passing beat is no longer a pass.
#    The metadata address stays: it is unreachable off a cloud host.
#  * ws/, mode 777, mounted as the workspace.
#  * fakeaws/credentials, any content: the careless run mounts it as ~/.aws.
#  * an image tagged agent-sandbox: alpine with git and curl, a uid-1000 user
#    `agent`, USER agent, /workspace owned by it, CMD sleep infinity.
#  * dsyver on PATH and a STATIC syver in $HOME/bin (see README.md).
#
# BOTH RUNS USE THE SAME SPEC, VARS AND IMAGE. The only difference is the
# `docker run` flags, because that is the point: most of what this profile
# checks is how the container was started, not what is in the image.
#
# Prerequisites and what to check in the result: README.md next to this file.
# Traps shared with the other tapes (single-line Type strings, no escaped double
# quotes, quoted absolute Output paths): see dsyver-demo.tape.

Output agent-sandbox-demo.mp4

Set Shell bash
Set Width 1080
Set Height 1080
Set FontSize 24
Set Padding 60
Set TypingSpeed 60ms
Set Framerate 24
Set Theme { "background": "#1a1f3a", "foreground": "#f5f6fa", "cursor": "#e91e8c", "selection": "#2f3766", "black": "#1a1f3a", "red": "#e91e8c", "green": "#78c896", "yellow": "#f0c674", "blue": "#8a92b2", "magenta": "#e91e8c", "cyan": "#8ec5d6", "white": "#f5f6fa", "brightBlack": "#8a92b2", "brightRed": "#e91e8c", "brightGreen": "#78c896", "brightYellow": "#f0c674", "brightBlue": "#a9b0cc", "brightMagenta": "#ff4fa8", "brightCyan": "#8ec5d6", "brightWhite": "#ffffff" }

# --- Setup, deliberately off camera --------------------------------------
# Environment only. The vars file is shown on camera, so what is checked is
# never left implicit.
Hide
Type "export PATH=$HOME/bin:$PATH SYVER_PATH=$HOME/bin/syver SYVER_VARS=vars.yaml" Enter
Type "rm -f bad.log; clear" Enter
Show

# --- 1. What the sandbox must and must not have --------------------------
Type "# What the agent needs, and what it must NOT have" Enter
Sleep 800ms
Type "grep -v '^#' vars.yaml | grep ." Enter
Sleep 6s

# --- 2. A locked-down sandbox passes ---------------------------------------
Type "clear" Enter
Type "# A locked-down sandbox" Enter
Sleep 800ms
Type@35ms "dsyver run --cap-drop=ALL --security-opt no-new-privileges --read-only --memory 512m -v $PWD/ws:/workspace agent-sandbox; echo 'exit status:' $?" Enter
Sleep 9s

# --- 3. Break it: the same image, started carelessly ----------------------
# The failing beat is the point. Root, the Docker socket, an AWS credentials
# file and a token in the environment: each is one flag someone adds to get
# unstuck. The full output is long, so it goes to a file and the next command
# shows one line per failure. The grep is ANCHORED because a failing contents
# check prints the whole file it read: unanchored, /proc/self/status floods the
# screen. \S* absorbs the colour code at the start of each result line.
Type "clear" Enter
Type "# Now start the same image carelessly" Enter
Sleep 800ms
Type@35ms "dsyver run --user 0 -e GITHUB_TOKEN=ghp_canary -v /var/run/docker.sock:/var/run/docker.sock -v $PWD/fakeaws:/home/agent/.aws -v $PWD/ws:/workspace agent-sandbox > bad.log; echo 'exit status:' $?" Enter
Sleep 7s
Type "# What it caught" Enter
Sleep 800ms
Type "sed -n '/Failures/,$p' bad.log | grep -E '^\S*(File|Command|Addr|Count): '" Enter
Sleep 8s

# --- 4. The secret itself never reaches the output -------------------------
# Checks assert a variable's NAME is absent, so even a failing check does not
# print its value. Show it: zero matches for the token in the whole log.
Type "# And the token value never appears in the output" Enter
Sleep 800ms
Type "grep -c ghp_canary bad.log" Enter
Sleep 4s
