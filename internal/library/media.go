package library

import (
	"path/filepath"
	"strings"
)

var videoExts = map[string]string{
	".mp4":  "video/mp4",
	".m4v":  "video/mp4",
	".webm": "video/webm",
	".mkv":  "video/x-matroska",
	".mov":  "video/quicktime",
}

// legacyBrowserUnsupported are containers browsers generally cannot play.
// They are not listed/uploaded; stream requests still return 503.
var legacyBrowserUnsupported = map[string]bool{
	".avi":  true,
	".rmvb": true,
	".rm":   true,
}

// IsVideoExt reports whether name's extension is a supported video container.
func IsVideoExt(name string) bool {
	_, ok := videoExts[extOf(name)]
	return ok
}

// ContentType returns the MIME type for a video path. Unknown extensions use octet-stream.
func ContentType(name string) string {
	if t, ok := videoExts[extOf(name)]; ok {
		return t
	}
	return "application/octet-stream"
}

// UnsupportedInBrowser reports containers that HTML5 video cannot play; callers should 503.
func UnsupportedInBrowser(name string) bool {
	return legacyBrowserUnsupported[extOf(name)]
}

func extOf(name string) string {
	return strings.ToLower(filepath.Ext(name))
}
