package cfclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSendEmail_Success(t *testing.T) {
	var gotAuth, gotContentType, gotPath, gotMethod string
	var gotBody sendEmailPayload

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		gotPath = r.URL.Path
		gotMethod = r.Method
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(apiResponse{
			Success: true,
			Result: &apiResult{
				Delivered: []string{"to@example.com"},
			},
		})
	}))
	defer srv.Close()

	c := New(Config{APIToken: "test-token", AccountID: "acct123", BaseURL: srv.URL})
	result, err := c.SendEmail(context.Background(), SendEmailRequest{
		From:    "from@example.com",
		To:      "to@example.com",
		Subject: "hello",
		Text:    "plain body",
	})
	if err != nil {
		t.Fatalf("SendEmail() error = %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}
	if gotAuth != "Bearer test-token" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer test-token")
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotContentType)
	}
	if !strings.Contains(gotPath, "/accounts/acct123/email/sending/send") {
		t.Errorf("path = %q, want to contain /accounts/acct123/email/sending/send", gotPath)
	}
	if gotBody.From != "from@example.com" || gotBody.To != "to@example.com" || gotBody.Subject != "hello" {
		t.Errorf("request body = %+v, unexpected", gotBody)
	}
	if len(result.Delivered) != 1 || result.Delivered[0] != "to@example.com" {
		t.Errorf("result.Delivered = %v, want [to@example.com]", result.Delivered)
	}
}

func TestSendEmail_APIErrors(t *testing.T) {
	cases := []struct {
		name       string
		httpStatus int
		cfCode     int
	}{
		{"rate_limited", http.StatusTooManyRequests, 10004},
		{"server_error", http.StatusInternalServerError, 10002},
		{"unauthorized", http.StatusUnauthorized, 10101},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.httpStatus)
				_ = json.NewEncoder(w).Encode(apiResponse{
					Success: false,
					Errors:  []apiError{{Code: tc.cfCode, Message: "boom"}},
				})
			}))
			defer srv.Close()

			c := New(Config{APIToken: "t", AccountID: "a", BaseURL: srv.URL})
			_, err := c.SendEmail(context.Background(), SendEmailRequest{From: "a@b.com", To: "c@d.com", Subject: "s", Text: "t"})
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			var sendErr *SendError
			if !asSendError(err, &sendErr) {
				t.Fatalf("error is not *SendError: %v (%T)", err, err)
			}
			if sendErr.CFCode != tc.cfCode {
				t.Errorf("CFCode = %d, want %d", sendErr.CFCode, tc.cfCode)
			}
			if sendErr.HTTPStatus != tc.httpStatus {
				t.Errorf("HTTPStatus = %d, want %d", sendErr.HTTPStatus, tc.httpStatus)
			}
		})
	}
}

func TestSendEmail_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	c := New(Config{APIToken: "t", AccountID: "a", BaseURL: srv.URL})
	_, err := c.SendEmail(context.Background(), SendEmailRequest{From: "a@b.com", To: "c@d.com", Subject: "s", Text: "t"})
	if err == nil {
		t.Fatal("expected error for malformed JSON body, got nil")
	}
	var sendErr *SendError
	if !asSendError(err, &sendErr) {
		t.Fatalf("error is not *SendError: %v (%T)", err, err)
	}
	if !sendErr.Temporary {
		t.Errorf("expected malformed-body error to be classified temporary, got permanent")
	}
}

func TestSendEmail_ContextTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	c := New(Config{APIToken: "t", AccountID: "a", BaseURL: srv.URL})
	_, err := c.SendEmail(ctx, SendEmailRequest{From: "a@b.com", To: "c@d.com", Subject: "s", Text: "t"})
	if err == nil {
		t.Fatal("expected context deadline error, got nil")
	}
	var sendErr *SendError
	if !asSendError(err, &sendErr) {
		t.Fatalf("error is not *SendError: %v (%T)", err, err)
	}
	if sendErr.HTTPStatus != 0 {
		t.Errorf("HTTPStatus = %d, want 0 for a network-level failure", sendErr.HTTPStatus)
	}
}

func TestNew_DefaultBaseURL(t *testing.T) {
	c := New(Config{APIToken: "t", AccountID: "a"})
	if c.cfg.BaseURL != defaultBaseURL {
		t.Errorf("BaseURL = %q, want default %q", c.cfg.BaseURL, defaultBaseURL)
	}
}

// asSendError is a small helper to avoid importing errors.As in every test.
func asSendError(err error, target **SendError) bool {
	se, ok := err.(*SendError)
	if !ok {
		return false
	}
	*target = se
	return true
}
