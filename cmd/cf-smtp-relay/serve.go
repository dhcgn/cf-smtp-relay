package main

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/dhcgn/cf-smtp-relay/internal/cfclient"
	"github.com/dhcgn/cf-smtp-relay/internal/config"
	"github.com/dhcgn/cf-smtp-relay/internal/smtpserver"
)

func serveCommand() *cli.Command {
	return &cli.Command{
		Name:   "serve",
		Usage:  "start the SMTP-to-Cloudflare relay",
		Action: runServe,
	}
}

func runServe(ctx context.Context, cmd *cli.Command) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := newLogger(cfg)
	logger.Info("starting cf-smtp-relay",
		"version", version, "commit", commit, "build_time", buildTime,
		"listen_addr", cfg.SMTPListenAddr, "hostname", cfg.SMTPHostname)

	cfClient := cfclient.New(cfclient.Config{
		APIToken:  cfg.CFAPIToken,
		AccountID: cfg.CFAccountID,
		BaseURL:   cfg.CFAPIBaseURL,
	})

	srv := smtpserver.NewServer(smtpserver.Config{
		ListenAddr:           cfg.SMTPListenAddr,
		Hostname:             cfg.SMTPHostname,
		MaxMessageSizeBytes:  cfg.SMTPMaxMessageSizeBytes,
		AllowedSenderDomains: cfg.SMTPAllowedSenderDomains,
	}, cfClient, logger)

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		if err != nil {
			return fmt.Errorf("smtp server: %w", err)
		}
		return nil
	case <-ctx.Done():
		logger.Info("shutdown signal received, draining connections", "timeout_seconds", cfg.ShutdownTimeoutSeconds)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.ShutdownTimeoutSeconds)*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		logger.Info("shutdown complete")
		return nil
	}
}
