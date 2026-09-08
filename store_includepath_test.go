package syver

import (
	"runtime"
	"testing"
)

// TestIsRootedIncludePath covers FEAT-013 / FEAT-011 W2-9 (SW-12).
//
// The cases are split three ways on purpose. The shared cases must hold on
// every platform; the Unix and Windows blocks assert the two places the answer
// legitimately differs. Writing them as one table with a GOOS switch inside
// would hide exactly the distinction this fix is about.
func TestIsRootedIncludePath(t *testing.T) {
	shared := []struct {
		path string
		want bool
	}{
		{"/etc/syver/base.yaml", true},
		{"/base.yaml", true},
		{"base.yaml", false},
		{"./base.yaml", false},
		{"../shared/base.yaml", false},
		{"sub/dir/base.yaml", false},
		{"", false},
	}
	for _, tc := range shared {
		if got := isRootedIncludePath(tc.path); got != tc.want {
			t.Errorf("isRootedIncludePath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}

	if runtime.GOOS == "windows" {
		// The regression this fix exists for: an absolute Windows path was
		// treated as relative and joined onto the including file's directory.
		windows := []struct {
			path string
			want bool
		}{
			{`C:\syver\base.yaml`, true},
			{`c:\syver\base.yaml`, true},
			{`C:/syver/base.yaml`, true},
			{`\\server\share\base.yaml`, true},
			{`C:base.yaml`, false}, // drive-relative, not absolute
			{`base.yaml`, false},
		}
		for _, tc := range windows {
			if got := isRootedIncludePath(tc.path); got != tc.want {
				t.Errorf("windows: isRootedIncludePath(%q) = %v, want %v", tc.path, got, tc.want)
			}
		}
		return
	}

	// On Unix a `C:\...` string is an ordinary relative filename. Asserting
	// that keeps the fix from quietly changing Linux behaviour, which is the
	// half of this change that carries real risk.
	unix := []struct {
		path string
		want bool
	}{
		{`C:\syver\base.yaml`, false},
		{`\\server\share\base.yaml`, false},
	}
	for _, tc := range unix {
		if got := isRootedIncludePath(tc.path); got != tc.want {
			t.Errorf("unix: isRootedIncludePath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}
