package syver

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeCurl records each URL install.sh asks for and writes a stub executable
// to the -o target, so the script's `--version` call succeeds offline.
const fakeCurl = `#!/bin/sh
out=""
url=""
while [ $# -gt 0 ]; do
  case "$1" in
    -o) out="$2"; shift ;;
    http*) url="$1" ;;
  esac
  shift
done
echo "$url" >> "$FAKE_CURL_LOG"
[ -n "$out" ] && printf '#!/bin/sh\necho fake\n' > "$out"
exit 0
`

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
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not available")
	}
	script, err := filepath.Abs("install.sh")
	if err != nil {
		t.Fatal(err)
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
			bin := t.TempDir()
			dst := t.TempDir()
			log := filepath.Join(t.TempDir(), "curl.log")
			writeExec := func(name, body string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			writeExec("curl", fakeCurl)
			writeExec("uname", "#!/bin/sh\ncase \"$1\" in -s) echo '"+tc.os+"' ;; *) echo '"+tc.uname+"' ;; esac\n")

			cmd := exec.Command(sh, script)
			cmd.Env = append(os.Environ(),
				"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
				"FAKE_CURL_LOG="+log,
				"SYVER_VER=v9.9.9", "GOSS_VER=",
				"SYVER_DST="+dst, "GOSS_DST=")
			out, err := cmd.CombinedOutput()

			if tc.asset == "" {
				if err == nil || !strings.Contains(string(out), tc.err) {
					t.Fatalf("want an %q error, got err=%v\n%s", tc.err, err, out)
				}
				return
			}
			if err != nil {
				t.Fatalf("install.sh failed: %v\n%s", err, out)
			}
			urls, err := os.ReadFile(log)
			if err != nil {
				t.Fatal(err)
			}
			// The script always resolves releases/latest first, even when a
			// version is pinned, so pick out the binary download.
			var got []string
			for line := range strings.Lines(string(urls)) {
				if strings.Contains(line, "/releases/download/") {
					got = append(got, strings.TrimSpace(line))
				}
			}
			want := "https://github.com/krameff/syver/releases/download/v9.9.9/" + tc.asset
			if len(got) != 1 || got[0] != want {
				t.Fatalf("binary downloads = %q, want exactly %q", got, want)
			}
		})
	}
}
