package channels

// VoiceCapabilities describes whether ASR and TTS are available for a channel.
type VoiceCapabilities struct {
	ASR bool
	TTS bool
}

// VoiceCapabilityProvider is an optional interface for channels that want to
// explicitly declare their ASR/TTS support.
type VoiceCapabilityProvider interface {
	VoiceCapabilities() VoiceCapabilities
}

// MediaSender is an optional interface for channels that can send media attachments.
// Declared here (and in manager.go locally) for use in voice capability detection.
type MediaSender interface {
	SendMedia(ctx interface{ Done() <-chan struct{} }, msg interface{}) ([]string, error)
}

var asrCapableChannels = map[string]bool{
	"discord":  true,
	"telegram": true,
	"matrix":   true,
	"qq":       true,
	"weixin":   true,
	"line":     true,
	"feishu":   true,
	"onebot":   true,
}

// DetectVoiceCapabilities returns ASR/TTS availability for a channel.
func DetectVoiceCapabilities(channelName string, ch Channel, asrAvailable bool, ttsAvailable bool) VoiceCapabilities {
	if ch == nil {
		return VoiceCapabilities{}
	}
	if vcp, ok := ch.(VoiceCapabilityProvider); ok {
		caps := vcp.VoiceCapabilities()
		if !asrAvailable {
			caps.ASR = false
		}
		if !ttsAvailable {
			caps.TTS = false
		}
		return caps
	}
	caps := VoiceCapabilities{}
	if asrAvailable {
		caps.ASR = asrCapableChannels[channelName]
	}
	return caps
}
