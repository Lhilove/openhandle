package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"openhandle/internal/api"
	"openhandle/internal/config"
	"openhandle/internal/db"
	"openhandle/internal/rpc"
	"openhandle/internal/store"
)

func main() {
	log := newLogger()

	cfg, err := config.Load()
	if err != nil {
		log.Error("configuration error", "err", err)
		os.Exit(1)
	}

	ctx := context.Background()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("database connection failed", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		log.Error("migrations failed", "err", err)
		os.Exit(1)
	}
	log.Info("migrations applied")

	st := store.New(pool)

	if strings.EqualFold(os.Getenv("SEED_DEMO"), "true") && !cfg.IsProduction() {
		if err := seedDemo(ctx, st, log); err != nil {
			log.Warn("demo seed skipped", "err", err)
		}
	}

	rpcClient := rpc.New(cfg.SolanaRPCURL, cfg.SolanaCluster)
	srv := api.NewServer(cfg, st, log, rpcClient)
	handler := api.Router(cfg, st, log, rpcClient)

	// Background workers: confirm pending transactions and prune stale rows.
	workerCtx, stopWorkers := context.WithCancel(ctx)
	defer stopWorkers()
	go runWorkers(workerCtx, srv, st, log)

	httpSrv := &http.Server{
		Addr:              listenAddr(),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Info("listening", "addr", httpSrv.Addr, "env", cfg.Env, "cluster", cfg.SolanaCluster)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	// Graceful shutdown.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown error", "err", err)
	}
}

func listenAddr() string {
	p := strings.TrimSpace(os.Getenv("PORT"))
	// Treat an empty or "0" PORT as unset: port 0 requests a random port, which
	// is never what a dev server should silently do.
	if p != "" && p != "0" {
		return ":" + p
	}
	return ":8080"
}

func newLogger() *slog.Logger {
	level := slog.LevelInfo
	switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}

func runWorkers(ctx context.Context, srv *api.Server, st *store.Store, log *slog.Logger) {
	confirm := time.NewTicker(15 * time.Second)
	prune := time.NewTicker(1 * time.Hour)
	defer confirm.Stop()
	defer prune.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-confirm.C:
			srv.ConfirmPending(ctx)
		case <-prune.C:
			if err := st.DeleteExpiredSessions(ctx); err != nil {
				log.Warn("prune sessions", "err", err)
			}
			if err := st.DeleteExpiredChallenges(ctx); err != nil {
				log.Warn("prune challenges", "err", err)
			}
		}
	}
}
