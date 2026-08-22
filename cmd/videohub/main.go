package main

import (
	"log"
	"net/http"

	"github.com/dxyinme/videohub/internal/api"
	"github.com/dxyinme/videohub/internal/config"
	"github.com/dxyinme/videohub/internal/library"
	"github.com/dxyinme/videohub/web"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	lib, err := library.New(cfg.VideoDir, cfg.ScanDepth, cfg.ListCacheTTL)
	if err != nil {
		log.Fatalf("library: %v", err)
	}

	srv := api.New(lib, web.FS, api.Options{MaxUploadBytes: cfg.MaxUploadBytes})
	log.Printf("videohub listening on %s (videos=%s depth=%d cache=%s max_upload=%dMB)",
		cfg.Addr, lib.Root(), cfg.ScanDepth, cfg.ListCacheTTL, cfg.MaxUploadBytes/(1024*1024))

	if err := http.ListenAndServe(cfg.Addr, srv.Handler()); err != nil {
		log.Fatalf("server: %v", err)
	}
}
