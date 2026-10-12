// Command server runs the KerjaLancar web application.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"kerjalancar/internal/config"
	"kerjalancar/internal/db"
	"kerjalancar/internal/web"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "kerjalancar:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer pool.Close()

	addr := net.JoinHostPort("", strconv.Itoa(cfg.Port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}

	srv := &http.Server{
		Handler:           web.NewRouter(web.Deps{Logger: logger, DB: pool}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	logger.Info("server listening", slog.String("addr", ln.Addr().String()), slog.String("env", cfg.Env))
	err = web.Serve(ctx, srv, ln, 10*time.Second)
	logger.Info("server stopped")
	return err
}
