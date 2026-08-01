package smtpserver

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"strings"
)

// maxMIMEDepth guards against pathologically nested multipart messages.
const maxMIMEDepth = 5

// ErrUnsupportedContent is returned when a message has no text/plain or
// text/html content at all (e.g. an attachment-only message).
var ErrUnsupportedContent = errors.New("smtpserver: message has no text or HTML body")

// ParsedMessage is the subset of an RFC 5322 message the M1 MVP forwards to
// Cloudflare: Subject plus plain-text and/or HTML bodies. Attachments are
// dropped rather than supported (logged at warn) — see the README's M1
// design decisions.
type ParsedMessage struct {
	Subject string
	Text    string
	HTML    string
}

// headerGetter is satisfied by both mail.Header and textproto.MIMEHeader, so
// the same extraction code handles the top-level message and every nested
// multipart.Part.
type headerGetter interface {
	Get(string) string
}

// ParseMessage parses a raw RFC 5322 message and extracts the Subject plus
// text/plain and/or text/html bodies, recursing into multipart content and
// dropping any non-text part. logger may be nil; when non-nil, dropped parts
// and unrecognized charsets are logged.
func ParseMessage(raw []byte, logger *slog.Logger) (*ParsedMessage, error) {
	m, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("smtpserver: parse message: %w", err)
	}

	text, html, err := extractContent(m.Header, m.Body, 0, logger)
	if err != nil {
		return nil, fmt.Errorf("smtpserver: extract content: %w", err)
	}
	if text == "" && html == "" {
		return nil, ErrUnsupportedContent
	}

	return &ParsedMessage{
		Subject: decodeSubject(m.Header.Get("Subject")),
		Text:    text,
		HTML:    html,
	}, nil
}

func decodeSubject(raw string) string {
	if raw == "" {
		return ""
	}
	decoded, err := (&mime.WordDecoder{}).DecodeHeader(raw)
	if err != nil {
		return raw
	}
	return decoded
}

func extractContent(header headerGetter, body io.Reader, depth int, logger *slog.Logger) (text, html string, err error) {
	if depth > maxMIMEDepth {
		if logger != nil {
			logger.Warn("dropping deeply nested MIME part", "depth", depth)
		}
		return "", "", nil
	}

	mediaType, params, err := mime.ParseMediaType(header.Get("Content-Type"))
	if err != nil || mediaType == "" {
		// RFC 2045 default when Content-Type is absent or unparseable.
		mediaType = "text/plain"
		params = map[string]string{}
	}

	if strings.HasPrefix(mediaType, "multipart/") {
		return extractMultipart(mediaType, params, body, depth, logger)
	}

	decoded, err := decodeCTE(header.Get("Content-Transfer-Encoding"), body)
	if err != nil {
		return "", "", fmt.Errorf("decode content-transfer-encoding: %w", err)
	}

	switch mediaType {
	case "text/plain":
		logUnrecognizedCharset(params, logger)
		return string(decoded), "", nil
	case "text/html":
		logUnrecognizedCharset(params, logger)
		return "", string(decoded), nil
	default:
		if logger != nil {
			logger.Warn("dropping unsupported message part", "content_type", mediaType)
		}
		return "", "", nil
	}
}

func extractMultipart(mediaType string, params map[string]string, body io.Reader, depth int, logger *slog.Logger) (text, html string, err error) {
	boundary := params["boundary"]
	if boundary == "" {
		return "", "", fmt.Errorf("%s content missing boundary", mediaType)
	}

	mr := multipart.NewReader(body, boundary)
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return text, html, fmt.Errorf("read multipart part: %w", err)
		}

		partText, partHTML, err := extractContent(part.Header, part, depth+1, logger)
		if err != nil {
			return text, html, err
		}
		if partText != "" && text == "" {
			text = partText
		}
		if partHTML != "" && html == "" {
			html = partHTML
		}
	}
	return text, html, nil
}

func decodeCTE(cte string, body io.Reader) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(cte)) {
	case "quoted-printable":
		return io.ReadAll(quotedprintable.NewReader(body))
	case "base64":
		return io.ReadAll(base64.NewDecoder(base64.StdEncoding, body))
	default:
		return io.ReadAll(body)
	}
}

func logUnrecognizedCharset(params map[string]string, logger *slog.Logger) {
	charset := strings.ToLower(params["charset"])
	if charset != "" && charset != "us-ascii" && charset != "utf-8" && logger != nil {
		logger.Debug("unrecognized charset, passing through raw bytes", "charset", charset)
	}
}
