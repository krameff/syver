package system

import (
	"strings"
	"testing"
)

// TestParseServiceExistsProbe is FEAT-010 Task 4's Linux-runnable proof of
// the locale-independent absence signal (D-7). It feeds captured PowerShell
// output through the parse directly, rather than spawning PowerShell -- see
// parseServiceExistsProbe's own comment.
func TestParseServiceExistsProbe(t *testing.T) {
	t.Run("True means the service exists", func(t *testing.T) {
		exists, err := parseServiceExistsProbe("True\r\n", "")
		if err != nil || !exists {
			t.Errorf("parseServiceExistsProbe(True) = (%v, %v), want (true, nil)", exists, err)
		}
	})

	t.Run("False means the service does not exist, and that is not an error", func(t *testing.T) {
		exists, err := parseServiceExistsProbe("False\r\n", "")
		if err != nil || exists {
			t.Errorf("parseServiceExistsProbe(False) = (%v, %v), want (false, nil)", exists, err)
		}
	})

	t.Run("anything else is an error naming what was seen, not a silent false", func(t *testing.T) {
		_, err := parseServiceExistsProbe("", "some unrelated PowerShell noise")
		if err == nil {
			t.Fatal("expected an error for unrecognised probe output")
		}
		if !strings.Contains(err.Error(), "unrelated PowerShell noise") {
			t.Errorf("error %q does not surface the captured stderr for diagnosis", err)
		}
	})
}

// TestParseServiceAttributeProbe covers Enabled/Running's combined
// existence-plus-attribute probe. The key regression this guards: a missing
// service must be distinguishable from a service that exists but is
// Manual/Disabled or Stopped -- SW-4's "TypoedName: {enabled: false} passes"
// defect.
func TestParseServiceAttributeProbe(t *testing.T) {
	t.Run("ABSENT means the service does not exist", func(t *testing.T) {
		value, exists := parseServiceAttributeProbe("ABSENT\r\n")
		if exists {
			t.Errorf("parseServiceAttributeProbe(ABSENT) reported exists=true, value=%q", value)
		}
	})

	t.Run("EXISTS|<value> means the service exists, value carries the attribute", func(t *testing.T) {
		value, exists := parseServiceAttributeProbe("EXISTS|Automatic\r\n")
		if !exists {
			t.Fatal("expected exists=true")
		}
		if !strings.Contains(value, "Automatic") {
			t.Errorf("value = %q, want it to contain Automatic", value)
		}
	})

	t.Run("EXISTS|Manual is present but not automatic -- distinguishable from absent", func(t *testing.T) {
		value, exists := parseServiceAttributeProbe("EXISTS|Manual\r\n")
		if !exists {
			t.Fatal("a Manual-start service still exists")
		}
		if strings.Contains(value, "Automatic") {
			t.Errorf("Manual should not be reported as Automatic, got %q", value)
		}
	})
}
