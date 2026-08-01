package cfclient

import "fmt"

// SendError is the only error type Client.SendEmail ever returns on failure —
// network failures, JSON decode failures, and Cloudflare API errors are all
// classified into one, so callers need exactly one errors.As check.
type SendError struct {
	Temporary     bool
	SMTPCode      int
	EnhancedCode  [3]int
	PublicMessage string

	// CFCode is 0 when the failure never reached a Cloudflare-coded response
	// (e.g. a network error or a non-JSON body).
	CFCode     int
	CFMessage  string
	HTTPStatus int
}

func (e *SendError) Error() string {
	if e.CFCode != 0 {
		return fmt.Sprintf("cfclient: cloudflare error %d (http %d): %s", e.CFCode, e.HTTPStatus, e.CFMessage)
	}
	return fmt.Sprintf("cfclient: request failed (http %d): %s", e.HTTPStatus, e.CFMessage)
}

// explicitCodes maps Cloudflare's documented error codes to an SMTP
// classification. Keep this in sync with the "Error mapping" table in
// README.md.
var explicitCodes = map[int]SendError{
	10004: {Temporary: true, SMTPCode: 450, EnhancedCode: [3]int{4, 7, 1}, PublicMessage: "rate limit exceeded, please retry"},

	10100: {Temporary: true, SMTPCode: 451, EnhancedCode: [3]int{4, 4, 2}, PublicMessage: "upstream auth service unavailable"},
	10002: {Temporary: true, SMTPCode: 451, EnhancedCode: [3]int{4, 4, 2}, PublicMessage: "internal error"},
	10003: {Temporary: true, SMTPCode: 451, EnhancedCode: [3]int{4, 4, 2}, PublicMessage: "not implemented"},

	10101: {Temporary: false, SMTPCode: 550, EnhancedCode: [3]int{5, 7, 1}, PublicMessage: "relay not authorized"},
	10102: {Temporary: false, SMTPCode: 550, EnhancedCode: [3]int{5, 7, 1}, PublicMessage: "relay not authorized"},
	10103: {Temporary: false, SMTPCode: 550, EnhancedCode: [3]int{5, 7, 1}, PublicMessage: "relay not authorized"},
	10203: {Temporary: false, SMTPCode: 550, EnhancedCode: [3]int{5, 7, 1}, PublicMessage: "sending disabled for this domain/account"},

	10200: {Temporary: false, SMTPCode: 552, EnhancedCode: [3]int{5, 3, 4}, PublicMessage: "message exceeds size limit"},

	10001: {Temporary: false, SMTPCode: 550, EnhancedCode: [3]int{5, 6, 0}, PublicMessage: "malformed request"},
	10201: {Temporary: false, SMTPCode: 550, EnhancedCode: [3]int{5, 6, 0}, PublicMessage: "malformed request"},
	10202: {Temporary: false, SMTPCode: 550, EnhancedCode: [3]int{5, 6, 0}, PublicMessage: "malformed content"},
}

// ClassifyError turns a Cloudflare error code (0 if none was returned, e.g. a
// network failure) plus the HTTP status into a SendError. Documented codes
// use the explicit table above; anything else (including undocumented codes
// like 10000/10105, and network-level failures with no HTTP status at all)
// falls back to a classification by HTTP status.
func ClassifyError(cfCode, httpStatus int) SendError {
	if se, ok := explicitCodes[cfCode]; ok {
		se.CFCode = cfCode
		se.HTTPStatus = httpStatus
		return se
	}

	se := classifyByHTTPStatus(httpStatus)
	se.CFCode = cfCode
	se.HTTPStatus = httpStatus
	return se
}

func classifyByHTTPStatus(httpStatus int) SendError {
	switch {
	case httpStatus == 429:
		return SendError{Temporary: true, SMTPCode: 450, EnhancedCode: [3]int{4, 7, 1}, PublicMessage: "rate limit exceeded, please retry"}
	case httpStatus >= 500:
		return SendError{Temporary: true, SMTPCode: 451, EnhancedCode: [3]int{4, 4, 2}, PublicMessage: "upstream error"}
	case httpStatus == 401 || httpStatus == 403:
		return SendError{Temporary: false, SMTPCode: 550, EnhancedCode: [3]int{5, 7, 1}, PublicMessage: "relay not authorized"}
	case httpStatus == 400 || httpStatus == 404:
		return SendError{Temporary: false, SMTPCode: 550, EnhancedCode: [3]int{5, 6, 0}, PublicMessage: "malformed request"}
	default:
		// Includes httpStatus == 0 (network error, no response received at all).
		return SendError{Temporary: true, SMTPCode: 451, EnhancedCode: [3]int{4, 4, 2}, PublicMessage: "temporary failure"}
	}
}
