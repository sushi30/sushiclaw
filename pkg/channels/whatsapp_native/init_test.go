package whatsapp

import (
	"testing"

	"github.com/sushi30/sushiclaw/pkg/channels"
	"github.com/sushi30/sushiclaw/pkg/config"
)

// TestFactoryRegisteredAsWhatsAppNative guards against re-registering the factory
// under the legacy "whatsapp" name, which fails picoclaw's readiness check that
// requires type == "whatsapp_native" for bridge-less native operation.
func TestFactoryRegisteredAsWhatsAppNative(t *testing.T) {
	names := channels.GetRegisteredFactoryNames()
	for _, n := range names {
		if n == config.ChannelWhatsAppNative {
			return
		}
	}
	t.Errorf("whatsapp_native factory not registered; got %v", names)
}
