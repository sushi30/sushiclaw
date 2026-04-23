package gateway_test

import (
	"testing"

	sushitools "github.com/sushi30/sushiclaw/pkg/tools"
)

// TestGateway_TrustedExecParsing verifies that ParseAllowedSenders correctly
// reads the environment variable.
func TestGateway_TrustedExecParsing(t *testing.T) {
	t.Setenv("SUSHICLAW_EXEC_ALLOWED_SENDERS", "+1234567890")

	allowedSenders := sushitools.ParseAllowedSenders()
	if len(allowedSenders) == 0 {
		t.Fatal("expected ParseAllowedSenders to return entries")
	}
	if allowedSenders[0] != "+1234567890" {
		t.Errorf("got %q, want +1234567890", allowedSenders[0])
	}
}

// TestGateway_NoSendersWhenEnvUnset verifies that ParseAllowedSenders returns
// nil when the env var is absent.
func TestGateway_NoSendersWhenEnvUnset(t *testing.T) {
	t.Setenv("SUSHICLAW_EXEC_ALLOWED_SENDERS", "")

	allowedSenders := sushitools.ParseAllowedSenders()
	if len(allowedSenders) > 0 {
		t.Fatal("expected ParseAllowedSenders to return nil when env var is empty")
	}
}
