package library

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

var (
	ErrOutsideRoot = errors.New("path escapes video root")
	ErrNotFile     = errors.New("path is not a regular file")
	ErrNotDir      = errors.New("path is not a directory")
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

// Folder is a subdirectory under the video root.
type Folder struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// BrowseResult is one directory listing (Synology-style current folder).
type BrowseResult struct {
	Path    string   `json:"path"`
	Parent  string   `json:"parent"`
	Folders []Folder `json:"folders"`
	Videos  []Video  `json:"videos"`
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
	abs, err := l.resolveWithinRoot(id, false)
	if err != nil {
		return "", err
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

// Browse lists folders and MP4 files in relPath (empty = root). Uses ReadDir only.
func (l *Library) Browse(relPath string) (BrowseResult, error) {
	rel, err := normalizeRelDir(relPath)
	if err != nil {
		return BrowseResult{}, err
	}

	abs, err := l.resolveWithinRoot(rel, true)
	if err != nil {
		return BrowseResult{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return BrowseResult{}, err
	}
	if !info.IsDir() {
		return BrowseResult{}, ErrNotDir
	}

	entries, err := os.ReadDir(abs)
	if err != nil {
		return BrowseResult{}, err
	}

	result := BrowseResult{
		Path:    rel,
		Parent:  parentRel(rel),
		Folders: []Folder{},
		Videos:  []Video{},
	}

	for _, e := range entries {
		name := e.Name()
		if name == "." || name == ".." || strings.HasPrefix(name, ".") {
			continue
		}
		childRel := name
		if rel != "" {
			childRel = rel + "/" + name
		}
		if e.IsDir() {
			result.Folders = append(result.Folders, Folder{Name: name, Path: childRel})
			continue
		}
		if !strings.EqualFold(filepath.Ext(name), ".mp4") {
			continue
		}
		fi, infoErr := e.Info()
		if infoErr != nil {
			return BrowseResult{}, infoErr
		}
		result.Videos = append(result.Videos, Video{
			ID:   childRel,
			Name: name,
			Path: childRel,
			Size: fi.Size(),
		})
	}

	sort.Slice(result.Folders, func(i, j int) bool {
		return strings.ToLower(result.Folders[i].Name) < strings.ToLower(result.Folders[j].Name)
	})
	sort.Slice(result.Videos, func(i, j int) bool {
		return strings.ToLower(result.Videos[i].Name) < strings.ToLower(result.Videos[j].Name)
	})
	return result, nil
}

// Search finds videos whose name or path contains query (case-insensitive).
// limit <= 0 defaults to 200.
func (l *Library) Search(query string, limit int) ([]Video, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []Video{}, nil
	}
	if limit <= 0 {
		limit = 200
	}
	all, err := l.List()
	if err != nil {
		return nil, err
	}
	needle := strings.ToLower(query)
	out := make([]Video, 0)
	for _, v := range all {
		if strings.Contains(strings.ToLower(v.Name), needle) ||
			strings.Contains(strings.ToLower(v.Path), needle) {
			out = append(out, v)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

// resolveWithinRoot joins rel under root with escape checks.
// allowEmpty permits "" meaning the root directory itself.
func (l *Library) resolveWithinRoot(rel string, allowEmpty bool) (string, error) {
	rel = strings.TrimSpace(rel)
	rel = strings.ReplaceAll(rel, "\\", "/")
	rel = strings.Trim(rel, "/")
	if rel == "" {
		if !allowEmpty {
			return "", fmt.Errorf("%w: empty id", ErrOutsideRoot)
		}
		return l.root, nil
	}
	if rel == ".." || strings.HasPrefix(rel, "../") || strings.Contains(rel, "/../") || strings.HasSuffix(rel, "/..") {
		return "", ErrOutsideRoot
	}
	for _, part := range strings.Split(rel, "/") {
		if part == "" || part == "." || part == ".." || strings.HasPrefix(part, ".") {
			return "", ErrOutsideRoot
		}
	}

	joined := filepath.Join(l.root, filepath.FromSlash(rel))
	abs, err := filepath.Abs(joined)
	if err != nil {
		return "", err
	}
	rootWithSep := l.root + string(os.PathSeparator)
	if abs != l.root && !strings.HasPrefix(abs, rootWithSep) {
		return "", ErrOutsideRoot
	}
	return abs, nil
}

func normalizeRelDir(relPath string) (string, error) {
	rel := strings.TrimSpace(relPath)
	rel = strings.ReplaceAll(rel, "\\", "/")
	rel = strings.Trim(rel, "/")
	if rel == "" {
		return "", nil
	}
	if rel == ".." || strings.HasPrefix(rel, "../") || strings.Contains(rel, "/../") || strings.HasSuffix(rel, "/..") {
		return "", ErrOutsideRoot
	}
	parts := strings.Split(rel, "/")
	clean := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || p == "." {
			continue
		}
		if p == ".." || strings.HasPrefix(p, ".") {
			return "", ErrOutsideRoot
		}
		clean = append(clean, p)
	}
	return strings.Join(clean, "/"), nil
}

func parentRel(rel string) string {
	if rel == "" {
		return ""
	}
	i := strings.LastIndex(rel, "/")
	if i < 0 {
		return ""
	}
	return rel[:i]
}

// Invalidate clears the in-memory list cache.
func (l *Library) Invalidate() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cached = nil
	l.cachedAt = time.Time{}
	l.cacheErr = nil
}

// Save writes an uploaded MP4 into the video root.
// filename may be a basename or a relative path (e.g. "movies/a.mp4");
// parent directories are created as needed. If the target exists, a numeric suffix is appended.
func (l *Library) Save(filename string, r io.Reader) (Video, error) {
	rel, err := sanitizeUploadRelPath(filename)
	if err != nil {
		return Video{}, err
	}

	dest := uniquePath(l.root, rel)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return Video{}, fmt.Errorf("create upload dir: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(dest), ".upload-*.tmp")
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

	outRel, err := filepath.Rel(l.root, dest)
	if err != nil {
		return Video{}, err
	}
	outRel = filepath.ToSlash(outRel)
	return Video{
		ID:   outRel,
		Name: filepath.Base(dest),
		Path: outRel,
		Size: written,
	}, nil
}

// sanitizeUploadRelPath accepts a basename or slash-separated relative path under the video root.
func sanitizeUploadRelPath(filename string) (string, error) {
	name := strings.TrimSpace(filename)
	name = strings.ReplaceAll(name, "\\", "/")
	name = strings.Trim(name, "/")
	if name == "" {
		return "", ErrInvalidName
	}

	parts := strings.Split(name, "/")
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." {
			continue
		}
		if part == ".." || strings.HasPrefix(part, ".") {
			return "", ErrInvalidName
		}
		for _, r := range part {
			if r < 32 || !unicode.IsPrint(r) {
				return "", ErrInvalidName
			}
		}
		clean = append(clean, part)
	}
	if len(clean) == 0 {
		return "", ErrInvalidName
	}
	if !strings.EqualFold(filepath.Ext(clean[len(clean)-1]), ".mp4") {
		return "", ErrNotMP4
	}
	return strings.Join(clean, "/"), nil
}

// uniquePath returns an absolute path under root for rel (slash-separated).
// If the path exists, inserts _N before the extension.
func uniquePath(root, rel string) string {
	candidate := filepath.Join(root, filepath.FromSlash(rel))
	if _, err := os.Stat(candidate); os.IsNotExist(err) {
		return candidate
	}
	dir := filepath.ToSlash(filepath.Dir(rel))
	base := filepath.Base(rel)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	for i := 1; ; i++ {
		name := fmt.Sprintf("%s_%d%s", stem, i, ext)
		if dir == "." {
			rel = name
		} else {
			rel = dir + "/" + name
		}
		candidate = filepath.Join(root, filepath.FromSlash(rel))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}
