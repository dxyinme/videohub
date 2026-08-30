package library

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResolveRejectsEscape(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.mp4"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	lib, err := New(root, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := lib.Resolve("../a.mp4"); err == nil {
		t.Fatal("expected escape to fail")
	}
	abs, err := lib.Resolve("a.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if abs != filepath.Join(root, "a.mp4") {
		t.Fatalf("got %q", abs)
	}
}

func TestListScanDepth(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "root.mp4"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "movies")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "nested.mp4"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	shallow, err := New(root, 1, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	videos, err := shallow.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(videos) != 1 || videos[0].ID != "root.mp4" {
		t.Fatalf("depth=1 got %#v", videos)
	}

	deep, err := New(root, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	videos, err = deep.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(videos) != 2 {
		t.Fatalf("depth=0 got %#v", videos)
	}
}

func TestSaveUpload(t *testing.T) {
	root := t.TempDir()
	lib, err := New(root, 0, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := lib.Save("../evil/../clip.mp4", strings.NewReader("data1")); err == nil {
		t.Fatal("expected path escape rejection")
	}

	v, err := lib.Save("clip.mp4", strings.NewReader("data1"))
	if err != nil {
		t.Fatal(err)
	}
	if v.ID != "clip.mp4" || v.Size != 5 {
		t.Fatalf("got %#v", v)
	}

	v2, err := lib.Save("clip.mp4", strings.NewReader("data22"))
	if err != nil {
		t.Fatal(err)
	}
	if v2.ID != "clip_1.mp4" {
		t.Fatalf("expected unique name, got %#v", v2)
	}

	nested, err := lib.Save("movies/action/a.mp4", strings.NewReader("nest"))
	if err != nil {
		t.Fatal(err)
	}
	if nested.ID != "movies/action/a.mp4" {
		t.Fatalf("expected nested path, got %#v", nested)
	}
	if _, err := os.Stat(filepath.Join(root, "movies", "action", "a.mp4")); err != nil {
		t.Fatal(err)
	}

	if _, err := lib.Save("note.txt", strings.NewReader("x")); err == nil {
		t.Fatal("expected non-mp4 rejection")
	}

	videos, err := lib.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(videos) != 3 {
		t.Fatalf("list after upload: %#v", videos)
	}
}
