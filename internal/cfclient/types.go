// Package cfclient talks to the Cloudflare Email Sending REST API
// (POST /accounts/{account_id}/email/sending/send).
package cfclient

// SendEmailRequest is the M1 subset of Cloudflare's request schema:
// single recipient, plain-text and/or HTML body, no cc/bcc/attachments/headers.
type SendEmailRequest struct {
	From    string
	To      string
	Subject string
	Text    string
	HTML    string
}

// SendResult mirrors the "result" object of a successful Cloudflare response.
type SendResult struct {
	Delivered        []string
	Queued           []string
	PermanentBounces []string
}

// sendEmailPayload is the JSON wire shape of the request body.
type sendEmailPayload struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Subject string `json:"subject"`
	Text    string `json:"text,omitempty"`
	HTML    string `json:"html,omitempty"`
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
