//go:build !windows

package system

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/krameff/syver/util"
)

// Contents must stream rather than read the whole file up front: a FIFO, or
// any file that does not end, would otherwise never return.
func TestFileContentsStreams(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("mkfifo: %v", err)
	}

	writerDone := make(chan struct{})
	release := make(chan struct{})
	go func() {
		defer close(writerDone)
		w, err := os.OpenFile(fifo, os.O_WRONLY, 0)
		if err != nil {
			return
		}
		defer w.Close()
		_, _ = w.WriteString("first line\n")
		<-release
	}()
	t.Cleanup(func() {
		close(release)
		<-writerDone
	})

	got := make(chan string, 1)
	go func() {
		r, err := NewDefFile(context.Background(), fifo, nil, util.Config{}).Contents()
		if err != nil {
			got <- "error: " + err.Error()
			return
		}
		defer r.(io.Closer).Close()
		buf := make([]byte, 64)
		n, _ := r.Read(buf)
		got <- string(buf[:n])
	}()

	select {
	case s := <-got:
		if s != "first line\n" {
			t.Fatalf("first read = %q, want %q", s, "first line\n")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Contents did not return data while the writer was still open: it is not streaming")
	}
}

// A caller that stops before EOF can still release the file, and reads after
// EOF keep returning EOF.
func TestFileContentsReaderClose(t *testing.T) {
	p := writeTempFile(t, "abcdef")

	r, err := NewDefFile(context.Background(), p, nil, util.Config{}).Contents()
	if err != nil {
		t.Fatalf("Contents: %v", err)
	}
	buf := make([]byte, 2)
	if _, err := r.Read(buf); err != nil {
		t.Fatalf("Read: %v", err)
	}
	c, ok := r.(io.Closer)
	if !ok {
		t.Fatal("reader does not implement io.Closer")
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if _, err := r.Read(buf); err == nil {
		t.Fatal("Read after Close returned no error")
	}

	r, err = NewDefFile(context.Background(), p, nil, util.Config{}).Contents()
	if err != nil {
		t.Fatalf("Contents: %v", err)
	}
	if _, err := io.ReadAll(r); err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if n, err := r.Read(buf); n != 0 || err != io.EOF {
		t.Fatalf("Read after EOF = %d, %v; want 0, EOF", n, err)
	}
}
