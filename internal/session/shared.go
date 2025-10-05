package session

import (
	"github.com/segmentio/ksuid"
)

// generateSessionID generates a new session ID with the "ses_" prefix
func generateSessionID() string {
	return "ses_" + ksuid.New().String()
}
