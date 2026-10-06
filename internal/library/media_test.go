package library

import "testing"

func TestIsVideoExt(t *testing.T) {
	if !IsVideoExt("a.MKV") || !IsVideoExt("b.mp4") || IsVideoExt("c.txt") {
		t.Fatal("ext whitelist")
	}
	if IsVideoExt("old.avi") || IsVideoExt("old.rmvb") || IsVideoExt("old.rm") {
		t.Fatal("avi/rmvb/rm must not be listed")
	}
	if ContentType("x.mkv") != "video/x-matroska" {
		t.Fatal(ContentType("x.mkv"))
	}
}

func TestUnsupportedInBrowser(t *testing.T) {
	if !UnsupportedInBrowser("a.avi") || !UnsupportedInBrowser("a.RMVB") || !UnsupportedInBrowser("a.rm") {
		t.Fatal("legacy formats should be unsupported")
	}
	if UnsupportedInBrowser("a.mp4") || UnsupportedInBrowser("a.mkv") {
		t.Fatal("mp4/mkv should stream")
	}
}
