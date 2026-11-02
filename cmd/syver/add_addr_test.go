package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// `syver add addr 127.0.0.1` has no port, so it can never be dialled. It must
// still succeed and write a `reachable: false` entry: the per-platform
// integration fixtures (commands/add.goss.yaml) assert exit 0 for exactly this.
func TestAddAddrWithoutPortWritesResource(t *testing.T) {
	spec := filepath.Join(t.TempDir(), "syver.yaml")

	err := newRunnableApp(t).Run(context.Background(), []string{"syver", "-g", spec, "add", "addr", "127.0.0.1"})
	require.NoError(t, err)

	b, err := os.ReadFile(spec)
	require.NoError(t, err, "nothing was written")
	got := string(b)
	assert.Contains(t, got, "tcp://127.0.0.1:")
	assert.Contains(t, got, "reachable: false")
}
