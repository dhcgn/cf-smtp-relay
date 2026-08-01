package smtpserver

import (
	"errors"
	"strings"
	"testing"
)

func TestParseMessage(t *testing.T) {
	cases := []struct {
		name        string
		raw         string
		wantSubject string
		wantText    string
		wantHTML    string
		wantErr     error
	}{
		{
			name: "plain_only",
			raw: "From: a@example.com\r\n" +
				"To: b@example.com\r\n" +
				"Subject: hello\r\n" +
				"Content-Type: text/plain\r\n" +
				"\r\n" +
				"plain body\r\n",
			wantSubject: "hello",
			wantText:    "plain body\r\n",
		},
		{
			name: "no_content_type_defaults_to_plain",
			raw: "From: a@example.com\r\n" +
				"To: b@example.com\r\n" +
				"Subject: hello\r\n" +
				"\r\n" +
				"bare body\r\n",
			wantSubject: "hello",
			wantText:    "bare body\r\n",
		},
		{
			name: "html_only",
			raw: "From: a@example.com\r\n" +
				"To: b@example.com\r\n" +
				"Subject: hello\r\n" +
				"Content-Type: text/html\r\n" +
				"\r\n" +
				"<p>hi</p>\r\n",
			wantSubject: "hello",
			wantHTML:    "<p>hi</p>\r\n",
		},
		{
			name: "multipart_alternative_both",
			raw: "From: a@example.com\r\n" +
				"To: b@example.com\r\n" +
				"Subject: hello\r\n" +
				"Content-Type: multipart/alternative; boundary=BOUND\r\n" +
				"\r\n" +
				"--BOUND\r\n" +
				"Content-Type: text/plain\r\n" +
				"\r\n" +
				"plain part\r\n" +
				"--BOUND\r\n" +
				"Content-Type: text/html\r\n" +
				"\r\n" +
				"<p>html part</p>\r\n" +
				"--BOUND--\r\n",
			wantSubject: "hello",
			// mime/multipart strips the CRLF immediately preceding a boundary
			// line (it's delimiter syntax per RFC 2046, not body content).
			wantText: "plain part",
			wantHTML: "<p>html part</p>",
		},
		{
			name: "rfc2047_encoded_subject",
			raw: "From: a@example.com\r\n" +
				"To: b@example.com\r\n" +
				"Subject: =?UTF-8?B?SGVsbG8gV29ybGQ=?=\r\n" +
				"Content-Type: text/plain\r\n" +
				"\r\n" +
				"body\r\n",
			wantSubject: "Hello World",
			wantText:    "body\r\n",
		},
		{
			name: "quoted_printable_body",
			raw: "From: a@example.com\r\n" +
				"To: b@example.com\r\n" +
				"Subject: qp\r\n" +
				"Content-Type: text/plain\r\n" +
				"Content-Transfer-Encoding: quoted-printable\r\n" +
				"\r\n" +
				"caf=C3=A9\r\n",
			wantSubject: "qp",
			wantText:    "café\r\n",
		},
		{
			name: "base64_body",
			raw: "From: a@example.com\r\n" +
				"To: b@example.com\r\n" +
				"Subject: b64\r\n" +
				"Content-Type: text/plain\r\n" +
				"Content-Transfer-Encoding: base64\r\n" +
				"\r\n" +
				"aGVsbG8gd29ybGQ=\r\n",
			wantSubject: "b64",
			wantText:    "hello world",
		},
		{
			name: "multipart_mixed_with_attachment_keeps_text",
			raw: "From: a@example.com\r\n" +
				"To: b@example.com\r\n" +
				"Subject: mixed\r\n" +
				"Content-Type: multipart/mixed; boundary=OUTER\r\n" +
				"\r\n" +
				"--OUTER\r\n" +
				"Content-Type: multipart/alternative; boundary=INNER\r\n" +
				"\r\n" +
				"--INNER\r\n" +
				"Content-Type: text/plain\r\n" +
				"\r\n" +
				"the text\r\n" +
				"--INNER\r\n" +
				"Content-Type: text/html\r\n" +
				"\r\n" +
				"<p>the html</p>\r\n" +
				"--INNER--\r\n" +
				"--OUTER\r\n" +
				"Content-Type: application/pdf\r\n" +
				"Content-Transfer-Encoding: base64\r\n" +
				"\r\n" +
				"aGVsbG8=\r\n" +
				"--OUTER--\r\n",
			wantSubject: "mixed",
			wantText:    "the text",
			wantHTML:    "<p>the html</p>",
		},
		{
			name: "attachment_only_rejected",
			raw: "From: a@example.com\r\n" +
				"To: b@example.com\r\n" +
				"Subject: attachment only\r\n" +
				"Content-Type: application/pdf\r\n" +
				"Content-Transfer-Encoding: base64\r\n" +
				"\r\n" +
				"aGVsbG8=\r\n",
			wantErr: ErrUnsupportedContent,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseMessage([]byte(tc.raw), nil)

			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected error %v, got %v (result: %+v)", tc.wantErr, err, got)
				}
				return
			}

			if err != nil {
				t.Fatalf("ParseMessage() unexpected error: %v", err)
			}
			if got.Subject != tc.wantSubject {
				t.Errorf("Subject = %q, want %q", got.Subject, tc.wantSubject)
			}
			if got.Text != tc.wantText {
				t.Errorf("Text = %q, want %q", got.Text, tc.wantText)
			}
			if got.HTML != tc.wantHTML {
				t.Errorf("HTML = %q, want %q", got.HTML, tc.wantHTML)
			}
		})
	}
}

func TestParseMessage_MalformedMessageAlwaysErrors(t *testing.T) {
	_, err := ParseMessage([]byte("this is not an RFC 5322 message"), nil)
	if err == nil {
		t.Fatal("expected an error for a malformed message, got nil")
	}
	if !strings.Contains(err.Error(), "smtpserver:") {
		t.Errorf("error should be wrapped with smtpserver context: %v", err)
	}
}
