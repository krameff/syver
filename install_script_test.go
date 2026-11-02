package syver

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// fakeCurl records each URL install.sh asks for and writes a stub to the -o
// target: the SHA256SUMS file is copied from $FAKE_SUMS, anything else gets
// fakeBinary, so the script's `--version` call succeeds offline.
//
// A URL ending in $FAKE_CURL_404 behaves like a real 404: with -f (or --fail)
// curl writes nothing and exits 22, and without it curl saves the server's
// error page and exits 0, which is how a mistyped version used to install an
// HTML page as the binary.
const fakeCurl = `#!/bin/sh
out=""
url=""
fail=0
while [ $# -gt 0 ]; do
  case "$1" in
    -o) out="$2"; shift ;;
    --fail) fail=1 ;;
    --*) ;;
    -*f*) fail=1 ;;
    http*) url="$1" ;;
  esac
  shift
done
echo "$url" >> "$FAKE_CURL_LOG"
if [ -n "$FAKE_CURL_404" ]; then
  case "$url" in
    *"$FAKE_CURL_404")
      [ "$fail" = 1 ] && exit 22
      [ -n "$out" ] && echo '<html>Not Found</html>' > "$out"
      exit 0 ;;
  esac
fi
if [ -n "$out" ]; then
  case "$url" in
    *_SHA256SUMS) cat "$FAKE_SUMS" > "$out" ;;
    *) printf '#!/bin/sh\necho fake\n' > "$out" ;;
  esac
fi
exit 0
`

// fakeBinary is what fakeCurl writes for a binary download.
const fakeBinary = "#!/bin/sh\necho fake\n"

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// sumsLine is one SHA256SUMS entry in the format goreleaser writes.
func sumsLine(content, name string) string {
	return sha256Hex(content) + "  " + name + "\n"
}

type installRun struct {
	out  string
	err  error
	urls []string
	dst  string
}

// runInstall runs install.sh with a fake uname and curl first on PATH, against
// a SHA256SUMS holding sums. When bin is non-empty the fakes go there and that
// directory is the whole PATH. Each extra entry is appended to the
// environment, overriding the defaults set here.
func runInstall(t *testing.T, osName, machine, sums, bin string, extra ...string) installRun {
	t.Helper()
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not available")
	}
	script, err := filepath.Abs("install.sh")
	if err != nil {
		t.Fatal(err)
	}
	path := bin
	if bin == "" {
		bin = t.TempDir()
		path = bin + string(os.PathListSeparator) + os.Getenv("PATH")
	}
	writeExec := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeExec("curl", fakeCurl)
	writeExec("uname", "#!/bin/sh\ncase \"$1\" in -s) echo '"+osName+"' ;; *) echo '"+machine+"' ;; esac\n")

	work := t.TempDir()
	log := filepath.Join(work, "curl.log")
	sumsFile := filepath.Join(work, "sums")
	if err := os.WriteFile(sumsFile, []byte(sums), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()

	cmd := exec.Command(sh, script)
	cmd.Env = append(os.Environ(),
		"PATH="+path,
		"FAKE_CURL_LOG="+log,
		"FAKE_SUMS="+sumsFile,
		"FAKE_CURL_404=",
		"SYVER_SKIP_CHECKSUM=",
		"SYVER_VER=v9.9.9", "GOSS_VER=",
		"SYVER_DST="+dst, "GOSS_DST=")
	cmd.Env = append(cmd.Env, extra...)
	out, runErr := cmd.CombinedOutput()

	r := installRun{out: string(out), err: runErr, dst: dst}
	if b, err := os.ReadFile(log); err == nil {
		for line := range strings.Lines(string(b)) {
			r.urls = append(r.urls, strings.TrimSpace(line))
		}
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return r
}

// dstEntries lists what install.sh left in its destination directory.
func dstEntries(t *testing.T, dst string) []string {
	t.Helper()
	entries, err := os.ReadDir(dst)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// TestInstallScriptArch runs install.sh against a fake uname and curl and
// checks each OS and machine name downloads a binary the release actually
// publishes (goreleaser's syver-<os>-<arch>), and that combinations with no
// published binary (32-bit machines, darwin on s390x, an OS with no build) are
// refused rather than sent to a URL that 404s.
//
// REVERT-PROOF: restoring the old `aarch32|arm) arch="arm"` or
// `i?86) arch="386"` mapping fails the "arm" or "i686" case, and the old
// hard-coded `syver-linux-$arch` URL fails both Darwin cases.
func TestInstallScriptArch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install.sh is a POSIX shell script")
	}

	tests := []struct {
		os    string // uname -s
		uname string // uname -m
		asset string // empty: no binary is published, so the script must refuse it
		err   string // the refusal expected when asset is empty
	}{
		{"Linux", "x86_64", "syver-linux-amd64", ""},
		{"Linux", "aarch64", "syver-linux-arm64", ""},
		{"Linux", "arm64", "syver-linux-arm64", ""},
		{"Linux", "s390x", "syver-linux-s390x", ""},
		{"Linux", "ppc64le", "syver-linux-ppc64le", ""},
		{"Linux", "ppc64", "", "unsupported architecture"},
		{"Linux", "armv7l", "", "unsupported architecture"},
		{"Linux", "arm", "", "unsupported architecture"},
		{"Linux", "i686", "", "unsupported architecture"},
		{"Linux", "mips", "", "unsupported architecture"},
		{"Darwin", "arm64", "syver-darwin-arm64", ""},
		{"Darwin", "x86_64", "syver-darwin-amd64", ""},
		{"Darwin", "s390x", "", "unsupported architecture"},
		{"FreeBSD", "x86_64", "", "unsupported OS"},
		{"MINGW64_NT-10.0", "x86_64", "", "unsupported OS"},
	}
	for _, tc := range tests {
		t.Run(tc.os+"/"+tc.uname, func(t *testing.T) {
			r := runInstall(t, tc.os, tc.uname, sumsLine(fakeBinary, tc.asset), "")

			if tc.asset == "" {
				if r.err == nil || !strings.Contains(r.out, tc.err) {
					t.Fatalf("want an %q error, got err=%v\n%s", tc.err, r.err, r.out)
				}
				return
			}
			if r.err != nil {
				t.Fatalf("install.sh failed: %v\n%s", r.err, r.out)
			}
			// Pick the binary download out of the release requests; the
			// SHA256SUMS fetch is checked in TestInstallScriptChecksum.
			var got []string
			for _, u := range r.urls {
				if strings.Contains(u, "/releases/download/") && !strings.HasSuffix(u, "_SHA256SUMS") {
					got = append(got, u)
				}
			}
			want := "https://github.com/krameff/syver/releases/download/v9.9.9/" + tc.asset
			if len(got) != 1 || got[0] != want {
				t.Fatalf("binary downloads = %q, want exactly %q", got, want)
			}
		})
	}
}

