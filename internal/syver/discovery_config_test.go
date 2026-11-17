package syver

import (
	"testing"

	"github.com/krameff/syver/resource"
)

func TestDiscoveryConfigEntries(t *testing.T) {
	cfg := SyverConfig{
		Discovery: DiscoveryConfig{
			Files: resource.FileMap{
				"/bin/clang": {
					Exists: true,
					DiscoveryMeta: resource.DiscoveryMeta{
						Register: "clang_available",
					},
				},
			},
		},
	}

	entries, err := cfg.Discovery.Entries()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Register != "clang_available" {
		t.Fatalf("expected register clang_available, got %q", entries[0].Register)
	}
}

func TestDiscoveryConfigRequiresRegister(t *testing.T) {
	cfg := SyverConfig{
		Discovery: DiscoveryConfig{
			Files: resource.FileMap{
				"/bin/clang": {
					Exists: true,
				},
			},
		},
	}

	if _, err := cfg.Discovery.Entries(); err == nil {
		t.Fatal("expected error for missing register")
	}
}

func TestDiscoveredFromVars(t *testing.T) {
	discovered := discoveredFromVars(map[string]any{
		"Discovered": map[string]any{
			"clang_available": true,
		},
		"OS": "linux",
	})

	if discovered["clang_available"] != true {
		t.Fatalf("expected discovered clang_available=true, got %#v", discovered["clang_available"])
	}
}

func TestBuildScheduleDependsOn(t *testing.T) {
	resources := []resource.Resource{
		&resource.File{
			Exists: true,
			DiscoveryMeta: resource.DiscoveryMeta{
				Register: "base",
			},
		},
		&resource.Command{
			ExitStatus: 0,
			DiscoveryMeta: resource.DiscoveryMeta{
				DependsOn: []string{"/tmp/base"},
			},
		},
	}
	resources[0].SetID("/tmp/base")
	resources[1].SetID("follow-up")

	schedule, err := buildSchedule(resources)
	if err != nil {
		t.Fatalf("unexpected schedule error: %v", err)
	}
	if len(schedule) != 2 {
		t.Fatalf("expected 2 scheduled resources, got %d", len(schedule))
	}
	if len(schedule[1].deps) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(schedule[1].deps))
	}
}

func TestBuildScheduleDetectsCycles(t *testing.T) {
	a := &resource.Command{ExitStatus: 0, DiscoveryMeta: resource.DiscoveryMeta{DependsOn: []string{"b"}}}
	b := &resource.Command{ExitStatus: 0, DiscoveryMeta: resource.DiscoveryMeta{DependsOn: []string{"a"}}}
	a.SetID("a")
	b.SetID("b")

	_, err := buildSchedule([]resource.Resource{a, b})
	if err == nil {
		t.Fatal("expected cycle detection error")
	}
}
