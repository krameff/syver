package resource

import (
	"io"
	"strings"
	"sync"
)

// lockedBuilder is a concurrency-safe capture buffer. strings.Builder is not
// safe for concurrent writes, and warnSpecOnce is deliberately called from many
// goroutines in TestWarnSpecOnceFiresOncePerKey: two DISTINCT keys can clear
// LoadOrStore at the same instant and write together. Guarding the buffer keeps
// the test measuring the guard rather than tripping over its own harness.
type lockedBuilder struct {
	mu sync.Mutex
	b  strings.Builder
}

func (l *lockedBuilder) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuilder) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// warnSpecOutput redirects warning output for a test and returns a restore func.
func warnSpecOutput(w io.Writer) func() {
	prev := warnOut
	warnOut = w
	warnedSpec.Range(func(k, _ any) bool { warnedSpec.Delete(k); return true })
	return func() {
		warnOut = prev
		warnedSpec.Range(func(k, _ any) bool { warnedSpec.Delete(k); return true })
	}
}
