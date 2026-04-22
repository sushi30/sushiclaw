package channels

import (
	"fmt"
	"sync"

	"github.com/sushi30/sushiclaw/pkg/bus"
	"github.com/sushi30/sushiclaw/pkg/config"
)

// ChannelFactory is a constructor function that creates a Channel.
type ChannelFactory func(channelName, channelType string, cfg *config.Config, bus *bus.MessageBus) (Channel, error)

var (
	factoriesMu sync.RWMutex
	factories   = map[string]ChannelFactory{}
)

// RegisterFactory registers a named channel factory. Called from channel subpackage init() functions.
func RegisterFactory(name string, f ChannelFactory) {
	factoriesMu.Lock()
	defer factoriesMu.Unlock()
	factories[name] = f
}

// RegisterSafeFactory is a convenience wrapper that decodes channel settings and calls the constructor.
func RegisterSafeFactory[S any](
	channelType string,
	ctor func(bc *config.Channel, settings *S, bus *bus.MessageBus) (Channel, error),
) {
	RegisterFactory(channelType, func(channelName, _ string, cfg *config.Config, b *bus.MessageBus) (Channel, error) {
		bc := cfg.Channels[channelName]
		if bc == nil {
			return nil, fmt.Errorf("channel %q: config not found", channelName)
		}
		var settings S
		if err := bc.Decode(&settings); err != nil {
			return nil, fmt.Errorf("channel %q: failed to decode settings: %w", channelName, err)
		}
		return ctor(bc, &settings, b)
	})
}

// getFactory looks up a channel factory by name.
func getFactory(name string) (ChannelFactory, bool) {
	factoriesMu.RLock()
	defer factoriesMu.RUnlock()
	f, ok := factories[name]
	return f, ok
}

// GetRegisteredFactoryNames returns all registered channel factory names.
func GetRegisteredFactoryNames() []string {
	factoriesMu.RLock()
	defer factoriesMu.RUnlock()
	names := make([]string, 0, len(factories))
	for name := range factories {
		names = append(names, name)
	}
	return names
}
