package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
	_ "time/tzdata"

	"coursework/internal/api"
	"coursework/internal/store"
	"coursework/web"
)

const shutdownTimeout = 10 * time.Second

type config struct {
	databaseURL     string
	listenAddr      string
	reposDir        string
	repoSizeLimitMB int
	gitToken        string
	cookieSecure    bool
}

func main() {
	if err := run(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := os.MkdirAll(cfg.reposDir, 0o755); err != nil {
		return fmt.Errorf("create repos dir: %w", err)
	}

	pool, err := store.Open(ctx, cfg.databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := store.Migrate(ctx, pool); err != nil {
		return err
	}

	spaFiles, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		return fmt.Errorf("open embedded web files: %w", err)
	}

	server := &http.Server{
		Addr:              cfg.listenAddr,
		Handler:           api.NewRouter(pool, cfg.cookieSecure, spaFiles),
		ReadHeaderTimeout: 10 * time.Second,
	}

	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.ListenAndServe()
	}()

	slog.Info("server started",
		"addr", cfg.listenAddr,
		"reposDir", cfg.reposDir,
		"repoSizeLimitMB", cfg.repoSizeLimitMB,
		"gitTokenConfigured", cfg.gitToken != "",
		"cookieSecure", cfg.cookieSecure,
	)

	select {
	case err := <-serverErrors:
		return err
	case <-ctx.Done():
	}

	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	return server.Shutdown(shutdownCtx)
}

func loadConfig() (config, error) {
	cfg := config{
		databaseURL: os.Getenv("DATABASE_URL"),
		listenAddr:  envOrDefault("LISTEN_ADDR", ":8080"),
		reposDir:    envOrDefault("REPOS_DIR", "repos"),
		gitToken:    os.Getenv("GIT_TOKEN"),
	}
	if cfg.databaseURL == "" {
		return config{}, errors.New("DATABASE_URL is required")
	}

	repoSizeLimitMB, err := strconv.Atoi(envOrDefault("REPO_SIZE_LIMIT_MB", "1024"))
	if err != nil {
		return config{}, fmt.Errorf("parse REPO_SIZE_LIMIT_MB: %w", err)
	}
	cfg.repoSizeLimitMB = repoSizeLimitMB

	cookieSecure, err := strconv.ParseBool(envOrDefault("COOKIE_SECURE", "false"))
	if err != nil {
		return config{}, fmt.Errorf("parse COOKIE_SECURE: %w", err)
	}
	cfg.cookieSecure = cookieSecure

	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
