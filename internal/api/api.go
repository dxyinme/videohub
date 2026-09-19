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

// Handler returns the root HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/videos", s.handleListVideos)
	mux.HandleFunc("GET /api/browse", s.handleBrowse)
	mux.HandleFunc("GET /api/search", s.handleSearch)
	mux.HandleFunc("POST /api/videos", s.handleUpload)
	// {id...} must be terminal in Go's ServeMux, so stream uses /api/stream/{id...}.
	mux.HandleFunc("GET /api/stream/{id...}", s.handleStream)
	mux.Handle("GET /", http.FileServer(http.FS(s.static)))
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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
	// Cap total request body; stream parts with MultipartReader (no full-form buffer).
	r.Body = http.MaxBytesReader(w, r.Body, s.maxUploadBytes+1024*1024) // multipart overhead

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
			// Optional relative path (FileName() strips directories per RFC 7578).
			raw, readErr := io.ReadAll(io.LimitReader(part, 4<<10))
			_ = part.Close()
			if readErr != nil {
				http.Error(w, "invalid path field", http.StatusBadRequest)
				return
			}
			relPath = strings.TrimSpace(string(raw))
			continue
		case "file":
			// handled below
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
		limited := &io.LimitedReader{R: part, N: s.maxUploadBytes + 1}
		video, err = s.lib.Save(filename, limited)
		// Drain any unread bytes so the multipart parser stays consistent.
		_, _ = io.Copy(io.Discard, part)
		_ = part.Close()
		if err != nil {
			switch {
			case errors.Is(err, library.ErrNotMP4), errors.Is(err, library.ErrInvalidName):
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

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.PathValue("id"), "/")
	abs, err := s.lib.Resolve(id)
	if err != nil {
		if errors.Is(err, library.ErrOutsideRoot) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if errors.Is(err, library.ErrNotFile) {
			http.Error(w, "not a file", http.StatusBadRequest)
			return
		}
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		log.Printf("resolve %q: %v", id, err)
		http.Error(w, "failed to open video", http.StatusInternalServerError)
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

	stat, err := f.Stat()
	if err != nil {
		http.Error(w, "failed to stat video", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "video/mp4")
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
