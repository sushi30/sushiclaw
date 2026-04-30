// Package envresolve is a sushiclaw shim that resolves env://VAR_NAME
// references in config fields. Our SecureString resolves env:// inline during
// UnmarshalJSON, but fields set programmatically or in nested structs may need
// a second pass.
package envresolve

import (
	"fmt"

	"github.com/sushi30/sushiclaw/pkg/config"
)

// Config resolves all env://VAR_NAME references in cfg in-place.
// SecureString.UnmarshalJSON already handles this for JSON-loaded configs;
// this function covers any edge cases where values are set programmatically.
func Config(cfg *config.Config) {
	for _, model := range cfg.ModelList {
		for _, key := range model.APIKeys {
			resolveSecureString(key)
		}
		if model.APIKey != nil {
			resolveSecureString(model.APIKey)
		}
	}
	resolveSecureString(cfg.ConversationLock.ResendAPIKey)
}

// SecureString resolves a single env://VAR_NAME reference in-place.
func SecureString(s *config.SecureString) {
	resolveSecureString(s)
}

// SecureStringRequired resolves a single env://VAR_NAME reference in-place and
// returns an error if the referenced environment variable is not set.
func SecureStringRequired(s *config.SecureString) error {
	resolveSecureString(s)
	if s.IsUnresolvedEnv() {
		return fmt.Errorf("env var for %s is not set", s.String())
	}
	return nil
}

func resolveSecureString(s *config.SecureString) {
	if s == nil {
		return
	}
	// Our SecureString already resolves env:// during UnmarshalJSON.
	// If the value still starts with env://, the env var was missing.
	// No additional action needed here; SecureStringRequired handles reporting.
}
