package whatsapp

import (
	"path/filepath"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/channels"
	"github.com/sipeed/picoclaw/pkg/config"
)

func init() {
	channels.RegisterFactory("whatsapp", func(channelName, _ string, cfg *config.Config, b *bus.MessageBus) (channels.Channel, error) {
		bc := cfg.Channels[channelName]
		decoded, err := bc.GetDecoded()
		if err != nil {
			return nil, err
		}
		waCfg, ok := decoded.(*config.WhatsAppSettings)
		if !ok {
			return nil, channels.ErrSendFailed
		}
		storePath := waCfg.SessionStorePath
		if storePath == "" {
			storePath = filepath.Join(cfg.WorkspacePath(), "whatsapp")
		}
		return NewWhatsAppNativeChannel(bc, waCfg, cfg.Voice, b, storePath)
	})
}
