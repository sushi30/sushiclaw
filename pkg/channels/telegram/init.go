package telegram

import (
	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/channels"
	"github.com/sipeed/picoclaw/pkg/config"
)

func init() {
	channels.RegisterSafeFactory(config.ChannelTelegram,
		func(bc *config.Channel, tgCfg *config.TelegramSettings, b *bus.MessageBus) (channels.Channel, error) {
			return NewTelegramChannel(bc, tgCfg, b)
		})
}
