package config

import (
	"strings"
	"testing"
)

func setRequired(t *testing.T) {
	t.Helper()
	t.Setenv("CF_API_TOKEN", "token123")
	t.Setenv("CF_ACCOUNT_ID", "acct123")
}

func TestLoad_MissingRequiredFields(t *testing.T) {
	cases := []struct {
		name        string
		setToken    bool
		setAccount  bool
		wantSubstrs []string
	}{
		{"missing_token", false, true, []string{"CF_API_TOKEN"}},
		{"missing_account", true, false, []string{"CF_ACCOUNT_ID"}},
		{"missing_both", false, false, []string{"CF_API_TOKEN", "CF_ACCOUNT_ID"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setToken {
				t.Setenv("CF_API_TOKEN", "token123")
			}
			if tc.setAccount {
				t.Setenv("CF_ACCOUNT_ID", "acct123")
			}

			_, err := Load()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			for _, s := range tc.wantSubstrs {
				if !strings.Contains(err.Error(), s) {
					t.Errorf("error %q does not mention %q", err.Error(), s)
				}
			}
		})
	}
}

func TestLoad_InvalidLogLevel(t *testing.T) {
	setRequired(t)
	t.Setenv("LOG_LEVEL", "verbose")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid LOG_LEVEL, got nil")
	}
	if !strings.Contains(err.Error(), "LOG_LEVEL") {
		t.Errorf("error %q does not mention LOG_LEVEL", err.Error())
	}
}

func TestLoad_InvalidLogFormat(t *testing.T) {
	setRequired(t)
	t.Setenv("LOG_FORMAT", "xml")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid LOG_FORMAT, got nil")
	}
	if !strings.Contains(err.Error(), "LOG_FORMAT") {
		t.Errorf("error %q does not mention LOG_FORMAT", err.Error())
	}
}

func TestLoad_NonPositiveMaxMessageSize(t *testing.T) {
	setRequired(t)
	t.Setenv("SMTP_MAX_MESSAGE_SIZE_BYTES", "0")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for non-positive SMTP_MAX_MESSAGE_SIZE_BYTES, got nil")
	}
	if !strings.Contains(err.Error(), "SMTP_MAX_MESSAGE_SIZE_BYTES") {
		t.Errorf("error %q does not mention SMTP_MAX_MESSAGE_SIZE_BYTES", err.Error())
	}
}

func TestLoad_NegativeShutdownTimeout(t *testing.T) {
	setRequired(t)
	t.Setenv("SHUTDOWN_TIMEOUT_SECONDS", "-1")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for negative SHUTDOWN_TIMEOUT_SECONDS, got nil")
	}
	if !strings.Contains(err.Error(), "SHUTDOWN_TIMEOUT_SECONDS") {
		t.Errorf("error %q does not mention SHUTDOWN_TIMEOUT_SECONDS", err.Error())
	}
}

func TestLoad_DefaultsApplied(t *testing.T) {
	setRequired(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if cfg.CFAPIBaseURL != "https://api.cloudflare.com/client/v4" {
		t.Errorf("CFAPIBaseURL = %q, want default", cfg.CFAPIBaseURL)
	}
	if cfg.SMTPListenAddr != ":2525" {
		t.Errorf("SMTPListenAddr = %q, want :2525", cfg.SMTPListenAddr)
	}
	if cfg.SMTPHostname != "localhost" {
		t.Errorf("SMTPHostname = %q, want localhost", cfg.SMTPHostname)
	}
	if cfg.SMTPMaxMessageSizeBytes != 5242880 {
		t.Errorf("SMTPMaxMessageSizeBytes = %d, want 5242880", cfg.SMTPMaxMessageSizeBytes)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want info", cfg.LogLevel)
	}
	if cfg.LogFormat != "json" {
		t.Errorf("LogFormat = %q, want json", cfg.LogFormat)
	}
	if cfg.ShutdownTimeoutSeconds != 10 {
		t.Errorf("ShutdownTimeoutSeconds = %d, want 10", cfg.ShutdownTimeoutSeconds)
	}
	if cfg.SMTPAllowedSenderDomains != nil {
		t.Errorf("SMTPAllowedSenderDomains = %v, want nil (allow all)", cfg.SMTPAllowedSenderDomains)
	}
}

func TestLoad_AllowedSenderDomainsSplitAndTrimmed(t *testing.T) {
	setRequired(t)
	t.Setenv("SMTP_ALLOWED_SENDER_DOMAINS", " Example.com , other.example ,,")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	want := []string{"example.com", "other.example"}
	if len(cfg.SMTPAllowedSenderDomains) != len(want) {
		t.Fatalf("SMTPAllowedSenderDomains = %v, want %v", cfg.SMTPAllowedSenderDomains, want)
	}
	for i, d := range want {
		if cfg.SMTPAllowedSenderDomains[i] != d {
			t.Errorf("SMTPAllowedSenderDomains[%d] = %q, want %q", i, cfg.SMTPAllowedSenderDomains[i], d)
		}
	}
}
