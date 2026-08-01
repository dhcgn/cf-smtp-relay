package cfclient

import "testing"

func TestClassifyError_ExplicitCodes(t *testing.T) {
	cases := []struct {
		name         string
		cfCode       int
		httpStatus   int
		wantTemp     bool
		wantSMTPCode int
	}{
		{"rate_limited", 10004, 429, true, 450},
		{"upstream_auth_unavailable", 10100, 503, true, 451},
		{"internal_error", 10002, 500, true, 451},
		{"not_implemented", 10003, 500, true, 451},
		{"bad_token_401", 10101, 401, false, 550},
		{"bad_token_403a", 10102, 403, false, 550},
		{"bad_token_403b", 10103, 403, false, 550},
		{"sending_disabled", 10203, 403, false, 550},
		{"too_big", 10200, 400, false, 552},
		{"malformed_request", 10001, 400, false, 550},
		{"malformed_content_a", 10201, 400, false, 550},
		{"malformed_content_b", 10202, 400, false, 550},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyError(tc.cfCode, tc.httpStatus)
			if got.Temporary != tc.wantTemp {
				t.Errorf("Temporary = %v, want %v", got.Temporary, tc.wantTemp)
			}
			if got.SMTPCode != tc.wantSMTPCode {
				t.Errorf("SMTPCode = %d, want %d", got.SMTPCode, tc.wantSMTPCode)
			}
			if got.CFCode != tc.cfCode {
				t.Errorf("CFCode = %d, want %d", got.CFCode, tc.cfCode)
			}
			if got.HTTPStatus != tc.httpStatus {
				t.Errorf("HTTPStatus = %d, want %d", got.HTTPStatus, tc.httpStatus)
			}
		})
	}
}

func TestClassifyError_FallbackByHTTPStatus(t *testing.T) {
	cases := []struct {
		name         string
		cfCode       int
		httpStatus   int
		wantTemp     bool
		wantSMTPCode int
	}{
		{"undocumented_not_found", 10000, 404, false, 550},
		{"undocumented_not_entitled", 10105, 403, false, 550},
		{"unknown_code_rate_limited", 99999, 429, true, 450},
		{"unknown_code_server_error", 99999, 502, true, 451},
		{"unknown_code_unauthorized", 99999, 401, false, 550},
		{"unknown_code_bad_request", 99999, 400, false, 550},
		{"network_error_no_status", 0, 0, true, 451},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyError(tc.cfCode, tc.httpStatus)
			if got.Temporary != tc.wantTemp {
				t.Errorf("Temporary = %v, want %v", got.Temporary, tc.wantTemp)
			}
			if got.SMTPCode != tc.wantSMTPCode {
				t.Errorf("SMTPCode = %d, want %d", got.SMTPCode, tc.wantSMTPCode)
			}
		})
	}
}

func TestSendError_Error(t *testing.T) {
	se := &SendError{CFCode: 10004, HTTPStatus: 429, CFMessage: "rate limited"}
	if got := se.Error(); got == "" {
		t.Error("Error() returned empty string")
	}

	se2 := &SendError{HTTPStatus: 0, CFMessage: "dial tcp: timeout"}
	if got := se2.Error(); got == "" {
		t.Error("Error() returned empty string for network error")
	}
}
