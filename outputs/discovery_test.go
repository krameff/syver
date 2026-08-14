package outputs

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/krameff/syver/util"
)

func TestDiscoveryOutput(t *testing.T) {
	var buf bytes.Buffer
	output := Discovery{}
	code := output.Output(&buf, map[string]bool{
		"auditd_installed": true,
		"clang_available":  false,
	}, util.OutputConfig{})

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid json output: %v", err)
	}

	discovered, ok := parsed["Discovered"].(map[string]any)
	if !ok {
		t.Fatalf("expected Discovered object, got %#v", parsed["Discovered"])
	}

	if discovered["auditd_installed"] != true {
		t.Fatalf("expected auditd_installed=true, got %#v", discovered["auditd_installed"])
	}
	if discovered["clang_available"] != false {
		t.Fatalf("expected clang_available=false, got %#v", discovered["clang_available"])
	}
}
