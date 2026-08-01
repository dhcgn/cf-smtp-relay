package smtpserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"strings"

	smtp "github.com/emersion/go-smtp"

	"github.com/dhcgn/cf-smtp-relay/internal/cfclient"
)

type session struct {
	backend *backend
	from    string
	to      string
}

func (s *session) Mail(from string, opts *smtp.MailOptions) error {
	addr, err := mail.ParseAddress(from)
	if err != nil {
		return &smtp.SMTPError{Code: 553, EnhancedCode: smtp.EnhancedCode{5, 1, 7}, Message: "malformed From address"}
	}

	if s.backend.allowedSenderDomains != nil {
		if _, ok := s.backend.allowedSenderDomains[domainOf(addr.Address)]; !ok {
			return &smtp.SMTPError{Code: 550, EnhancedCode: smtp.EnhancedCode{5, 7, 1}, Message: "sender domain not allowed"}
		}
	}

	s.from = addr.Address
	return nil
}

func (s *session) Rcpt(to string, opts *smtp.RcptOptions) error {
	addr, err := mail.ParseAddress(to)
	if err != nil {
		return &smtp.SMTPError{Code: 553, EnhancedCode: smtp.EnhancedCode{5, 1, 3}, Message: "malformed recipient address"}
	}
	// go-smtp's own MaxRecipients=1 setting already rejects a second RCPT TO
	// with 452 4.5.3 before this method is invoked again.
	s.to = addr.Address
	return nil
}

func (s *session) Data(r io.Reader) error {
	raw, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("smtpserver: read DATA: %w", err)
	}

	parsed, err := ParseMessage(raw, s.backend.logger)
	if err != nil {
		if errors.Is(err, ErrUnsupportedContent) {
			return &smtp.SMTPError{Code: 550, EnhancedCode: smtp.EnhancedCode{5, 6, 1}, Message: "message has no supported text or HTML content"}
		}
		if s.backend.logger != nil {
			s.backend.logger.Warn("failed to parse message", "error", err)
		}
		return &smtp.SMTPError{Code: 550, EnhancedCode: smtp.EnhancedCode{5, 6, 0}, Message: "malformed message"}
	}

	req := cfclient.SendEmailRequest{
		From:    s.from,
		To:      s.to,
		Subject: parsed.Subject,
		Text:    parsed.Text,
		HTML:    parsed.HTML,
	}

	if _, err := s.backend.sender.SendEmail(context.Background(), req); err != nil {
		if s.backend.logger != nil {
			s.backend.logger.Error("cloudflare send failed", "error", err, "from", s.from, "to", s.to)
		}
		return toSMTPError(err)
	}

	return nil
}

func (s *session) Reset() {
	s.from = ""
	s.to = ""
}

func (s *session) Logout() error {
	return nil
}

func domainOf(addr string) string {
	i := strings.LastIndex(addr, "@")
	if i < 0 {
		return ""
	}
	return strings.ToLower(addr[i+1:])
}
