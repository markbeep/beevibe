// Command beevibe is the HTTP + WebSocket backend: rooms, users, static sites,
// speech-to-text, the per-user agent and the preview coordinator.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/mark/beevibe/internal/agent"
	"github.com/mark/beevibe/internal/auth"
	"github.com/mark/beevibe/internal/config"
	"github.com/mark/beevibe/internal/core"
	"github.com/mark/beevibe/internal/db"
	"github.com/mark/beevibe/internal/files"
	"github.com/mark/beevibe/internal/httpapi"
	"github.com/mark/beevibe/internal/preview"
	"github.com/mark/beevibe/internal/stt"

	"github.com/zendev-sh/goai/provider"
	"github.com/zendev-sh/goai/provider/deepseek"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "beevibe:", err)
		os.Exit(1)
	}
}

func run() error {
	logger, err := zap.NewProduction()
	if err != nil {
		return err
	}
	defer func() { _ = logger.Sync() }()

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.DeepSeekAPIKey == "" {
		// The app stays fully usable without a key; only agent runs fail (D14).
		logger.Error("config: DEEPSEEK_API_KEY is not set — every agent run will fail")
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return fmt.Errorf("data directory: %w", err)
	}

	ctx := context.Background()
	handle, err := db.Open(ctx, filepath.Join(cfg.DataDir, "beevibe.db"))
	if err != nil {
		return err
	}
	defer func() { _ = handle.Close() }()

	q := db.Queries(handle)
	// Runs left queued/running by a crash are marked failed (schema.md §3).
	if err := q.FailStaleAgentRuns(ctx, sql.NullInt64{Int64: time.Now().Unix(), Valid: true}); err != nil {
		return fmt.Errorf("mark stale agent runs: %w", err)
	}

	store := files.New(cfg.DataDir)
	transcriber, err := stt.New(cfg.WhisperModel)
	if err != nil {
		return fmt.Errorf("speech-to-text: %w", err)
	}
	defer func() { _ = transcriber.Close() }()

	sessions := auth.New(cfg.SessionSecret)
	c := core.New(cfg, handle, q, store, logger)

	renderer := preview.NewClient(cfg.RendererURL)
	previews := preview.NewManager(cfg, renderer, func(roomID string, userID int64) {
		c.EmitUserUpdate(context.Background(), roomID, userID)
	})
	agents := agent.New(agent.Options{
		Cfg:    cfg,
		Q:      q,
		Files:  store,
		Events: c,
		Log:    logger,
		NewModel: func(modelID string) provider.LanguageModel {
			return deepseek.Chat(modelID, deepseek.WithAPIKey(cfg.DeepSeekAPIKey))
		},
		SystemPrompt: agent.SystemPrompt,
	})
	c.Attach(agents, previews)

	api := httpapi.New(cfg, handle, q, sessions, c, transcriber, renderer, logger)
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           api.Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.ListenAndServe() }()
	logger.Info("beevibe listening",
		zap.String("port", cfg.Port),
		zap.String("dataDir", cfg.DataDir),
		zap.String("renderer", cfg.RendererURL),
		zap.String("model", cfg.AgentModel),
	)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case sig := <-signals:
		logger.Info("shutdown requested", zap.String("signal", sig.String()))
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("shutdown: server", zap.Error(err))
		}
		// Close WebSockets with a normal close frame and mark running agent runs
		// failed via FinishAgentRun.
		c.Shutdown()
	}
	return nil
}
