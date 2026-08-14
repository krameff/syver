package syver

import (
	"testing"

	"github.com/krameff/syver/resource"
)

func TestMergePreservesDiscovery(t *testing.T) {
	incoming := SyverConfig{
		Discovery: DiscoveryConfig{
			Files: map[string]*resource.File{
				"/etc/hosts": {
					Exists: true,
					DiscoveryMeta: resource.DiscoveryMeta{
						Register: "hosts_exists",
					},
				},
			},
		},
	}

	merged, err := mergeJSONData(incoming, 0, t.TempDir())
	if err != nil {
		t.Fatalf("mergeJSONData: %v", err)
	}

	if merged.Discovery.IsEmpty() {
		t.Fatal("expected discovery section to survive mergeJSONData")
	}

	entries, err := merged.Discovery.Entries()
	if err != nil {
		t.Fatalf("discovery entries: %v", err)
	}
	if len(entries) != 1 || entries[0].Register != "hosts_exists" {
		t.Fatalf("unexpected discovery entries: %#v", entries)
	}
}
