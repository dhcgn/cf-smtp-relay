package smtpserver

import (
	smtp "github.com/emersion/go-smtp"

	"github.com/dhcgn/cf-smtp-relay/internal/cfclient"
)

// toSMTPError adapts a *cfclient.SendError to a *smtp.SMTPError. Any other
// error type is a defensive fallback — cfclient.Client.SendEmail guarantees
// its non-nil errors are always *SendError, so this should not normally
// trigger.
func toSMTPError(err error) *smtp.SMTPError {
	if se, ok := err.(*cfclient.SendError); ok {
		return &smtp.SMTPError{
			Code:         se.SMTPCode,
			EnhancedCode: smtp.EnhancedCode(se.EnhancedCode),
			Message:      se.PublicMessage,
		}
	}
	return &smtp.SMTPError{
		Code:         451,
		EnhancedCode: smtp.EnhancedCode{4, 4, 2},
		Message:      "temporary failure forwarding message",
	}
}
