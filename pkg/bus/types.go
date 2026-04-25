package bus

// SenderInfo provides structured sender identity information.
type SenderInfo struct {
	Platform    string `json:"platform,omitempty"`
	PlatformID  string `json:"platform_id,omitempty"`
	CanonicalID string `json:"canonical_id,omitempty"`
	Username    string `json:"username,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
}

// InboundContext captures the normalized, platform-agnostic facts about an inbound message.
type InboundContext struct {
	Channel string `json:"channel"`
	Account string `json:"account,omitempty"`

	ChatID   string `json:"chat_id"`
	ChatType string `json:"chat_type,omitempty"`
	TopicID  string `json:"topic_id,omitempty"`

	SpaceID   string `json:"space_id,omitempty"`
	SpaceType string `json:"space_type,omitempty"`

	SenderID  string `json:"sender_id"`
	MessageID string `json:"message_id,omitempty"`

	Mentioned bool `json:"mentioned,omitempty"`

	ReplyToMessageID string `json:"reply_to_message_id,omitempty"`
	ReplyToSenderID  string `json:"reply_to_sender_id,omitempty"`

	ReplyHandles map[string]string `json:"reply_handles,omitempty"`
	Raw          map[string]string `json:"raw,omitempty"`
}

type InboundMessage struct {
	Context    InboundContext `json:"context"`
	Sender     SenderInfo     `json:"sender"`
	Content    string         `json:"content"`
	Media      []string       `json:"media,omitempty"`
	MediaScope string         `json:"media_scope,omitempty"`
	SessionKey string         `json:"session_key"`

	Channel   string `json:"channel"`
	SenderID  string `json:"sender_id"`
	ChatID    string `json:"chat_id"`
	MessageID string `json:"message_id,omitempty"`
}

// OutboundScope captures structured session scope without depending on the session package.
type OutboundScope struct {
	Version    int               `json:"version,omitempty"`
	AgentID    string            `json:"agent_id,omitempty"`
	Channel    string            `json:"channel,omitempty"`
	Account    string            `json:"account,omitempty"`
	Dimensions []string          `json:"dimensions,omitempty"`
	Values     map[string]string `json:"values,omitempty"`
}

type OutboundMessage struct {
	Channel          string         `json:"channel"`
	ChatID           string         `json:"chat_id"`
	Context          InboundContext `json:"context"`
	AgentID          string         `json:"agent_id,omitempty"`
	SessionKey       string         `json:"session_key,omitempty"`
	Scope            *OutboundScope `json:"scope,omitempty"`
	Content          string         `json:"content"`
	ReplyToMessageID string         `json:"reply_to_message_id,omitempty"`
}

// MediaPart describes a single media attachment to send.
type MediaPart struct {
	Type        string `json:"type"`
	Ref         string `json:"ref"`
	Caption     string `json:"caption,omitempty"`
	Filename    string `json:"filename,omitempty"`
	ContentType string `json:"content_type,omitempty"`
}

// OutboundMediaMessage carries media attachments from agent to channels.
type OutboundMediaMessage struct {
	Channel    string         `json:"channel"`
	ChatID     string         `json:"chat_id"`
	Context    InboundContext `json:"context"`
	AgentID    string         `json:"agent_id,omitempty"`
	SessionKey string         `json:"session_key,omitempty"`
	Scope      *OutboundScope `json:"scope,omitempty"`
	Parts      []MediaPart    `json:"parts"`
}

// AudioChunk represents a chunk of streaming voice data.
type AudioChunk struct {
	SessionID  string `json:"session_id"`
	SpeakerID  string `json:"speaker_id"`
	ChatID     string `json:"chat_id"`
	Channel    string `json:"channel"`
	Sequence   uint64 `json:"sequence"`
	Timestamp  uint32 `json:"timestamp"`
	SampleRate int    `json:"sample_rate"`
	Channels   int    `json:"channels"`
	Format     string `json:"format"`
	Data       []byte `json:"data"`
}

// VoiceControl represents state or commands for voice sessions.
type VoiceControl struct {
	SessionID string `json:"session_id"`
	ChatID    string `json:"chat_id"`
	Type      string `json:"type"`
	Action    string `json:"action"`
}
