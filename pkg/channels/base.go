package channels

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/sushi30/sushiclaw/pkg/bus"
	"github.com/sushi30/sushiclaw/pkg/config"
	"github.com/sushi30/sushiclaw/pkg/identity"
	"github.com/sushi30/sushiclaw/pkg/logger"
	"github.com/sushi30/sushiclaw/pkg/media"
)

var (
	uniqueIDCounter uint64
	uniqueIDPrefix  string
)

func init() {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		binary.BigEndian.PutUint64(b[:], uint64(time.Now().UnixNano()))
	}
	uniqueIDPrefix = hex.EncodeToString(b[:])
}

var audioAnnotationRe = regexp.MustCompile(`\[(voice|audio)(?::[^\]]*)?\]`)

func uniqueID() string {
	n := atomic.AddUint64(&uniqueIDCounter, 1)
	return uniqueIDPrefix + strconv.FormatUint(n, 16)
}

// Channel is the interface all channel implementations must satisfy.
type Channel interface {
	Name() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Send(ctx context.Context, msg bus.OutboundMessage) ([]string, error)
	IsRunning() bool
	IsAllowed(senderID string) bool
	IsAllowedSender(sender bus.SenderInfo) bool
	ReasoningChannelID() string
}

// BaseChannelOption is a functional option for configuring a BaseChannel.
type BaseChannelOption func(*BaseChannel)

// WithMaxMessageLength sets the maximum message length (in runes).
func WithMaxMessageLength(n int) BaseChannelOption {
	return func(c *BaseChannel) { c.maxMessageLength = n }
}

// WithGroupTrigger sets the group trigger configuration.
func WithGroupTrigger(gt config.GroupTriggerConfig) BaseChannelOption {
	return func(c *BaseChannel) { c.groupTrigger = gt }
}

// WithReasoningChannelID sets the reasoning channel ID.
func WithReasoningChannelID(id string) BaseChannelOption {
	return func(c *BaseChannel) { c.reasoningChannelID = id }
}

// MessageLengthProvider is an opt-in interface for channels advertising their max message length.
type MessageLengthProvider interface {
	MaxMessageLength() int
}

// BaseChannel provides shared functionality for channel implementations.
type BaseChannel struct {
	config              any
	bus                 *bus.MessageBus
	running             atomic.Bool
	name                string
	allowList           []string
	maxMessageLength    int
	groupTrigger        config.GroupTriggerConfig
	mediaStore          media.MediaStore
	placeholderRecorder PlaceholderRecorder
	owner               Channel
	reasoningChannelID  string
}

// NewBaseChannel creates a new BaseChannel.
func NewBaseChannel(
	name string,
	cfg any,
	messageBus *bus.MessageBus,
	allowList []string,
	opts ...BaseChannelOption,
) *BaseChannel {
	isEmpty := true
	for _, s := range allowList {
		if s != "" {
			isEmpty = false
			break
		}
	}
	if isEmpty {
		allowList = []string{}
	}
	bc := &BaseChannel{
		config:    cfg,
		bus:       messageBus,
		name:      name,
		allowList: allowList,
	}
	for _, opt := range opts {
		opt(bc)
	}
	if len(bc.allowList) == 0 {
		logger.WarnCF("channels", "SECURITY: Channel allows EVERYONE (allow_from is empty)", map[string]any{
			"channel": bc.name,
			"hint":    "Set allow_from to your ID, or use '*' to explicitly acknowledge open access.",
		})
	}
	return bc
}

// MaxMessageLength returns the max message length (0 = no limit).
func (c *BaseChannel) MaxMessageLength() int { return c.maxMessageLength }

// Name returns the channel name.
func (c *BaseChannel) Name() string { return c.name }

// SetName updates the channel name.
func (c *BaseChannel) SetName(name string) { c.name = name }

// ReasoningChannelID returns the reasoning channel ID.
func (c *BaseChannel) ReasoningChannelID() string { return c.reasoningChannelID }

