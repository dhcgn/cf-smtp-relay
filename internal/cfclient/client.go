package cfclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Config holds the connection details for a Client.
type Config struct {
	APIToken  string
	AccountID string
	// BaseURL defaults to https://api.cloudflare.com/client/v4 when empty.
	BaseURL string
}

const defaultBaseURL = "https://api.cloudflare.com/client/v4"

// Client sends emails via the Cloudflare Email Sending REST API.
type Client struct {
	httpClient *http.Client
	cfg        Config
}

// New returns a Client using cfg. If cfg.BaseURL is empty, the default
// Cloudflare API base URL is used.
func New(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		cfg:        cfg,
	}
}

// SendEmail forwards req to Cloudflare. Any non-nil error is a *SendError.
func (c *Client) SendEmail(ctx context.Context, req SendEmailRequest) (*SendResult, error) {
	body, err := json.Marshal(sendEmailPayload{
		From:    req.From,
		To:      req.To,
		Subject: req.Subject,
		Text:    req.Text,
		HTML:    req.HTML,
	})
	if err != nil {
		return nil, wrapNetworkError(fmt.Errorf("cfclient: marshal request: %w", err))
	}

	url := fmt.Sprintf("%s/accounts/%s/email/sending/send", c.cfg.BaseURL, c.cfg.AccountID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, wrapNetworkError(fmt.Errorf("cfclient: build request: %w", err))
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIToken)
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, wrapNetworkError(fmt.Errorf("cfclient: send request: %w", err))
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, wrapNetworkErrorWithStatus(httpResp.StatusCode, fmt.Errorf("cfclient: read response: %w", err))
	}

	var apiResp apiResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, wrapNetworkErrorWithStatus(httpResp.StatusCode, fmt.Errorf("cfclient: decode response: %w", err))
	}

	if !apiResp.Success {
		cfCode := 0
		cfMessage := "unknown error"
		if len(apiResp.Errors) > 0 {
			cfCode = apiResp.Errors[0].Code
			cfMessage = apiResp.Errors[0].Message
		}
		se := ClassifyError(cfCode, httpResp.StatusCode)
		se.CFMessage = cfMessage
		return nil, &se
	}

	result := &SendResult{}
	if apiResp.Result != nil {
		result.Delivered = apiResp.Result.Delivered
		result.Queued = apiResp.Result.Queued
		result.PermanentBounces = apiResp.Result.PermanentBounces
	}
	return result, nil
}

// wrapNetworkError classifies a failure that never produced an HTTP response
// (DNS/dial/timeout/marshal errors) as a SendError with HTTPStatus 0.
func wrapNetworkError(err error) *SendError {
	return wrapNetworkErrorWithStatus(0, err)
}

func wrapNetworkErrorWithStatus(httpStatus int, err error) *SendError {
	se := ClassifyError(0, httpStatus)
	se.CFMessage = err.Error()
	return &se
}
