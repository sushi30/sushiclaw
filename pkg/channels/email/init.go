package email

import (
	"github.com/sushi30/sushiclaw/pkg/bus"
	"github.com/sushi30/sushiclaw/pkg/channels"
	"github.com/sushi30/sushiclaw/pkg/config"
)

func init() {
	channels.RegisterSafeFactory(config.ChannelEmail,
		func(bc *config.Channel, emailCfg *config.EmailSettings, b *bus.MessageBus) (channels.Channel, error) {
			return NewEmailChannel(bc, emailCfg, b)
		})
}