// IsRunning returns whether the channel is currently running.
func (c *BaseChannel) IsRunning() bool { return c.running.Load() }

// SetRunning updates the running state.
func (c *BaseChannel) SetRunning(running bool) { c.running.Store(running) }

// IsAllowed checks whether a senderID is permitted by the allow-list.
func (c *BaseChannel) IsAllowed(senderID string) bool {
	if len(c.allowList) == 0 {
		return true
	}
	idPart := senderID
	userPart := ""
	if idx := strings.Index(senderID, "|"); idx > 0 {
		idPart = senderID[:idx]
		userPart = senderID[idx+1:]
	}
	for _, allowed := range c.allowList {
		if allowed == "*" {
			return true
		}
		trimmed := strings.TrimPrefix(allowed, "@")
		allowedID := trimmed
		allowedUser := ""
		if idx := strings.Index(trimmed, "|"); idx > 0 {
			allowedID = trimmed[:idx]
			allowedUser = trimmed[idx+1:]
		}
		if senderID == allowed ||
			idPart == allowed ||
			senderID == trimmed ||
			idPart == trimmed ||
			idPart == allowedID ||
			(allowedUser != "" && senderID == allowedUser) ||
			(userPart != "" && (userPart == allowed || userPart == trimmed || userPart == allowedUser)) {
			return true
		}
	}
	return false
}

// IsAllowedSender checks whether a SenderInfo is permitted by the allow-list.
func (c *BaseChannel) IsAllowedSender(sender bus.SenderInfo) bool {
	if len(c.allowList) == 0 {
		return true
	}
	for _, allowed := range c.allowList {
		if allowed == "*" || identity.MatchAllowed(sender, allowed) {
			return true
		}
	}
	return false
}

// ShouldRespondInGroup determines whether the bot should respond in a group chat.
func (c *BaseChannel) ShouldRespondInGroup(isMentioned bool, content string) (bool, string) {
	gt := c.groupTrigger
	if isMentioned {
		return true, strings.TrimSpace(content)
	}
	if gt.MentionOnly {
		return false, content
	}
	if len(gt.Prefixes) > 0 {
		for _, prefix := range gt.Prefixes {
			if prefix != "" && strings.HasPrefix(content, prefix) {
				return true, strings.TrimSpace(strings.TrimPrefix(content, prefix))
			}
		}
		return false, content
	}
	return true, strings.TrimSpace(content)
}

// HandleMessageWithContext publishes an inbound message to the bus,
// auto-triggering typing/reaction/placeholder if the channel supports them.
func (c *BaseChannel) HandleMessageWithContext(
	ctx context.Context,
	deliveryChatID, content string,
	mediaFiles []string,
	inboundCtx bus.InboundContext,
	senderOpts ...bus.SenderInfo,
) {
	c.HandleMessageWithContextAndSession(ctx, deliveryChatID, content, mediaFiles, inboundCtx, "", senderOpts...)
}

