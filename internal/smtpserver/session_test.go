package smtpserver

import (
	"context"
	"net"
	"net/smtp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dhcgn/cf-smtp-relay/internal/cfclient"
)

// fakeSender implements CloudflareSender for tests, avoiding real HTTP calls.
type fakeSender struct {
	mu      sync.Mutex
	lastReq cfclient.SendEmailRequest
	called  bool
	err     error
	result  *cfclient.SendResult
}

func (f *fakeSender) SendEmail(ctx context.Context, req cfclient.SendEmailRequest) (*cfclient.SendResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.called = true
	f.lastReq = req
	if f.err != nil {
		return nil, f.err
	}
	if f.result != nil {
		return f.result, nil
	}
	return &cfclient.SendResult{Delivered: []string{req.To}}, nil
}

func startTestServer(t *testing.T, cfg Config, sender CloudflareSender) (addr string, stop func()) {
	t.Helper()
	if cfg.Hostname == "" {
		cfg.Hostname = "test.local"
	}
	if cfg.MaxMessageSizeBytes == 0 {
		cfg.MaxMessageSizeBytes = 1024 * 1024
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}

	srv := NewServer(cfg, sender, nil)
	go func() {
		_ = srv.Serve(ln)
	}()

	stop = func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}
	return ln.Addr().String(), stop
}

const testMessage = "Subject: hi\r\n" +
	"Content-Type: text/plain\r\n" +
	"\r\n" +
	"hello world\r\n"

