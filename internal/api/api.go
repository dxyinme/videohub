package api

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dxyinme/videohub/internal/library"
)

// Server exposes HTTP handlers for Video Hub.
type Server struct {
	lib            *library.Library
	static         fs.FS
	showRoot       bool
	maxUploadBytes int64
}

// Options configures optional Server behavior.
type Options struct {
	MaxUploadBytes int64
}

// New creates an API server. static should be the web/ filesystem root.
func New(lib *library.Library, static fs.FS, opts Options) *Server {
	maxUpload := opts.MaxUploadBytes
	if maxUpload <= 0 {
		maxUpload = 2 * 1024 * 1024 * 1024
	}
	return &Server{
		lib:            lib,
		static:         static,
		showRoot:       true,
		maxUploadBytes: maxUpload,
	}
}

// Handler returns the root HTTP handler with request timing logs.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/videos", s.handleListVideos)
	mux.HandleFunc("GET /api/browse", s.handleBrowse)
	mux.HandleFunc("GET /api/search", s.handleSearch)
	mux.HandleFunc("POST /api/videos", s.handleUpload)
	mux.HandleFunc("GET /api/stream/{id...}", s.handleStream)
	mux.Handle("GET /", http.FileServer(http.FS(s.static)))
	return withRequestLog(mux)
}

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.bytes += int64(n)
	return n, err
}

func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func withRequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		log.Printf(
			`msg=request method=%s path=%q query=%q status=%d bytes=%d duration_ms=%d remote=%q`,
			r.Method,
			r.URL.Path,
			r.URL.RawQuery,
			sw.status,
			sw.bytes,
			time.Since(start).Milliseconds(),
			r.RemoteAddr,
		)
	})
}

// probeVideoDir reports whether the video root exists and is writable.
func probeVideoDir(root string) (exists, writable bool, detail string) {
	st, err := os.Stat(root)
	if err != nil {
		return false, false, err.Error()
	}
	if !st.IsDir() {
		return false, false, "not a directory"
	}
	f, err := os.CreateTemp(root, ".videohub-health-*")
	if err != nil {
		return true, false, err.Error()
	}
	name := f.Name()
	_ = f.Close()
	if err := os.Remove(name); err != nil {
		return true, false, err.Error()
	}
	return true, true, ""
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	root := s.lib.Root()
	exists, writable, detail := probeVideoDir(root)
	status := "ok"
	code := http.StatusOK
	if !exists || !writable {
		status = "unhealthy"
		code = http.StatusServiceUnavailable
	}
	resp := map[string]any{
		"status":           status,
		"videoDir":         root,
		"videoDirExists":   exists,
		"videoDirWritable": writable,
	}
	if detail != "" {
		resp["detail"] = detail
	}
	writeJSON(w, code, resp)
}

