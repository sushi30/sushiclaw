package utils

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTruncate(t *testing.T) {
	tests := []struct {
		s      string
		maxLen int
		want   string
	}{
		{"hello", 10, "hello"},
		{"hello world", 8, "hello..."},
		{"hi", 3, "hi"},
		{"hello", 3, "hel"},
		{"", 5, ""},
		{"hello", 0, ""},
		{"hello", -1, ""},
		{"日本語テスト", 4, "日..."},
	}
	for _, tc := range tests {
		got := Truncate(tc.s, tc.maxLen)
		if got != tc.want {
			t.Errorf("Truncate(%q, %d) = %q, want %q", tc.s, tc.maxLen, got, tc.want)
		}
	}
}

func TestTruncate_Disable(t *testing.T) {
	disableTruncation.Store(true)
	defer disableTruncation.Store(false)

	got := Truncate("hello world", 3)
	if got != "hello world" {
		t.Errorf("expected truncation disabled, got %q", got)
	}
}

func TestSanitizeMessageContent(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"hello\x00world", "helloworld"},
		{"hello\nworld", "hello\nworld"},
		{"hello\tworld", "hello\tworld"},
		{"normal text", "normal text"},
	}
	for _, tc := range tests {
		got := SanitizeMessageContent(tc.input)
		if got != tc.want {
			t.Errorf("SanitizeMessageContent(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"/etc/passwd", "passwd"},
		{"../../secret", "secret"},
		{"file/name", "name"},
		{"file\\name", "file_name"},
		{"normal.txt", "normal.txt"},
	}
	for _, tc := range tests {
		got := SanitizeFilename(tc.input)
		if got != tc.want {
			t.Errorf("SanitizeFilename(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestDownloadFile_InvalidURL(t *testing.T) {
	// Should return empty string for invalid URLs without panicking.
	got := DownloadFile("://invalid-url", "test.txt", DownloadOptions{})
	if got != "" {
		t.Errorf("expected empty string for invalid URL, got %q", got)
	}
}

func TestDownloadFile_Non200Status(t *testing.T) {
	// Start a local test server that returns 404.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	got := DownloadFile(ts.URL, "test.txt", DownloadOptions{Timeout: 5 * time.Second})
	if got != "" {
		t.Errorf("expected empty string for 404, got %q", got)
	}
}

func TestDownloadFile_Success(t *testing.T) {
	// Start a local test server.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello world"))
	}))
	defer ts.Close()

	got := DownloadFile(ts.URL, "test.bin", DownloadOptions{Timeout: 5 * time.Second})
	if got == "" {
		t.Fatal("expected non-empty path for successful download")
	}
	if !strings.Contains(got, "test.bin") {
		t.Errorf("expected path to contain filename, got %q", got)
	}
}
