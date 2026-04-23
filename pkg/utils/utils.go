package utils

import (
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/google/uuid"

	"github.com/sushi30/sushiclaw/pkg/logger"
	"github.com/sushi30/sushiclaw/pkg/media"
)

var disableTruncation atomic.Bool

// Truncate returns a truncated string with at most maxLen runes.
func Truncate(s string, maxLen int) string {
	if disableTruncation.Load() {
		return s
	}
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-3]) + "..."
}

// SanitizeMessageContent removes Unicode control and format characters.
func SanitizeMessageContent(input string) string {
	var sb strings.Builder
	sb.Grow(len(input))
	for _, r := range input {
		if unicode.IsGraphic(r) || r == '\n' || r == '\r' || r == '\t' {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// SanitizeFilename returns a safe base filename.
func SanitizeFilename(filename string) string {
	base := filepath.Base(filename)
	base = strings.ReplaceAll(base, "..", "")
	base = strings.ReplaceAll(base, "/", "_")
	base = strings.ReplaceAll(base, "\\", "_")
	return base
}

// DownloadOptions holds optional parameters for DownloadFile.
type DownloadOptions struct {
	Timeout      time.Duration
	ExtraHeaders map[string]string
	LoggerPrefix string
	ProxyURL     string
}

// DownloadFile downloads a file from urlStr to a local temp directory.
// Returns the local file path or empty string on error.
func DownloadFile(urlStr, filename string, opts DownloadOptions) string {
	if opts.Timeout == 0 {
		opts.Timeout = 60 * time.Second
	}
	if opts.LoggerPrefix == "" {
		opts.LoggerPrefix = "utils"
	}

	mediaDir := media.TempDir()
	if err := os.MkdirAll(mediaDir, 0o700); err != nil {
		logger.ErrorCF(opts.LoggerPrefix, "Failed to create media directory", map[string]any{
			"error": err.Error(),
		})
		return ""
	}

	safeName := SanitizeFilename(filename)
	localPath := filepath.Join(mediaDir, uuid.New().String()[:8]+"_"+safeName)

	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		logger.ErrorCF(opts.LoggerPrefix, "Failed to create download request", map[string]any{
			"error": err.Error(),
		})
		return ""
	}

	for key, value := range opts.ExtraHeaders {
		req.Header.Set(key, value)
	}

	client := &http.Client{Timeout: opts.Timeout}
	if opts.ProxyURL != "" {
		proxyURL, parseErr := url.Parse(opts.ProxyURL)
		if parseErr == nil {
			client.Transport = &http.Transport{Proxy: http.ProxyURL(proxyURL)}
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		logger.ErrorCF(opts.LoggerPrefix, "Failed to download file", map[string]any{
			"error": err.Error(),
			"url":   urlStr,
		})
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.ErrorCF(opts.LoggerPrefix, "File download returned non-200 status", map[string]any{
			"status": resp.StatusCode,
			"url":    urlStr,
		})
		return ""
	}

	out, err := os.Create(localPath)
	if err != nil {
		logger.ErrorCF(opts.LoggerPrefix, "Failed to create local file", map[string]any{
			"error": err.Error(),
		})
		return ""
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		out.Close()
		os.Remove(localPath)
		logger.ErrorCF(opts.LoggerPrefix, "Failed to write file", map[string]any{
			"error": err.Error(),
		})
		return ""
	}

	return localPath
}
