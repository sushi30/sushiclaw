package whatsapp

import (
	"path/filepath"

	"github.com/sushi30/sushiclaw/pkg/bus"
	"github.com/sushi30/sushiclaw/pkg/channels"
	"github.com/sushi30/sushiclaw/pkg/config"
)

func init() {
	channels.RegisterFactory(config.ChannelWhatsAppNative, func(channelName, _ string, cfg *config.Config, b *bus.MessageBus) (channels.Channel, error) {
		bc := cfg.Channels[channelName]
		if bc == nil {
			return nil, channels.ErrSendFailed
		}
		var waCfg config.WhatsAppSettings
		if err := bc.Decode(&waCfg); err != nil {
			return nil, err
		}
		storePath := waCfg.SessionStorePath
		if storePath == "" {
			storePath = filepath.Join(cfg.WorkspacePath(), "whatsapp")
		}
		return NewWhatsAppNativeChannel(bc, &waCfg, cfg.Voice(), b, storePath)
	})
}
