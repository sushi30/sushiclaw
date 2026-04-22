package telegram

import (
	"github.com/sushi30/sushiclaw/pkg/bus"
	"github.com/sushi30/sushiclaw/pkg/channels"
	"github.com/sushi30/sushiclaw/pkg/config"
)

func init() {
	channels.RegisterSafeFactory(config.ChannelTelegram,
		func(bc *config.Channel, tgCfg *config.TelegramSettings, b *bus.MessageBus) (channels.Channel, error) {
			return NewTelegramChannel(bc, tgCfg, b)
		})
}