// TestInstallScriptChecksum covers how install.sh fetches and verifies the
// binary: it is checked against the release's SHA256SUMS before it is
// installed, and any failed download or failed check leaves nothing at the
// destination, neither a broken file nor the private temporary directory, and
// leaves a previously installed binary untouched.
//
// REVERT-PROOF: the pre-fix install.sh (curl without -f, written straight to
// the destination, no checksum) fails every case except "skip checksum" and
// "unwritable destination". Dropping -f from download() alone fails "binary
// 404" and "wrapper 404". Making the comparison always pass fails "checksum
// mismatch" and "failed upgrade keeps the old binary". Downloading straight
// to $INSTALL_LOC fails "checksum mismatch", "binary not listed", "SHA256SUMS
// 404" and "failed upgrade keeps the old binary".
func TestInstallScriptChecksum(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install.sh is a POSIX shell script")
	}
	const asset = "syver-linux-amd64"
	good := sumsLine("other", "syver-windows-amd64.exe") + sumsLine(fakeBinary, asset)
	sumsURL := "https://github.com/krameff/syver/releases/download/v9.9.9/syver_9.9.9_SHA256SUMS"

	tests := []struct {
		name     string
		sums     string
		env      []string
		existing bool   // a syver binary is already installed
		missing  bool   // SYVER_DST does not exist
		wantErr  string // empty: the install must succeed
		want     []string
	}{
		{name: "verified", sums: good, want: []string{"dgoss", "dsyver", "syver"}},
		{name: "binary 404", sums: good, env: []string{"FAKE_CURL_404=/" + asset}, wantErr: "Download failed"},
		{name: "checksum mismatch", sums: sumsLine("tampered", asset), wantErr: "Checksum mismatch"},
		{
			// The asset's name only appears inside other entries, so a match
			// on anything looser than the whole name would pass.
			name: "binary not listed", sums: sumsLine(fakeBinary, "x"+asset) + sumsLine(fakeBinary, asset+".sbom.json"),
			wantErr: "is not listed",
		},
		{name: "SHA256SUMS 404", sums: good, env: []string{"FAKE_CURL_404=_SHA256SUMS"}, wantErr: "SYVER_SKIP_CHECKSUM=1"},
		{
			name: "skip checksum", sums: good, env: []string{"FAKE_CURL_404=_SHA256SUMS", "SYVER_SKIP_CHECKSUM=1"},
			want: []string{"dgoss", "dsyver", "syver"},
		},
		{
			name: "failed upgrade keeps the old binary", sums: sumsLine("tampered", asset), existing: true,
			wantErr: "Checksum mismatch", want: []string{"syver"},
		},
		{
			// The binary is already in place by then; the point is that the
			// failed wrapper leaves no file of its own behind.
			name: "wrapper 404", sums: good, env: []string{"FAKE_CURL_404=/extras/dsyver/dsyver"},
			wantErr: "Download failed", want: []string{"syver"},
		},
		{name: "unwritable destination", sums: good, missing: true, wantErr: "Cannot write to"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := tc.env
			var dst string
			if tc.existing || tc.missing {
				dst = t.TempDir()
				if tc.missing {
					dst = filepath.Join(dst, "missing")
				} else if err := os.WriteFile(filepath.Join(dst, "syver"), []byte("old\n"), 0o755); err != nil {
					t.Fatal(err)
				}
				env = append(slices.Clone(env), "SYVER_DST="+dst)
			}
			r := runInstall(t, "Linux", "x86_64", tc.sums, "", env...)
			if dst == "" {
				dst = r.dst
			}

			if tc.wantErr == "" {
				if r.err != nil {
					t.Fatalf("install.sh failed: %v\n%s", r.err, r.out)
				}
			} else if r.err == nil || !strings.Contains(r.out, tc.wantErr) {
				t.Fatalf("want a %q error, got err=%v\n%s", tc.wantErr, r.err, r.out)
			}
			if tc.missing {
				if _, err := os.Stat(dst); !os.IsNotExist(err) {
					t.Fatalf("install.sh created the missing destination: %v", err)
				}
				return
			}

			if got := dstEntries(t, dst); !slices.Equal(got, tc.want) {
				t.Errorf("destination holds %q, want %q\n%s", got, tc.want, r.out)
			}
			if slices.Contains(tc.want, "syver") {
				b, err := os.ReadFile(filepath.Join(dst, "syver"))
				if err != nil {
					t.Fatal(err)
				}
				want := fakeBinary
				if tc.existing {
					want = "old\n"
				}
				if string(b) != want {
					t.Errorf("installed syver is %q, want %q", b, want)
				}
			}

			// Every case that gets as far as a downloaded binary must fetch
			// SHA256SUMS, unless the check was skipped.
			skipped := slices.Contains(tc.env, "SYVER_SKIP_CHECKSUM=1")
			if fetched := slices.Contains(r.urls, sumsURL); tc.name != "binary 404" && fetched == skipped {
				t.Errorf("SHA256SUMS fetched=%v with SYVER_SKIP_CHECKSUM=1 %v; requests:\n%s",
					fetched, skipped, strings.Join(r.urls, "\n"))
			}
			if tc.name == "verified" && !strings.Contains(r.out, "Verified "+asset) {
				t.Errorf("no verification message\n%s", r.out)
			}
		})
	}
}