// HandleMessageWithContextAndSession is like HandleMessageWithContext but allows
// the caller to specify an explicit session key for per-conversation isolation.
func (c *BaseChannel) HandleMessageWithContextAndSession(
	ctx context.Context,
	deliveryChatID, content string,
	mediaFiles []string,
	inboundCtx bus.InboundContext,
	sessionKey string,
	senderOpts ...bus.SenderInfo,
) {
	var sender bus.SenderInfo
	if len(senderOpts) > 0 {
		sender = senderOpts[0]
	}
	senderID := strings.TrimSpace(inboundCtx.SenderID)
	if sender.CanonicalID != "" || sender.PlatformID != "" {
		if !c.IsAllowedSender(sender) {
			return
		}
	} else {
		if !c.IsAllowed(senderID) {
			return
		}
	}

	resolvedSenderID := senderID
	if sender.CanonicalID != "" {
		resolvedSenderID = sender.CanonicalID
	}
	if resolvedSenderID == "" {
		resolvedSenderID = senderID
	}

	inboundCtx.Channel = c.name
	if inboundCtx.ChatID == "" {
		inboundCtx.ChatID = deliveryChatID
	}
	if inboundCtx.SenderID == "" {
		inboundCtx.SenderID = resolvedSenderID
	}

	scope := BuildMediaScope(c.name, deliveryChatID, inboundCtx.MessageID)

	msg := bus.InboundMessage{
		Context:    inboundCtx,
		Sender:     sender,
		Content:    content,
		Media:      mediaFiles,
		MediaScope: scope,
		SessionKey: sessionKey,
	}
	msg = bus.NormalizeInboundMessage(msg)

	if c.owner != nil && c.placeholderRecorder != nil {
		if tc, ok := c.owner.(TypingCapable); ok {
			if stop, err := tc.StartTyping(ctx, deliveryChatID); err == nil {
				c.placeholderRecorder.RecordTypingStop(c.name, deliveryChatID, stop)
			}
		}
		if rc, ok := c.owner.(ReactionCapable); ok && msg.MessageID != "" {
			if undo, err := rc.ReactToMessage(ctx, deliveryChatID, msg.MessageID); err == nil {
				c.placeholderRecorder.RecordReactionUndo(c.name, deliveryChatID, undo)
			}
		}
		if !audioAnnotationRe.MatchString(content) {
			if pc, ok := c.owner.(PlaceholderCapable); ok {
				if phID, err := pc.SendPlaceholder(ctx, deliveryChatID); err == nil && phID != "" {
					c.placeholderRecorder.RecordPlaceholder(c.name, deliveryChatID, phID)
				}
			}
		}
	}

	if err := c.bus.PublishInbound(ctx, msg); err != nil {
		logger.ErrorCF("channels", "Failed to publish inbound message", map[string]any{
			"channel": c.name,
			"chat_id": deliveryChatID,
			"error":   err.Error(),
		})
	}
}

// HandleInboundContext publishes a normalized inbound message using the structured context.
func (c *BaseChannel) HandleInboundContext(
	ctx context.Context,
	deliveryChatID, content string,
	mediaFiles []string,
	inboundCtx bus.InboundContext,
	senderOpts ...bus.SenderInfo,
) {
	c.HandleMessageWithContext(ctx, deliveryChatID, content, mediaFiles, inboundCtx, senderOpts...)
}

// HandleInboundContextAndSession is like HandleInboundContext but allows the
// caller to specify an explicit session key for per-conversation isolation.
func (c *BaseChannel) HandleInboundContextAndSession(
	ctx context.Context,
	deliveryChatID, content string,
	mediaFiles []string,
	inboundCtx bus.InboundContext,
	sessionKey string,
	senderOpts ...bus.SenderInfo,
) {
	c.HandleMessageWithContextAndSession(ctx, deliveryChatID, content, mediaFiles, inboundCtx, sessionKey, senderOpts...)
}

// SetMediaStore injects a MediaStore into the channel.
func (c *BaseChannel) SetMediaStore(s media.MediaStore) { c.mediaStore = s }

// GetMediaStore returns the injected MediaStore (may be nil).
func (c *BaseChannel) GetMediaStore() media.MediaStore { return c.mediaStore }

// SetPlaceholderRecorder injects a PlaceholderRecorder into the channel.
func (c *BaseChannel) SetPlaceholderRecorder(r PlaceholderRecorder) {
	c.placeholderRecorder = r
}

// GetPlaceholderRecorder returns the injected PlaceholderRecorder (may be nil).
func (c *BaseChannel) GetPlaceholderRecorder() PlaceholderRecorder {
	return c.placeholderRecorder
}

// SetOwner injects the concrete channel that embeds this BaseChannel.
func (c *BaseChannel) SetOwner(ch Channel) {
	c.owner = ch
}

// BuildMediaScope constructs a scope key for media lifecycle tracking.
func BuildMediaScope(channel, chatID, messageID string) string {
	id := messageID
	if id == "" {
		id = uniqueID()
	}
	return channel + ":" + chatID + ":" + id
}
