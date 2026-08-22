package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

const (
	envAddr         = "VIDEOHUB_ADDR"
	envVideoDir     = "VIDEOHUB_VIDEO_DIR"
	envScanDepth    = "VIDEOHUB_SCAN_DEPTH"
	envListCacheSec = "VIDEOHUB_LIST_CACHE_SEC"
	envMaxUploadMB  = "VIDEOHUB_MAX_UPLOAD_MB"
)

// Config holds process-wide settings loaded from environment variables.
type Config struct {
	Addr           string
	VideoDir       string
	ScanDepth      int           // 0 means unlimited
	ListCacheTTL   time.Duration // 0 disables cache
	MaxUploadBytes int64
}

// Load reads configuration from environment variables and applies defaults.
func Load() (Config, error) {
	cfg := Config{
		Addr:           envOr(envAddr, ":8080"),
		VideoDir:       envOr(envVideoDir, "./videos"),
		ScanDepth:      0,
		ListCacheTTL:   5 * time.Second,
		MaxUploadBytes: 2 * 1024 * 1024 * 1024, // 2 GiB
	}

	if v := os.Getenv(envScanDepth); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return Config{}, fmt.Errorf("%s must be a non-negative integer, got %q", envScanDepth, v)
		}
		cfg.ScanDepth = n
	}

	if v := os.Getenv(envListCacheSec); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return Config{}, fmt.Errorf("%s must be a non-negative integer, got %q", envListCacheSec, v)
		}
		cfg.ListCacheTTL = time.Duration(n) * time.Second
	}

	if v := os.Getenv(envMaxUploadMB); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("%s must be a positive integer, got %q", envMaxUploadMB, v)
		}
		cfg.MaxUploadBytes = n * 1024 * 1024
	}

	if cfg.Addr == "" {
		return Config{}, fmt.Errorf("%s must not be empty", envAddr)
	}
	if cfg.VideoDir == "" {
		return Config{}, fmt.Errorf("%s must not be empty", envVideoDir)
	}

	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
