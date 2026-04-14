// Package envresolve is a sushiclaw shim that resolves env://VAR_NAME
// references in picoclaw config fields. Upstream picoclaw's SecureString
// resolver handles enc:// and file:// but not env://; this package fills
// that gap until the fix lands in sipeed/picoclaw.
package envresolve

import (
	"fmt"
	"os"
	"strings"

	"github.com/sipeed/picoclaw/pkg/config"
)

// Config resolves all env://VAR_NAME references in cfg in-place.
func Config(cfg *config.Config) {
	for _, model := range cfg.ModelList {
		for _, key := range model.APIKeys {
			resolveSecureString(key)
		}
	}
}

// SecureString resolves a single env://VAR_NAME reference in-place.
func SecureString(s *config.SecureString) {
	resolveSecureString(s)
}

// SecureStringRequired resolves a single env://VAR_NAME reference in-place and
// returns an error if the referenced environment variable is not set.
func SecureStringRequired(s *config.SecureString) error {
	resolveSecureString(s)
	if varName, ok := strings.CutPrefix(s.String(), "env://"); ok {
		return fmt.Errorf("env var %s is not set", varName)
	}
	return nil
}

func resolveSecureString(s *config.SecureString) {
	if s == nil {
		return
	}
	v := s.String()
	if !strings.HasPrefix(v, "env://") {
		return
	}
	if resolved := os.Getenv(strings.TrimPrefix(v, "env://")); resolved != "" {
		s.Set(resolved)
	}
}
