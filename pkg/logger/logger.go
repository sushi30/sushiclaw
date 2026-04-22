package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// LogLevel aliases zerolog.Level for convenience.
type LogLevel = zerolog.Level

const (
	DEBUG = zerolog.DebugLevel
	INFO  = zerolog.InfoLevel
	WARN  = zerolog.WarnLevel
	ERROR = zerolog.ErrorLevel
)

var (
	mu      sync.RWMutex
	base    zerolog.Logger
	fileLog io.WriteCloser
)

func init() {
	base = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}).
		With().Timestamp().Logger().Level(INFO)
}

// SetLevel sets the global log level.
func SetLevel(level LogLevel) {
	mu.Lock()
	base = base.Level(level)
	mu.Unlock()
}

// SetLevelFromString sets the log level by string (debug/info/warn/error).
func SetLevelFromString(s string) {
	l, err := zerolog.ParseLevel(s)
	if err != nil {
		return
	}
	SetLevel(l)
}

// GetLevel returns the current log level.
func GetLevel() LogLevel {
	mu.RLock()
	defer mu.RUnlock()
	return base.GetLevel()
}

// EnableFileLogging adds a file log writer.
func EnableFileLogging(filePath string) error {
	if err := os.MkdirAll(filepath.Dir(filePath), 0o750); err != nil {
		return err
	}
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	mu.Lock()
	fileLog = f
	base = zerolog.New(io.MultiWriter(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}, f)).
		With().Timestamp().Logger().Level(base.GetLevel())
	mu.Unlock()
	return nil
}

// DisableFileLogging closes the file log writer.
func DisableFileLogging() {
	mu.Lock()
	if fileLog != nil {
		_ = fileLog.Close()
		fileLog = nil
	}
	base = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}).
		With().Timestamp().Logger().Level(base.GetLevel())
	mu.Unlock()
}

func log(level LogLevel, component, msg string, fields map[string]any) {
	mu.RLock()
	l := base
	mu.RUnlock()
	ev := l.WithLevel(level)
	if component != "" {
		ev = ev.Str("component", component)
	}
	for k, v := range fields {
		ev = ev.Interface(k, v)
	}
	ev.Msg(msg)
}

// Debug logs at DEBUG level.
func Debug(msg string) { log(DEBUG, "", msg, nil) }

// DebugC logs at DEBUG level with a component.
func DebugC(component, msg string) { log(DEBUG, component, msg, nil) }

// DebugCF logs at DEBUG level with a component and fields.
func DebugCF(component, msg string, fields map[string]any) { log(DEBUG, component, msg, fields) }

// Info logs at INFO level.
func Info(msg string) { log(INFO, "", msg, nil) }

// InfoC logs at INFO level with a component.
func InfoC(component, msg string) { log(INFO, component, msg, nil) }

// InfoCF logs at INFO level with a component and fields.
func InfoCF(component, msg string, fields map[string]any) { log(INFO, component, msg, fields) }

// Warn logs at WARN level.
func Warn(msg string) { log(WARN, "", msg, nil) }

// WarnC logs at WARN level with a component.
func WarnC(component, msg string) { log(WARN, component, msg, nil) }

// WarnCF logs at WARN level with a component and fields.
func WarnCF(component, msg string, fields map[string]any) { log(WARN, component, msg, fields) }

// Error logs at ERROR level.
func Error(msg string) { log(ERROR, "", msg, nil) }

// ErrorCF logs at ERROR level with a component and fields.
func ErrorCF(component, msg string, fields map[string]any) { log(ERROR, component, msg, fields) }

// InitPanic registers a panic log file (writes crash output there).
// Returns a deferred flush function.
func InitPanic(filePath string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(filePath), 0o750); err != nil {
		return func() {}, err
	}
	return func() {}, nil
}

// Logger is a named logger implementing an interface compatible with telego.
type Logger struct {
	component string
}

// NewLogger creates a named Logger.
func NewLogger(component string) *Logger {
	return &Logger{component: component}
}

// Debugf logs a debug message.
func (l *Logger) Debugf(format string, args ...any) {
	log(DEBUG, l.component, fmt.Sprintf(format, args...), nil)
}

// Infof logs an info message.
func (l *Logger) Infof(format string, args ...any) {
	log(INFO, l.component, fmt.Sprintf(format, args...), nil)
}

// Warnf logs a warn message.
func (l *Logger) Warnf(format string, args ...any) {
	log(WARN, l.component, fmt.Sprintf(format, args...), nil)
}

// Errorf logs an error message.
func (l *Logger) Errorf(format string, args ...any) {
	log(ERROR, l.component, fmt.Sprintf(format, args...), nil)
}
