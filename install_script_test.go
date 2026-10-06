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
// checks each machine name downloads a binary the release actually publishes
// (goreleaser's syver-linux-<arch>), and that 32-bit machines, for which no
// binary is published, are refused rather than sent to a URL that 404s.
//
// REVERT-PROOF: restoring the old `aarch32|arm) arch="arm"` or
// `i?86) arch="386"` mapping fails the "arm" or "i686" case.
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
		uname string
		asset string // empty: no binary is published, so the script must refuse it
	}{
		{"x86_64", "syver-linux-amd64"},
		{"aarch64", "syver-linux-arm64"},
		{"arm64", "syver-linux-arm64"},
		{"s390x", "syver-linux-s390x"},
		{"ppc64le", "syver-linux-ppc64le"},
		{"ppc64", ""},
		{"armv7l", ""},
		{"arm", ""},
		{"i686", ""},
		{"mips", ""},
	}
	for _, tc := range tests {
		t.Run(tc.uname, func(t *testing.T) {
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
			writeExec("uname", "#!/bin/sh\necho "+tc.uname+"\n")

			cmd := exec.Command(sh, script)
			cmd.Env = append(os.Environ(),
				"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
				"FAKE_CURL_LOG="+log,
				"SYVER_VER=v9.9.9", "GOSS_VER=",
				"SYVER_DST="+dst, "GOSS_DST=")
			out, err := cmd.CombinedOutput()

			if tc.asset == "" {
				if err == nil || !strings.Contains(string(out), "unsupported architecture") {
					t.Fatalf("want an unsupported-architecture error, got err=%v\n%s", err, out)
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