// TestInstallScriptShasum runs the verified install with shasum, macOS's tool,
// as the only SHA-256 command on PATH, so the fallback is exercised on Linux.
//
// REVERT-PROOF: removing the shasum branch from sha256() fails this test with
// the "Neither sha256sum nor shasum" error.
func TestInstallScriptShasum(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install.sh is a POSIX shell script")
	}
	sha256sum, err := exec.LookPath("sha256sum")
	if err != nil {
		t.Skip("sha256sum not available to back the fake shasum")
	}
	// PATH is only this directory, so link in exactly what the script and
	// fakeCurl run besides shell builtins.
	bin := t.TempDir()
	for _, tool := range []string{"mktemp", "rm", "awk", "chmod", "mv", "cat"} {
		p, err := exec.LookPath(tool)
		if err != nil {
			t.Skipf("%s not available", tool)
		}
		if err := os.Symlink(p, filepath.Join(bin, tool)); err != nil {
			t.Fatal(err)
		}
	}
	shasum := "#!/bin/sh\n[ \"$1\" = -a ] && [ \"$2\" = 256 ] || exit 2\nshift 2\nexec " + sha256sum + " \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "shasum"), []byte(shasum), 0o755); err != nil {
		t.Fatal(err)
	}

	r := runInstall(t, "Darwin", "arm64", sumsLine(fakeBinary, "syver-darwin-arm64"), bin)
	if r.err != nil || !strings.Contains(r.out, "Verified syver-darwin-arm64") {
		t.Fatalf("install.sh did not verify with shasum: err=%v\n%s", r.err, r.out)
	}
}
