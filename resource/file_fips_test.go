package resource

import (
	"crypto/fips140"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/krameff/syver/system"
	"gotest.tools/v3/assert"
)

// TestFileMd5EmptyFileUnderFIPSOnly is a FIPS-only test: it skips unless the
// process runs in Go's FIPS 140-only mode. Run it with
//
//	GODEBUG=fips140=only go test -count=1 -v -run TestFileMd5EmptyFileUnderFIPSOnly ./resource/
//
// and check it reports PASS, not SKIP. A file resource with `exists: true` and
// `md5:` on an EMPTY file must give two results: `exists` passes and `md5`
// fails with syver's FIPS message. Without the guard in system/file.go the md5
// hash panics in that mode, and ValidateSafe's single `panic` result replaces
// both. Revert-proof (run in the only-mode pass): delete the fipsHashError
// call in system/file.go's hash(); this test then sees one `panic` result.
func TestFileMd5EmptyFileUnderFIPSOnly(t *testing.T) {
	if !fips140.Enforced() {
		t.Skip("run with GODEBUG=fips140=only go test -count=1 -v -run TestFileMd5EmptyFileUnderFIPSOnly ./resource/")
	}
	p := filepath.Join(t.TempDir(), "empty")
	assert.NilError(t, os.WriteFile(p, nil, 0o600))

	f := &File{Path: p, Exists: true, Md5: "d41d8cd98f00b204e9800998ecf8427e"}
	f.SetID(p)
	results := ValidateSafe(t.Context(), f, &system.System{NewFile: system.NewDefFile})

	assert.Equal(t, len(results), 2, "want exists and md5, got %+v", results)
	byProp := map[string]TestResult{}
	for _, r := range results {
		byProp[r.Property] = r
	}
	_, panicked := byProp["panic"]
	assert.Assert(t, !panicked, "a panic result replaced the resource's results: %+v", results)

	exists, ok := byProp["exists"]
	assert.Assert(t, ok, "no exists result: %+v", results)
	assert.Equal(t, exists.Result, SUCCESS)

	md5, ok := byProp["md5"]
	assert.Assert(t, ok, "no md5 result: %+v", results)
	assert.Equal(t, md5.Result, FAIL)
	assert.Assert(t, md5.Err != nil, "md5 failed without an error")
	assert.Assert(t, strings.Contains(md5.Err.Error(), "FIPS 140-only mode"), "md5 error: %v", md5.Err)
	assert.Assert(t, strings.Contains(md5.Err.Error(), "sha256"), "md5 error: %v", md5.Err)
}
