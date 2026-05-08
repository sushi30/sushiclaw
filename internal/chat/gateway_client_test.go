package chat

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGatewayClientRunnerRequiresConnectionDetails(t *testing.T) {
	_, err := NewGatewayClientRunner(GatewayClientOptions{Token: "token"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gateway host")

	_, err = NewGatewayClientRunner(GatewayClientOptions{Host: "127.0.0.1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token")
}

func TestGatewayWebSocketURL(t *testing.T) {
	tests := []struct {
		name string
		opts GatewayClientOptions
		want string
	}{
		{
			name: "host and port",
			opts: GatewayClientOptions{Host: "127.0.0.1", Port: 18801, SessionID: "cli"},
			want: "ws://127.0.0.1:18801/websocket/ws?session_id=cli",
		},
		{
			name: "http URL converted to websocket",
			opts: GatewayClientOptions{Host: "http://localhost:18801", SessionID: "cli"},
			want: "ws://localhost:18801/websocket/ws?session_id=cli",
		},
		{
			name: "preserves explicit path and session",
			opts: GatewayClientOptions{Host: "wss://example.test/custom?session_id=other", SessionID: "cli"},
			want: "wss://example.test/custom?session_id=other",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := gatewayWebSocketURL(tc.opts)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
