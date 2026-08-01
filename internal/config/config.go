// Package config loads cf-smtp-relay's configuration from environment
// variables only — no config file, no CLI flags (see README's "Design
// decisions").
package config

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Config holds all relay configuration, sourced entirely from environment
// variables (see README's Configuration table).
type Config struct {
	CFAPIToken   string
	CFAccountID  string
	CFAPIBaseURL string

	SMTPListenAddr           string
	SMTPHostname             string
	SMTPAllowedSenderDomains []string // nil = allow all
	SMTPMaxMessageSizeBytes  int64

	LogLevel  string
	LogFormat string

	ShutdownTimeoutSeconds int
}

var validLogLevels = map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
var validLogFormats = map[string]bool{"json": true, "text": true}

// envBindings maps viper keys to the environment variables they read.
var envBindings = map[string]string{
	"cf_api_token":                "CF_API_TOKEN",
	"cf_account_id":               "CF_ACCOUNT_ID",
	"cf_api_base_url":             "CF_API_BASE_URL",
	"smtp_listen_addr":            "SMTP_LISTEN_ADDR",
	"smtp_hostname":               "SMTP_HOSTNAME",
	"smtp_allowed_sender_domains": "SMTP_ALLOWED_SENDER_DOMAINS",
	"smtp_max_message_size_bytes": "SMTP_MAX_MESSAGE_SIZE_BYTES",
	"log_level":                   "LOG_LEVEL",
	"log_format":                  "LOG_FORMAT",
	"shutdown_timeout_seconds":    "SHUTDOWN_TIMEOUT_SECONDS",
}

// Load reads configuration from environment variables, applies defaults, and
// validates the result before returning.
func Load() (*Config, error) {
	v := viper.New() // local instance, not the global viper — keeps tests hermetic

	v.SetDefault("cf_api_base_url", "https://api.cloudflare.com/client/v4")
	v.SetDefault("smtp_listen_addr", ":2525")
	v.SetDefault("smtp_hostname", "localhost")
	v.SetDefault("smtp_max_message_size_bytes", int64(5242880))
	v.SetDefault("log_level", "info")
	v.SetDefault("log_format", "json")
	v.SetDefault("shutdown_timeout_seconds", 10)

	// viper's AutomaticEnv+Unmarshal silently drops env-only keys that
	// aren't explicitly bound, so every key is bound individually instead.
	for key, env := range envBindings {
		if err := v.BindEnv(key, env); err != nil {
			return nil, fmt.Errorf("config: bind %s: %w", env, err)
		}
	}

	cfg := &Config{
		CFAPIToken:               v.GetString("cf_api_token"),
		CFAccountID:              v.GetString("cf_account_id"),
		CFAPIBaseURL:             v.GetString("cf_api_base_url"),
		SMTPListenAddr:           v.GetString("smtp_listen_addr"),
		SMTPHostname:             v.GetString("smtp_hostname"),
		SMTPAllowedSenderDomains: parseDomains(v.GetString("smtp_allowed_sender_domains")),
		SMTPMaxMessageSizeBytes:  v.GetInt64("smtp_max_message_size_bytes"),
		LogLevel:                 strings.ToLower(v.GetString("log_level")),
		LogFormat:                strings.ToLower(v.GetString("log_format")),
		ShutdownTimeoutSeconds:   v.GetInt("shutdown_timeout_seconds"),
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func parseDomains(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var domains []string
	for p := range strings.SplitSeq(raw, ",") {
		p = strings.ToLower(strings.TrimSpace(p))
		if p != "" {
			domains = append(domains, p)
		}
	}
	return domains
}

// Validate reports every configuration problem found, joined into a single
// error, so an operator sees all of them at once instead of fixing one env
// var at a time via repeated restarts.
func (c *Config) Validate() error {
	var errs []error

	if strings.TrimSpace(c.CFAPIToken) == "" {
		errs = append(errs, errors.New("CF_API_TOKEN is required"))
	}
	if strings.TrimSpace(c.CFAccountID) == "" {
		errs = append(errs, errors.New("CF_ACCOUNT_ID is required"))
	}
	if !validLogLevels[c.LogLevel] {
		errs = append(errs, fmt.Errorf("LOG_LEVEL %q is invalid (must be debug, info, warn, or error)", c.LogLevel))
	}
	if !validLogFormats[c.LogFormat] {
		errs = append(errs, fmt.Errorf("LOG_FORMAT %q is invalid (must be json or text)", c.LogFormat))
	}
	if c.SMTPMaxMessageSizeBytes <= 0 {
		errs = append(errs, fmt.Errorf("SMTP_MAX_MESSAGE_SIZE_BYTES must be positive, got %d", c.SMTPMaxMessageSizeBytes))
	}
	if c.ShutdownTimeoutSeconds < 0 {
		errs = append(errs, fmt.Errorf("SHUTDOWN_TIMEOUT_SECONDS must not be negative, got %d", c.ShutdownTimeoutSeconds))
	}

	return errors.Join(errs...)
}