func (s *Server) handleListVideos(w http.ResponseWriter, _ *http.Request) {
	videos, err := s.lib.List()
	if err != nil {
		log.Printf("list videos: %v", err)
		http.Error(w, "failed to list videos", http.StatusInternalServerError)
		return
	}
	if videos == nil {
		videos = []library.Video{}
	}
	resp := map[string]any{"videos": videos}
	if s.showRoot {
		resp["root"] = s.lib.Root()
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	result, err := s.lib.Browse(path)
	if err != nil {
		if errors.Is(err, library.ErrOutsideRoot) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if errors.Is(err, library.ErrNotDir) {
			http.Error(w, "not a directory", http.StatusBadRequest)
			return
		}
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		log.Printf("browse %q: %v", path, err)
		http.Error(w, "failed to browse", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	videos, err := s.lib.Search(q, 200)
	if err != nil {
		log.Printf("search %q: %v", q, err)
		http.Error(w, "failed to search", http.StatusInternalServerError)
		return
	}
	if videos == nil {
		videos = []library.Video{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"query":  q,
		"videos": videos,
		"limit":  200,
	})
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, s.maxUploadBytes+1024*1024)

	reader, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "expected multipart form", http.StatusBadRequest)
		return
	}

	var (
		video   library.Video
		found   bool
		relPath string
	)
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if isMaxBytesError(err) {
				http.Error(w, "upload too large", http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, "invalid multipart form", http.StatusBadRequest)
			return
		}

		switch part.FormName() {
		case "path":
			raw, readErr := io.ReadAll(io.LimitReader(part, 4<<10))
			_ = part.Close()
			if readErr != nil {
				http.Error(w, "invalid path field", http.StatusBadRequest)
				return
			}
			relPath = strings.TrimSpace(string(raw))
			continue
		case "file":
		default:
			_, _ = io.Copy(io.Discard, io.LimitReader(part, 1<<20))
			_ = part.Close()
			continue
		}

		if found {
			_ = part.Close()
			http.Error(w, `only one "file" field is allowed`, http.StatusBadRequest)
			return
		}
		found = true

		filename := relPath
		if filename == "" {
			filename = part.FileName()
		}
		if library.UnsupportedInBrowser(filename) {
			_ = part.Close()
			http.Error(w, "AVI/RMVB are not supported; upload HTML5-playable video (e.g. MP4/WebM)", http.StatusServiceUnavailable)
			return
		}
		limited := &io.LimitedReader{R: part, N: s.maxUploadBytes + 1}
		video, err = s.lib.Save(filename, limited)
		_, _ = io.Copy(io.Discard, part)
		_ = part.Close()
		if err != nil {
			switch {
			case errors.Is(err, library.ErrUnsupportedVideo), errors.Is(err, library.ErrInvalidName):
				http.Error(w, err.Error(), http.StatusBadRequest)
			default:
				log.Printf("upload %q: %v", filename, err)
				http.Error(w, "failed to save upload", http.StatusInternalServerError)
			}
			return
		}
		if limited.N == 0 {
			if abs, rerr := s.lib.Resolve(video.ID); rerr == nil {
				_ = os.Remove(abs)
			}
			s.lib.Invalidate()
			http.Error(w, "upload too large", http.StatusRequestEntityTooLarge)
			return
		}
	}

	if !found {
		http.Error(w, `form field "file" is required`, http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"video": video})
}

func (s *Server) resolveVideo(w http.ResponseWriter, r *http.Request, id string) (abs string, info os.FileInfo, ok bool) {
	if library.UnsupportedInBrowser(id) {
		http.Error(w, "AVI/RMVB are not supported in HTML5 video; convert to MP4/WebM before upload", http.StatusServiceUnavailable)
		return "", nil, false
	}
	abs, err := s.lib.Resolve(id)
	if err != nil {
		if errors.Is(err, library.ErrOutsideRoot) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return "", nil, false
		}
		if errors.Is(err, library.ErrNotFile) {
			http.Error(w, "not a file", http.StatusBadRequest)
			return "", nil, false
		}
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return "", nil, false
		}
		log.Printf("resolve %q: %v", id, err)
		http.Error(w, "failed to open video", http.StatusInternalServerError)
		return "", nil, false
	}
	st, err := os.Stat(abs)
	if err != nil {
		http.Error(w, "failed to stat video", http.StatusInternalServerError)
		return "", nil, false
	}
	return abs, st, true
}

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.PathValue("id"), "/")
	abs, stat, ok := s.resolveVideo(w, r, id)
	if !ok {
		return
	}

	f, err := os.Open(abs)
	if err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		log.Printf("open %q: %v", abs, err)
		http.Error(w, "failed to open video", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", library.ContentType(id))
	http.ServeContent(w, r, stat.Name(), stat.ModTime(), f)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	if err := enc.Encode(v); err != nil {
		log.Printf("write json: %v", err)
	}
}

func isMaxBytesError(err error) bool {
	var maxBytesErr *http.MaxBytesError
	return errors.As(err, &maxBytesErr)
}
