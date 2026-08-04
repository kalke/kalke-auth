package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kalke/kalke-auth/internal/config"
	"github.com/kalke/kalke-auth/internal/db"
	"github.com/kalke/kalke-auth/internal/httpapi"
	"github.com/kalke/kalke-auth/internal/keycloak"
	"github.com/kalke/kalke-auth/internal/mail"
	"github.com/kalke/kalke-auth/internal/migrate"
	"github.com/kalke/kalke-auth/internal/store"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	cfg, err := config.Load()
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL, cfg.DBSearchPath)
	if err != nil {
		log.Error("db", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := migrate.Up(ctx, pool); err != nil {
		log.Error("migrate", "err", err)
		os.Exit(1)
	}
	if err := db.EnsureAppTables(ctx, pool); err != nil {
		log.Error("schema", "err", err)
		os.Exit(1)
	}

	rdb := httpapi.NewRedis(cfg)
	defer func() { _ = rdb.Close() }()

	mailer := newMailer(cfg, log)
	kc := keycloak.New(cfg.KCInternalURL, cfg.KCPublicIssuer, cfg.BFFClientID, cfg.BFFClientSecret)
	admin := keycloak.NewAdmin(cfg.KCInternalURL, cfg.KCAdminUser, cfg.KCAdminPassword)
	srv := httpapi.New(cfg, store.New(pool), kc, admin, rdb, mailer, log)

	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       120 * time.Second,
		WriteTimeout:      180 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Info("listening", "addr", cfg.HTTPAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("http", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
}

func newMailer(cfg config.Config, log *slog.Logger) mail.Mailer {
	if cfg.MailDevLog {
		return mail.LogMailer{Log: log}
	}
	if cfg.MailgunAPIKey != "" {
		return mail.NewMailgun(cfg.MailgunAPIKey, cfg.MailgunDomain, cfg.MailFrom)
	}
	if cfg.ResendAPIKey != "" {
		return mail.NewResend(cfg.ResendAPIKey, cfg.MailFrom)
	}
	return mail.LogMailer{Log: log}
}