func TestSMTPSession_HappyPath(t *testing.T) {
	sender := &fakeSender{}
	addr, stop := startTestServer(t, Config{}, sender)
	defer stop()

	c, err := smtp.Dial(addr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	if err := c.Mail("from@example.com"); err != nil {
		t.Fatalf("MAIL FROM: %v", err)
	}
	if err := c.Rcpt("to@example.com"); err != nil {
		t.Fatalf("RCPT TO: %v", err)
	}
	wc, err := c.Data()
	if err != nil {
		t.Fatalf("DATA: %v", err)
	}
	if _, err := wc.Write([]byte(testMessage)); err != nil {
		t.Fatalf("write message: %v", err)
	}
	if err := wc.Close(); err != nil {
		t.Fatalf("DATA close (expected 250): %v", err)
	}

	sender.mu.Lock()
	defer sender.mu.Unlock()
	if !sender.called {
		t.Fatal("expected sender.SendEmail to be called")
	}
	if sender.lastReq.From != "from@example.com" || sender.lastReq.To != "to@example.com" {
		t.Errorf("unexpected request: %+v", sender.lastReq)
	}
	if sender.lastReq.Text != "hello world\r\n" {
		t.Errorf("Text = %q, want %q", sender.lastReq.Text, "hello world\r\n")
	}
}

func TestSMTPSession_CloudflareTemporaryErrorSurfacesOnData(t *testing.T) {
	sender := &fakeSender{err: &cfclient.SendError{
		Temporary:     true,
		SMTPCode:      450,
		EnhancedCode:  [3]int{4, 7, 1},
		PublicMessage: "rate limit exceeded, please retry",
	}}
	addr, stop := startTestServer(t, Config{}, sender)
	defer stop()

	c, err := smtp.Dial(addr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	if err := c.Mail("from@example.com"); err != nil {
		t.Fatalf("MAIL FROM: %v", err)
	}
	if err := c.Rcpt("to@example.com"); err != nil {
		t.Fatalf("RCPT TO: %v", err)
	}
	wc, err := c.Data()
	if err != nil {
		t.Fatalf("DATA: %v", err)
	}
	_, _ = wc.Write([]byte(testMessage))
	err = wc.Close()
	if err == nil {
		t.Fatal("expected an error closing DATA, got nil")
	}
	if !strings.Contains(err.Error(), "450") {
		t.Errorf("expected SMTP 450 in error, got: %v", err)
	}
}

func TestSMTPSession_SecondRecipientRejected(t *testing.T) {
	sender := &fakeSender{}
	addr, stop := startTestServer(t, Config{}, sender)
	defer stop()

	c, err := smtp.Dial(addr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	if err := c.Mail("from@example.com"); err != nil {
		t.Fatalf("MAIL FROM: %v", err)
	}
	if err := c.Rcpt("to1@example.com"); err != nil {
		t.Fatalf("first RCPT TO: %v", err)
	}
	err = c.Rcpt("to2@example.com")
	if err == nil {
		t.Fatal("expected second RCPT TO to be rejected, got nil error")
	}
	if !strings.Contains(err.Error(), "452") {
		t.Errorf("expected SMTP 452 for second recipient, got: %v", err)
	}
}

func TestSMTPSession_OversizedMessageRejected(t *testing.T) {
	sender := &fakeSender{}
	addr, stop := startTestServer(t, Config{MaxMessageSizeBytes: 16}, sender)
	defer stop()

	c, err := smtp.Dial(addr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	if err := c.Mail("from@example.com"); err != nil {
		t.Fatalf("MAIL FROM: %v", err)
	}
	if err := c.Rcpt("to@example.com"); err != nil {
		t.Fatalf("RCPT TO: %v", err)
	}
	wc, err := c.Data()
	if err != nil {
		t.Fatalf("DATA: %v", err)
	}
	_, _ = wc.Write([]byte(testMessage))
	err = wc.Close()
	if err == nil {
		t.Fatal("expected oversized message to be rejected, got nil")
	}
	if !strings.Contains(err.Error(), "552") {
		t.Errorf("expected SMTP 552 for oversized message, got: %v", err)
	}
}

func TestSMTPSession_DisallowedSenderDomainRejectedAtMailFrom(t *testing.T) {
	sender := &fakeSender{}
	addr, stop := startTestServer(t, Config{AllowedSenderDomains: []string{"allowed.example"}}, sender)
	defer stop()

	c, err := smtp.Dial(addr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	err = c.Mail("from@not-allowed.example")
	if err == nil {
		t.Fatal("expected MAIL FROM to be rejected for disallowed domain, got nil")
	}
	if !strings.Contains(err.Error(), "550") {
		t.Errorf("expected SMTP 550 for disallowed sender domain, got: %v", err)
	}

	sender.mu.Lock()
	defer sender.mu.Unlock()
	if sender.called {
		t.Error("SendEmail should not have been called")
	}
}

const testMessageWithAttachment = "Subject: has attachment\r\n" +
	"Content-Type: multipart/mixed; boundary=BOUND\r\n" +
	"\r\n" +
	"--BOUND\r\n" +
	"Content-Type: text/plain\r\n" +
	"\r\n" +
	"hello world\r\n" +
	"--BOUND\r\n" +
	"Content-Type: text/plain\r\n" +
	"Content-Transfer-Encoding: base64\r\n" +
	"Content-Disposition: attachment; filename=\"notes.txt\"\r\n" +
	"\r\n" +
	"aGVsbG8=\r\n" +
	"--BOUND--\r\n"

func TestSMTPSession_AttachmentForwarded(t *testing.T) {
	sender := &fakeSender{}
	addr, stop := startTestServer(t, Config{}, sender)
	defer stop()

	c, err := smtp.Dial(addr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	if err := c.Mail("from@example.com"); err != nil {
		t.Fatalf("MAIL FROM: %v", err)
	}
	if err := c.Rcpt("to@example.com"); err != nil {
		t.Fatalf("RCPT TO: %v", err)
	}
	wc, err := c.Data()
	if err != nil {
		t.Fatalf("DATA: %v", err)
	}
	if _, err := wc.Write([]byte(testMessageWithAttachment)); err != nil {
		t.Fatalf("write message: %v", err)
	}
	if err := wc.Close(); err != nil {
		t.Fatalf("DATA close (expected 250): %v", err)
	}

	sender.mu.Lock()
	defer sender.mu.Unlock()
	if len(sender.lastReq.Attachments) != 1 {
		t.Fatalf("Attachments = %+v, want 1 entry", sender.lastReq.Attachments)
	}
	got := sender.lastReq.Attachments[0]
	if got.Filename != "notes.txt" {
		t.Errorf("Filename = %q, want %q", got.Filename, "notes.txt")
	}
	if got.ContentType != "text/plain" {
		t.Errorf("ContentType = %q, want %q", got.ContentType, "text/plain")
	}
	if string(got.Content) != "hello" {
		t.Errorf("Content = %q, want %q", got.Content, "hello")
	}
}
