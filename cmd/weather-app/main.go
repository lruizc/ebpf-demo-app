package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/luisruiz/weather-app/internal/cache"
	"github.com/luisruiz/weather-app/internal/config"
	"github.com/luisruiz/weather-app/internal/obs"
	"github.com/luisruiz/weather-app/internal/server"
	"github.com/luisruiz/weather-app/internal/weather"
)

// version is overridden at build time via -ldflags "-X main.version=<VERSION>".
var version = "dev"

// Constitution v1.0.1 — Principle IV (eBPF Narrative Integrity):
// The ONLY outbound TCP dials this process makes are:
//   (a) cfg.RedisAddr    — cache reads/writes for non-bypass cities.
//   (b) cfg.OpenMeteoURL — weather fetches for cache misses and bypass cities.
// No background goroutines, no telemetry, no update checks, no pre-warming.

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("configuration error", "err", err)
		os.Exit(1)
	}

	log := obs.NewLogger(cfg.LogLevel)

	// Attempt to connect to Redis; fall back to degraded mode (nil store) if all attempts fail.
	// The initContainer in the Deployment already waits for Redis, so this loop is a safety net
	// for local runs or environments without initContainers.
	const redisMaxAttempts = 10
	var cacheStore *cache.Store
	for attempt := 1; attempt <= redisMaxAttempts; attempt++ {
		cacheStore, err = cache.New(cfg.RedisAddr, log)
		if err == nil {
			break
		}
		log.Warn("redis_unavailable",
			"addr", cfg.RedisAddr,
			"attempt", attempt,
			"err", err.Error(),
		)
		if attempt < redisMaxAttempts {
			time.Sleep(3 * time.Second)
		}
	}
	if err != nil {
		log.Warn("starting_degraded_no_cache", "reason", err.Error())
		cacheStore = nil
	}

	wc := weather.New(cfg.OpenMeteoURL, nil)
	m := server.NewMetrics()

	h, err := server.NewHandlers(cfg, cacheStore, wc, m, log, version)
	if err != nil {
		log.Error("init handlers", "err", err)
		os.Exit(1)
	}

	mux, err := server.NewMux(h, m)
	if err != nil {
		log.Error("init mux", "err", err)
		os.Exit(1)
	}

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	done := make(chan struct{})
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
		<-quit
		log.Info("shutting_down")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Error("shutdown error", "err", err)
		}
		close(done)
	}()

	log.Info("listening", "addr", cfg.ListenAddr, "version", version)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Error("server error", "err", err)
		os.Exit(1)
	}
	<-done
}
