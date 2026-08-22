package library

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
)

var (
	ErrOutsideRoot = errors.New("path escapes video root")
	ErrNotFile     = errors.New("path is not a regular file")
	ErrInvalidName = errors.New("invalid filename")
	ErrNotMP4      = errors.New("only .mp4 uploads are allowed")
)

// Video is a discovered MP4 under the configured root.
type Video struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// Library scans and resolves videos under a root directory.
type Library struct {
	root      string
	scanDepth int
	cacheTTL  time.Duration

	mu        sync.Mutex
	cachedAt  time.Time
	cached    []Video
	cacheErr  error
}

// New creates a Library rooted at videoDir (must exist and be a directory).
func New(videoDir string, scanDepth int, cacheTTL time.Duration) (*Library, error) {
	abs, err := filepath.Abs(videoDir)
	if err != nil {
		return nil, fmt.Errorf("resolve video dir: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("video dir %q: %w", abs, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("video dir %q is not a directory", abs)
	}
	return &Library{
		root:      abs,
		scanDepth: scanDepth,
		cacheTTL:  cacheTTL,
	}, nil
}

// Root returns the absolute video root directory.
func (l *Library) Root() string {
	return l.root
}

// List returns discovered MP4 files, using a short in-memory cache when enabled.
func (l *Library) List() ([]Video, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.cacheTTL > 0 && time.Since(l.cachedAt) < l.cacheTTL && l.cached != nil {
		return append([]Video(nil), l.cached...), l.cacheErr
	}

	videos, err := l.scan()
	l.cached = videos
	l.cacheErr = err
	l.cachedAt = time.Now()
	return append([]Video(nil), videos...), err
}

func (l *Library) scan() ([]Video, error) {
	var videos []Video
	err := filepath.WalkDir(l.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if name != "." && strings.HasPrefix(name, ".") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if l.scanDepth > 0 {
				rel, relErr := filepath.Rel(l.root, path)
				if relErr != nil {
					return relErr
				}
				if rel != "." {
					depth := 1 + strings.Count(rel, string(os.PathSeparator))
					if depth >= l.scanDepth {
						return fs.SkipDir
					}
				}
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(name), ".mp4") {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		rel, relErr := filepath.Rel(l.root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		videos = append(videos, Video{
			ID:   rel,
			Name: name,
			Path: rel,
			Size: info.Size(),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return videos, nil
}

// Resolve maps a video id (slash-separated relative path) to an absolute file path.
func (l *Library) Resolve(id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", fmt.Errorf("%w: empty id", ErrOutsideRoot)
	}
	id = strings.ReplaceAll(id, "\\", "/")
	id = strings.TrimPrefix(id, "/")
	if id == ".." || strings.HasPrefix(id, "../") || strings.Contains(id, "/../") || strings.HasSuffix(id, "/..") {
		return "", ErrOutsideRoot
	}

	joined := filepath.Join(l.root, filepath.FromSlash(id))
	abs, err := filepath.Abs(joined)
	if err != nil {
		return "", err
	}
	rootWithSep := l.root + string(os.PathSeparator)
	if abs != l.root && !strings.HasPrefix(abs, rootWithSep) {
		return "", ErrOutsideRoot
	}

	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", ErrNotFile
	}
	return abs, nil
}

// Invalidate clears the in-memory list cache.
func (l *Library) Invalidate() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cached = nil
	l.cachedAt = time.Time{}
	l.cacheErr = nil
}

// Save writes an uploaded MP4 into the video root using a sanitized basename.
// If the target name already exists, a numeric suffix is appended.
func (l *Library) Save(filename string, r io.Reader) (Video, error) {
	name, err := sanitizeUploadName(filename)
	if err != nil {
		return Video{}, err
	}

	dest := uniquePath(l.root, name)
	tmp, err := os.CreateTemp(l.root, ".upload-*.tmp")
	if err != nil {
		return Video{}, fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		_ = tmp.Close()
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	written, err := io.Copy(tmp, r)
	if err != nil {
		return Video{}, fmt.Errorf("write upload: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return Video{}, fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpName, dest); err != nil {
		return Video{}, fmt.Errorf("finalize upload: %w", err)
	}
	cleanup = false
	l.Invalidate()

	rel, err := filepath.Rel(l.root, dest)
	if err != nil {
		return Video{}, err
	}
	rel = filepath.ToSlash(rel)
	return Video{
		ID:   rel,
		Name: filepath.Base(dest),
		Path: rel,
		Size: written,
	}, nil
}

func sanitizeUploadName(filename string) (string, error) {
	name := strings.TrimSpace(filename)
	name = strings.ReplaceAll(name, "\\", "/")
	name = filepath.Base(name)
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || strings.HasPrefix(name, ".") {
		return "", ErrInvalidName
	}
	if strings.ContainsAny(name, "/\\") {
		return "", ErrInvalidName
	}
	if !strings.EqualFold(filepath.Ext(name), ".mp4") {
		return "", ErrNotMP4
	}
	for _, r := range name {
		if r < 32 || !unicode.IsPrint(r) {
			return "", ErrInvalidName
		}
	}
	return name, nil
}

func uniquePath(root, name string) string {
	candidate := filepath.Join(root, name)
	if _, err := os.Stat(candidate); os.IsNotExist(err) {
		return candidate
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for i := 1; ; i++ {
		candidate = filepath.Join(root, fmt.Sprintf("%s_%d%s", stem, i, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}
