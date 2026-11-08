package api

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProbeVideoDirOK(t *testing.T) {
	dir := t.TempDir()
	exists, writable, detail := probeVideoDir(dir)
	if !exists || !writable || detail != "" {
		t.Fatalf("exists=%v writable=%v detail=%q", exists, writable, detail)
	}
}

func TestProbeVideoDirMissing(t *testing.T) {
	exists, writable, detail := probeVideoDir(filepath.Join(t.TempDir(), "nope"))
	if exists || writable || detail == "" {
		t.Fatalf("exists=%v writable=%v detail=%q", exists, writable, detail)
	}
}

func TestProbeVideoDirNotWritable(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	exists, writable, _ := probeVideoDir(dir)
	if !exists {
		t.Fatal("expected exists")
	}
	if writable {
		t.Fatal("expected not writable")
	}
}
