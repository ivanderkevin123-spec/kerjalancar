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
	_ "time/tzdata" // workspace time zones must resolve inside a minimal container

	"kerjalancar/internal/auth"
	"kerjalancar/internal/config"
	"kerjalancar/internal/customer"
	"kerjalancar/internal/db"
	"kerjalancar/internal/lead"
	"kerjalancar/internal/web"
	"kerjalancar/internal/workspace"
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

	templates, err := web.LoadTemplates()
	if err != nil {
		return err
	}
	deps := web.Deps{
		Logger:           logger,
		DB:               pool,
		TrustProxy:       cfg.TrustedProxy,
		Sessions:         web.NewSessionManager(pool, cfg.Env == "production", 30*time.Minute),
		Auth:             auth.NewService(pool, auth.DefaultParams),
		Workspaces:       workspace.NewStore(pool),
		Customers:        customer.NewStore(pool),
		Leads:            lead.NewStore(pool),
		Templates:        templates,
		RegistrationOpen: cfg.RegistrationOpen,
		RegistrationCode: cfg.RegistrationCode,
		BaseURL:          cfg.BaseURL,
	}

	srv := &http.Server{
		Handler:           web.NewRouter(deps),
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
