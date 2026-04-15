package gateway

import (
	"context"
	"sync"
	"time"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/logger"

	"github.com/sushi30/sushiclaw/internal/commandfilter"
)

type BusBridge struct {
	realBus  *bus.MessageBus
	agentBus *bus.MessageBus
	filter   *commandfilter.CommandFilter
	wg       sync.WaitGroup
}

const sendTimeout = 5 * time.Second

func NewBusBridge(realBus, agentBus *bus.MessageBus, filter *commandfilter.CommandFilter) *BusBridge {
	return &BusBridge{
		realBus:  realBus,
		agentBus: agentBus,
		filter:   filter,
	}
}

func (b *BusBridge) Start() {
	b.wg.Add(2)
	go b.filterInbound()
	go b.forwardOutbound()
}

func (b *BusBridge) Stop() {
	b.agentBus.Close()
	b.realBus.Close()
	b.wg.Wait()
}

func (b *BusBridge) filterInbound() {
	defer b.wg.Done()
	for msg := range b.realBus.InboundChan() {
		dec := b.filter.Filter(msg)
		if dec.Result == commandfilter.Block {
			logger.InfoCF("commandfilter", "Blocked unrecognized slash command",
				map[string]any{
					"channel": msg.Channel,
					"chat_id": msg.ChatID,
					"command": dec.Command,
				})
			pubCtx, cancel := context.WithTimeout(context.Background(), sendTimeout)
			err := b.realBus.PublishOutbound(pubCtx, bus.OutboundMessage{
				Channel: msg.Channel,
				ChatID:  msg.ChatID,
				Content: dec.ErrMsg,
			})
			cancel()
			if err != nil {
				logger.WarnCF("commandfilter", "Failed to send error reply for blocked command",
					map[string]any{"error": err.Error()})
			}
			continue
		}
		pubCtx, cancel := context.WithTimeout(context.Background(), sendTimeout)
		err := b.agentBus.PublishInbound(pubCtx, msg)
		cancel()
		if err != nil {
			logger.WarnCF("commandfilter", "Failed to forward inbound message",
				map[string]any{"error": err.Error()})
		}
	}
}

func (b *BusBridge) forwardOutbound() {
	defer b.wg.Done()
	var outboundWg sync.WaitGroup
	outboundWg.Add(2)

	go func() {
		defer outboundWg.Done()
		for msg := range b.agentBus.OutboundChan() {
			pubCtx, cancel := context.WithTimeout(context.Background(), sendTimeout)
			err := b.realBus.PublishOutbound(pubCtx, msg)
			cancel()
			if err != nil {
				logger.WarnCF("busbridge", "Failed to forward outbound message",
					map[string]any{"error": err.Error()})
			}
		}
	}()

	go func() {
		defer outboundWg.Done()
		for msg := range b.agentBus.OutboundMediaChan() {
			pubCtx, cancel := context.WithTimeout(context.Background(), sendTimeout)
			err := b.realBus.PublishOutboundMedia(pubCtx, msg)
			cancel()
			if err != nil {
				logger.WarnCF("busbridge", "Failed to forward outbound media message",
					map[string]any{"error": err.Error()})
			}
		}
	}()

	outboundWg.Wait()
}
