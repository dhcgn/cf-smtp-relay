package smtpserver

import (
	"log/slog"
	"strings"

	smtp "github.com/emersion/go-smtp"
)

// Config configures the SMTP listener.
type Config struct {
	ListenAddr          string
	Hostname            string
	MaxMessageSizeBytes int64
	// AllowedSenderDomains is a case-insensitive allowlist of From domains.
	// Empty/nil means allow all.
	AllowedSenderDomains []string
}

// NewServer builds a *smtp.Server wired to sender. It does not start
// listening; call ListenAndServe (or Serve) on the result, and Shutdown for
// a graceful stop.
func NewServer(cfg Config, sender CloudflareSender, logger *slog.Logger) *smtp.Server {
	be := &backend{
		sender:               sender,
		logger:               logger,
		allowedSenderDomains: normalizeDomains(cfg.AllowedSenderDomains),
	}

	s := smtp.NewServer(be)
	s.Addr = cfg.ListenAddr
	s.Domain = cfg.Hostname
	s.MaxMessageBytes = cfg.MaxMessageSizeBytes
	// M1 supports a single recipient; go-smtp enforces this itself with a
	// 452 4.5.3 reply to a second RCPT TO before Session.Rcpt runs again.
	s.MaxRecipients = 1

	return s
}

func normalizeDomains(domains []string) map[string]struct{} {
	if len(domains) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(domains))
	for _, d := range domains {
		d = strings.ToLower(strings.TrimSpace(d))
		if d == "" {
			continue
		}
		set[d] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	return set
}
