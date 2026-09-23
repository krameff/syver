# Demo content

Terminal recordings for the documentation and for release announcements. A
recording is generated from a [VHS](https://github.com/charmbracelet/vhs) tape,
so it is a script: reproducible, reviewable in a diff, and correctable without
re-filming anything.

| File | What it is |
| --- | --- |
| `dsyver-demo.tape` | Checking a container image with `dsyver`: a spec, a passing run, then a failing run with its exit code |
| `autoadd.tape` | Writing a spec from a running host with `syver autoadd`, validating it, breaking it, validating again |
| `make-card.sh` | Builds a 1080x1080 title card from `images/syver-logo.svg` |
| `bake-card.sh` | Concatenates an opening card and a closing card onto a finished recording |
| `card-dsyver.png`, `card-autoadd.png` | The opening card for each tape |
| `card-outro.png` | The shared closing card, which carries the call to action |

**The renders themselves are not committed.** A minute of 1080x1080 video is
most of a megabyte, against a repository whose largest tracked file is
`CHANGELOG.md`. Git keeps every version of it forever, nobody can review it in
a diff, and every clone pays for it. `.gitignore` covers `*.mp4`, `*.gif` and
`*.webm` in this directory. See Publishing below.

The cards ARE committed, although they are reproducible from `make-card.sh` and
the logo. They are small, they change rarely, and having them here means baking
a recording needs only `ffmpeg`, not `rsvg-convert` and a font stack as well.
Rebuilding one is byte-identical, so a card that differs from its script is a
signal rather than noise.

## MP4 only. No GIFs.

Decided 2026-09-23, and the tapes carry no `Output *.gif` line.

The reason is measured rather than aesthetic. GIF encoding is a
`palettegen`/`paletteuse` two-pass that holds frames in memory, so its cost
scales with the **length** of the recording, not its resolution. On the render
host `ffmpeg` was OOM-killed part way through -- `Out of memory: Killed process
(ffmpeg)` in the kernel log, roughly 1.4GB resident -- and **VHS still exited
0**, leaving a zero-byte `.gif` next to a perfectly good `.mp4`. It failed the
same way after the host went from 1.6GB to 3.4GB.

A short tape at the same 1080x1080 renders a valid GIF, so this is not a
resolution or a VHS problem. If a GIF is ever wanted, cut the recording short
and check the file is non-zero before publishing it: a silent zero-byte output
is the failure mode to expect.

## Prerequisites on the machine that renders

VHS needs more than itself:

* `vhs`
* `ttyd` and `ffmpeg` -- VHS drives a headless terminal through the first and
  encodes with the second. VHS will not run without them.
* `rsvg-convert` (`librsvg2-bin`), for `make-card.sh` only.
* **At least one monospace font installed.** On a bare server `fc-list` often
  returns nothing, and VHS fails or renders blank boxes rather than saying the
  font is missing. Check with `fc-list | wc -l` before blaming the tape.

Per tape, on top of that:

* `dsyver-demo.tape` needs a container runtime and the ability to pull
  `nginx:alpine`, plus `dsyver` on `PATH` and `SYVER_PATH` pointing at a
  **statically linked** syver binary. Released binaries are static. A locally
  built one is not unless you build it with `CGO_ENABLED=0`, and inside an
  Alpine image the failure reads `sh: /syver/syver: not found` -- a missing
  loader, not a missing file.
* `autoadd.tape` needs a running `sshd`, because it writes a spec from it, and
  `syver` on `PATH`.

## Rendering

Run from **this directory**: each tape's `Output` is a bare filename, so the
render lands wherever you ran it.

```sh
vhs dsyver-demo.tape
cp dsyver-demo.mp4 dsyver-demo.raw.mp4
./bake-card.sh -i card-dsyver.png -o card-outro.png dsyver-demo.mp4
mv dsyver-demo.branded.mp4 dsyver-demo.mp4
```

**Keep the `.raw.mp4`.** Rendering is a minute or more; baking is seconds. A
change to a card should re-bake from the raw file rather than re-render, and
baking twice onto an already-branded recording gives it two of each card, which
nothing checks.

## Cards

```sh
./make-card.sh "Testing container images" card-dsyver.png
SUBTITLE=syver.readthedocs.io ./make-card.sh "Get started" card-outro.png
```

The logo already carries the wordmark, the tagline and the company line, so the
only text the script adds is the title, and the call to action on the closing
card. Each line is drawn by its own `drawtext` so each is centred on its own
width; one multi-line textfile would left-align the shorter line inside the
block.

**Titles are passed through `textfile=`, never an inline `text=`.** ffmpeg's
filtergraph parser eats spaces in an inline value and renders the card with the
logo and NO title at all, exit 0, no warning. A one-word title works fine,
which is exactly how it hides.

**Keep titles to about 24 characters.** The title is drawn at a fixed 54pt with
no fit logic, so a long one runs to the canvas edges and eventually off them.
`Writing a spec from a running host` measured 1080px wide on a 1080px canvas --
not clipped, but it read as clipped, and it was replaced with
`Auto generating a spec`. `Testing container images` leaves about 160px each
side.

## Baking the cards on

`bake-card.sh` puts an opening card at the front and a closing card at the end:

```sh
./bake-card.sh -i card-dsyver.png -o card-outro.png dsyver-demo.mp4
```

Defaults are two seconds for the opening card and three for the closing one,
`-s` and `-e` to change them. The closing card is longer on purpose: a viewer
reads the opening card knowing the demo follows, but has to act on the closing
one.

**Neither card can come from the tape.** VHS drives a terminal and has no
directive for showing an image, so the cards are concatenated afterwards.

**The script encodes each card from the recording's own parameters, and this
matters more than it looks.** Concat with `-c copy` only works when the card
matches the recording exactly, and a mismatch does not fail in a way that says
so: it either refuses, or stitches the card in at the wrong rate and the result
merely looks odd. Two constants that were wrong when measured on 2026-09-23:

* **Frame rate.** Both recordings come out at **25 fps**, not 24. Note that
  `dsyver-demo.tape` sets `Set Framerate 24` and `autoadd.tape` sets nothing,
  and both render at 25 on VHS v0.11.0. Whatever that directive steers, it is
  not the output frame rate.
* **Audio.** Both recordings are video-only. Encoding a card with an
  `anullsrc` track, which is the usual recipe, gives it a stream the recording
  does not have.

`bake-card.sh` reads width, height, pixel format and frame rate from the
recording with `ffprobe`, refuses outright if the recording has an audio
stream, and checks afterwards that the output actually grew by the length of
the cards. ffmpeg exits 0 on plenty of outputs nobody wants.

## What to check in the result

1. **Nothing sensitive is in frame.** The tapes hide their setup, but the
   prompt may carry a hostname or username depending on the shell.
   `Set Shell bash` keeps it plain; check anyway, because this gets published.
2. **The failing run is legible.** It is the point of both demos -- a passing
   test only shows the tool runs, a failing one shows it catches something and
   prints the exit code that makes it a CI gate.
3. **The pauses are right.** The `Sleep` after each run is a guess: the checks
   take milliseconds but container start dominates, and that varies by machine
   and by whether the image is already pulled. Too short and the output scrolls
   past unread; too long and the file is bloated.
4. **The cards are really on it, and are the right way round.** Compare a frame
   from inside each card against the PNG with ffmpeg's `ssim` filter; a match
   scores about 0.998. Check the closing frame against the OPENING card too and
   confirm it scores lower, or a doubled intro reads as a pass.

## Publishing

Upload the finished `.mp4` as an asset on a GitHub release and reference the
absolute URL, which keeps it on GitHub's CDN and out of the history:

```markdown
[Watch the walkthrough](https://github.com/krameff/syver/releases/download/<tag>/dsyver-demo.mp4)
```

Do NOT route it through `.goreleaser.yaml`'s `extra_files`, which uploads from
the working tree and so puts the file back in the repository.

**And add nothing to the docs until the URL resolves.** A reference to a file
that does not exist builds a broken page, and `mkdocs build --strict` fails on
it.
