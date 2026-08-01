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
// text/html content at all (e.g. an attachment-only message). Cloudflare's
// Email Sending API requires at least one of text/html on every send, so
// this rejection applies even when the message has attachments.
var ErrUnsupportedContent = errors.New("smtpserver: message has no text or HTML body")

// ParsedMessage is the subset of an RFC 5322 message forwarded to
// Cloudflare: Subject, plain-text and/or HTML bodies, and file attachments.
type ParsedMessage struct {
	Subject     string
	Text        string
	HTML        string
	Attachments []Attachment
}

// Attachment is a file extracted from a MIME part that isn't part of the
// text/plain or text/html body: one explicitly marked as an attachment,
// carrying a filename, or simply of a non-text content type.
type Attachment struct {
	Filename    string
	ContentType string
	Content     []byte
}

// headerGetter is satisfied by both mail.Header and textproto.MIMEHeader, so
// the same extraction code handles the top-level message and every nested
// multipart.Part.
type headerGetter interface {
	Get(string) string
}

// ParseMessage parses a raw RFC 5322 message and extracts the Subject,
// text/plain and/or text/html bodies, and any file attachments, recursing
// into multipart content. logger may be nil; when non-nil, deeply-nested
// dropped parts and unrecognized charsets are logged.
func ParseMessage(raw []byte, logger *slog.Logger) (*ParsedMessage, error) {
	m, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("smtpserver: parse message: %w", err)
	}

	text, html, attachments, err := extractContent(m.Header, m.Body, 0, logger)
	if err != nil {
		return nil, fmt.Errorf("smtpserver: extract content: %w", err)
	}
	if text == "" && html == "" {
		return nil, ErrUnsupportedContent
	}

	return &ParsedMessage{
		Subject:     decodeSubject(m.Header.Get("Subject")),
		Text:        text,
		HTML:        html,
		Attachments: attachments,
	}, nil
}

func decodeSubject(raw string) string {
	return decodeMIMEWord(raw)
}

// decodeMIMEWord decodes RFC 2047 encoded-words (e.g. in a Subject or a
// legacy attachment filename), falling back to the raw string on error.
func decodeMIMEWord(raw string) string {
	if raw == "" {
		return ""
	}
	decoded, err := (&mime.WordDecoder{}).DecodeHeader(raw)
	if err != nil {
		return raw
	}
	return decoded
}

func extractContent(header headerGetter, body io.Reader, depth int, logger *slog.Logger) (text, html string, attachments []Attachment, err error) {
	if depth > maxMIMEDepth {
		if logger != nil {
			logger.Warn("dropping deeply nested MIME part", "depth", depth)
		}
		return "", "", nil, nil
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
		return "", "", nil, fmt.Errorf("decode content-transfer-encoding: %w", err)
	}

	dispType, dispParams := parseContentDisposition(header.Get("Content-Disposition"))
	filename := decodeMIMEWord(firstNonEmpty(dispParams["filename"], params["filename"], params["name"]))

	// Any part explicitly marked as an attachment, carrying a filename, or
	// simply not text/plain / text/html is treated as a file attachment
	// rather than dropped.
	if dispType == "attachment" || filename != "" || (mediaType != "text/plain" && mediaType != "text/html") {
		if filename == "" {
			filename = fallbackFilename(mediaType)
		}
		return "", "", []Attachment{{Filename: filename, ContentType: mediaType, Content: decoded}}, nil
	}

	switch mediaType {
	case "text/plain":
		logUnrecognizedCharset(params, logger)
		return string(decoded), "", nil, nil
	case "text/html":
		logUnrecognizedCharset(params, logger)
		return "", string(decoded), nil, nil
	default:
		// Unreachable in practice: the attachment check above already
		// claims every non-text/plain, non-text/html part.
		if logger != nil {
			logger.Warn("dropping unsupported message part", "content_type", mediaType)
		}
		return "", "", nil, nil
	}
}

func extractMultipart(mediaType string, params map[string]string, body io.Reader, depth int, logger *slog.Logger) (text, html string, attachments []Attachment, err error) {
	boundary := params["boundary"]
	if boundary == "" {
		return "", "", nil, fmt.Errorf("%s content missing boundary", mediaType)
	}

	mr := multipart.NewReader(body, boundary)
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return text, html, attachments, fmt.Errorf("read multipart part: %w", err)
		}

		partText, partHTML, partAttachments, err := extractContent(part.Header, part, depth+1, logger)
		if err != nil {
			return text, html, attachments, err
		}
		attachments = append(attachments, partAttachments...)
		if partText != "" && text == "" {
			text = partText
		}
		if partHTML != "" && html == "" {
			html = partHTML
		}
	}
	return text, html, attachments, nil
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

// parseContentDisposition returns the lowercased disposition type (e.g.
// "attachment", "inline") and its parameters, or ("", nil) when the header
// is absent or unparseable.
func parseContentDisposition(raw string) (string, map[string]string) {
	if raw == "" {
		return "", nil
	}
	dispType, params, err := mime.ParseMediaType(raw)
	if err != nil {
		return "", nil
	}
	return strings.ToLower(dispType), params
}

// firstNonEmpty returns the first non-empty string in values, or "".
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// fallbackFilename generates a filename for an attachment part that didn't
// carry one, guessing an extension from its media type when possible.
func fallbackFilename(mediaType string) string {
	if exts, err := mime.ExtensionsByType(mediaType); err == nil && len(exts) > 0 {
		return "attachment" + exts[0]
	}
	return "attachment"
}
