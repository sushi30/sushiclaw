package channels

import "errors"

var (
	ErrNotRunning = errors.New("channel not running")
	ErrRateLimit  = errors.New("rate limited")
	ErrTemporary  = errors.New("temporary failure")
)

// ErrSendFailed is declared in manager.go
