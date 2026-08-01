// Package cfclient talks to the Cloudflare Email Sending REST API
// (POST /accounts/{account_id}/email/sending/send).
package cfclient

// SendEmailRequest is the subset of Cloudflare's request schema this relay
// supports: single recipient, plain-text and/or HTML body, file attachments,
// no cc/bcc/custom headers.
type SendEmailRequest struct {
	From        string
	To          string
	Subject     string
	Text        string
	HTML        string
	Attachments []Attachment
}

// Attachment is a single file to send as a regular (non-inline) attachment.
type Attachment struct {
	Filename    string
	ContentType string
	Content     []byte
}

// SendResult mirrors the "result" object of a successful Cloudflare response.
type SendResult struct {
	Delivered        []string
	Queued           []string
	PermanentBounces []string
}

// sendEmailPayload is the JSON wire shape of the request body.
type sendEmailPayload struct {
	From        string              `json:"from"`
	To          string              `json:"to"`
	Subject     string              `json:"subject"`
	Text        string              `json:"text,omitempty"`
	HTML        string              `json:"html,omitempty"`
	Attachments []attachmentPayload `json:"attachments,omitempty"`
}

// attachmentPayload is the JSON wire shape of one entry in "attachments".
// Cloudflare requires all four fields for the (non-inline) attachment variant.
type attachmentPayload struct {
	Content     string `json:"content"`
	Filename    string `json:"filename"`
	Type        string `json:"type"`
	Disposition string `json:"disposition"`
}

// apiResponse is the JSON wire shape of every Cloudflare API response.
type apiResponse struct {
	Success bool       `json:"success"`
	Errors  []apiError `json:"errors"`
	Result  *apiResult `json:"result"`
}

type apiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type apiResult struct {
	Delivered        []string `json:"delivered"`
	Queued           []string `json:"queued"`
	PermanentBounces []string `json:"permanent_bounces"`
}
