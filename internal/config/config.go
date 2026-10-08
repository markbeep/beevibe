// Package config parses the process environment into the settings described in
// plans/1-techstack.md §7.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds every runtime setting. Durations are already converted.
type Config struct {
	// Required.
	AdminPassword string
	SessionSecret []byte

	// Optional secrets: an empty DeepSeekAPIKey is tolerated at startup — the
	// app runs and every agent run fails with the canned error message (D14).
	DeepSeekAPIKey string

	Port string
	// DataDir holds beevibe.db, rooms/ and templates/.
	DataDir string
	// WhisperModel is the ggml model file loaded once at startup.
	WhisperModel string
	// RendererURL is the base URL of the preview renderer service.
	RendererURL string
	// AppURL is the base URL the renderer uses to reach the app's static route.
	AppURL string
	// AgentModel is the model new rooms are created with.
	AgentModel string

	AgentMaxSteps     int
	AgentContextTurns int
	MaxUsersPerRoom   int
	MaxRooms          int
	MaxConcurrentRuns int
	PreviewDebounce   time.Duration
	AgentRunTimeout   time.Duration
}

// Load reads the environment, applying the documented defaults. It fails only
// when ADMIN_PASSWORD or SESSION_SECRET is unset.
func Load() (*Config, error) {
	cfg := &Config{
		AdminPassword:     os.Getenv("ADMIN_PASSWORD"),
		SessionSecret:     []byte(os.Getenv("SESSION_SECRET")),
		DeepSeekAPIKey:    os.Getenv("DEEPSEEK_API_KEY"),
		Port:              envOr("PORT", "8080"),
		DataDir:           envOr("DATA_DIR", "./data"),
		WhisperModel:      envOr("WHISPER_MODEL", "models/ggml-base.en.bin"),
		RendererURL:       envOr("RENDERER_URL", "http://renderer:9000"),
		AppURL:            envOr("APP_URL", "http://backend:8080"),
		AgentModel:        envOr("AGENT_MODEL", "deepseek-flash"),
		AgentMaxSteps:     envInt("AGENT_MAX_STEPS", 20),
		AgentContextTurns: envInt("AGENT_CONTEXT_TURNS", 50),
		MaxUsersPerRoom:   envInt("MAX_USERS_PER_ROOM", 100),
		MaxRooms:          envInt("MAX_ROOMS", 20),
		MaxConcurrentRuns: envInt("MAX_CONCURRENT_RUNS", 8),
		PreviewDebounce:   time.Duration(envInt("PREVIEW_DEBOUNCE_MS", 2000)) * time.Millisecond,
		AgentRunTimeout:   time.Duration(envInt("AGENT_RUN_TIMEOUT_S", 120)) * time.Second,
	}

	if cfg.AdminPassword == "" {
		return nil, fmt.Errorf("config: ADMIN_PASSWORD is required")
	}
	if len(cfg.SessionSecret) == 0 {
		return nil, fmt.Errorf("config: SESSION_SECRET is required")
	}
	// The renderer URL is used as a base for path joins; a trailing slash would
	// produce "//capture".
	cfg.RendererURL = strings.TrimRight(cfg.RendererURL, "/")
	if _, err := url.Parse(cfg.RendererURL); err != nil {
		return nil, fmt.Errorf("config: RENDERER_URL: %w", err)
	}
	return cfg, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
