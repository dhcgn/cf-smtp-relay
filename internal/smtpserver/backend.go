package smtpserver

import (
	"context"
	"log/slog"

	smtp "github.com/emersion/go-smtp"

	"github.com/dhcgn/cf-smtp-relay/internal/cfclient"
)

// CloudflareSender is the subset of *cfclient.Client that smtpserver depends
// on, so tests can substitute a fake instead of making real HTTP calls.
type CloudflareSender interface {
	SendEmail(ctx context.Context, req cfclient.SendEmailRequest) (*cfclient.SendResult, error)
}

var _ CloudflareSender = (*cfclient.Client)(nil)

type backend struct {
	sender               CloudflareSender
	logger               *slog.Logger
	allowedSenderDomains map[string]struct{} // nil = allow all
}

func (b *backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	return &session{backend: b}, nil
}
