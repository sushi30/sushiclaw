package identity

import (
	"testing"

	"github.com/sushi30/sushiclaw/pkg/bus"
)

func TestBuildCanonicalID(t *testing.T) {
	tests := []struct {
		platform, platformID, want string
	}{
		{"telegram", "123", "telegram:123"},
		{"Telegram", " 123 ", "telegram:123"},
		{"", "123", ""},
		{"telegram", "", ""},
		{"", "", ""},
	}
	for _, tc := range tests {
		got := BuildCanonicalID(tc.platform, tc.platformID)
		if got != tc.want {
			t.Errorf("BuildCanonicalID(%q, %q) = %q, want %q", tc.platform, tc.platformID, got, tc.want)
		}
	}
}

func TestParseCanonicalID(t *testing.T) {
	tests := []struct {
		input        string
		wantPlatform string
		wantID       string
		wantOK       bool
	}{
		{"telegram:123", "telegram", "123", true},
		{" telegram:123 ", "telegram", "123", true},
		{"telegram:", "", "", false},
		{":123", "", "", false},
		{"nocolon", "", "", false},
		{"", "", "", false},
	}
	for _, tc := range tests {
		p, id, ok := ParseCanonicalID(tc.input)
		if ok != tc.wantOK || p != tc.wantPlatform || id != tc.wantID {
			t.Errorf("ParseCanonicalID(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tc.input, p, id, ok, tc.wantPlatform, tc.wantID, tc.wantOK)
		}
	}
}

func TestMatchAllowed(t *testing.T) {
	sender := bus.SenderInfo{
		Platform:    "telegram",
		PlatformID:  "123456",
		CanonicalID: "telegram:123456",
		Username:    "alice",
	}

	tests := []struct {
		allowed string
		want    bool
	}{
		{"telegram:123456", true},
		{"telegram:999999", false},
		{"123456", true},
		{"123456|alice", true},
		{"123456|bob", true},
		{"@alice", true},
		{"@bob", false},
		{"", false},
		{"999999", false},
	}

	for _, tc := range tests {
		got := MatchAllowed(sender, tc.allowed)
		if got != tc.want {
			t.Errorf("MatchAllowed(sender, %q) = %v, want %v", tc.allowed, got, tc.want)
		}
	}
}

func TestMatchAllowed_CanonicalID(t *testing.T) {
	sender := bus.SenderInfo{
		Platform:    "whatsapp",
		PlatformID:  "+1234567890",
		CanonicalID: "whatsapp:+1234567890",
	}

	if !MatchAllowed(sender, "whatsapp:+1234567890") {
		t.Error("expected canonical ID match")
	}
	if MatchAllowed(sender, "whatsapp:+9999999999") {
		t.Error("expected no match for different canonical ID")
	}
}

func TestIsNumeric(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"123", true},
		{"-123", true},
		{"12a3", false},
		{"", false},
		{"0", true},
		{"-", false},
	}
	for _, tc := range tests {
		got := isNumeric(tc.input)
		if got != tc.want {
			t.Errorf("isNumeric(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}
