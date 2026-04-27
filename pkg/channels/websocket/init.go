package websocket

import (
	"github.com/sushi30/sushiclaw/pkg/bus"
	"github.com/sushi30/sushiclaw/pkg/channels"
	"github.com/sushi30/sushiclaw/pkg/config"
)

func init() {
	channels.RegisterSafeFactory(config.ChannelWebSocket,
		func(bc *config.Channel, cfg *config.WebSocketSettings, b *bus.MessageBus) (channels.Channel, error) {
			return NewWebSocketChannel(bc, cfg, b)
		})
	channels.RegisterSafeFactory(config.ChannelWebSocketClient,
		func(bc *config.Channel, cfg *config.WebSocketClientSettings, b *bus.MessageBus) (channels.Channel, error) {
			return NewWebSocketClientChannel(bc, cfg, b)
		})
}
