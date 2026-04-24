package media

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTempDir(t *testing.T) {
	dir := TempDir()
	if dir == "" {
		t.Fatal("TempDir returned empty string")
	}
	if !filepath.IsAbs(dir) {
		t.Errorf("TempDir should be absolute, got %q", dir)
	}
}

func TestNewFileMediaStore(t *testing.T) {
	s := NewFileMediaStore()
	if s == nil {
		t.Fatal("NewFileMediaStore returned nil")
	}
}

func TestFileMediaStore_StoreAndResolve(t *testing.T) {
	s := NewFileMediaStore()

	// Create a temp file to store
	tmpFile := filepath.Join(t.TempDir(), "test.txt")
	if err := os.WriteFile(tmpFile, []byte("hello"), 0o600); err != nil {
		t.Fatalf("create temp file: %v", err)
	}

	ref, err := s.Store(tmpFile, MediaMeta{Filename: "test.txt"}, "scope1")
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	if ref == "" {
		t.Fatal("Store returned empty ref")
	}

	// Resolve
	resolved, err := s.Resolve(ref)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved != tmpFile {
		t.Errorf("Resolve = %q, want %q", resolved, tmpFile)
	}

	// ResolveWithMeta
	resolved2, meta, err := s.ResolveWithMeta(ref)
	if err != nil {
		t.Fatalf("ResolveWithMeta: %v", err)
	}
	if resolved2 != tmpFile {
		t.Errorf("ResolveWithMeta path = %q, want %q", resolved2, tmpFile)
	}
	if meta.Filename != "test.txt" {
		t.Errorf("ResolveWithMeta meta.Filename = %q, want %q", meta.Filename, "test.txt")
	}
}

func TestFileMediaStore_StoreMissingFile(t *testing.T) {
	s := NewFileMediaStore()
	_, err := s.Store("/nonexistent/path", MediaMeta{}, "scope")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestFileMediaStore_ResolveUnknown(t *testing.T) {
	s := NewFileMediaStore()
	_, err := s.Resolve("media://unknown")
	if err == nil {
		t.Error("expected error for unknown ref")
	}
}

func TestFileMediaStore_ReleaseAll(t *testing.T) {
	s := NewFileMediaStore()

	tmpFile := filepath.Join(t.TempDir(), "release.txt")
	if err := os.WriteFile(tmpFile, []byte("hello"), 0o600); err != nil {
		t.Fatalf("create test file: %v", err)
	}

	ref, _ := s.Store(tmpFile, MediaMeta{Filename: "release.txt", CleanupPolicy: CleanupPolicyDeleteOnCleanup}, "scope1")

	// Release
	if err := s.ReleaseAll("scope1"); err != nil {
		t.Fatalf("ReleaseAll: %v", err)
	}

	// Should no longer resolve
	_, err := s.Resolve(ref)
	if err == nil {
		t.Error("expected error after release")
	}

	// File should be deleted (delete_on_cleanup policy)
	if _, err := os.Stat(tmpFile); !os.IsNotExist(err) {
		t.Error("expected file to be deleted")
	}
}

func TestFileMediaStore_CleanExpired(t *testing.T) {
	s := NewFileMediaStoreWithCleanup(MediaCleanerConfig{
		Enabled:  true,
		MaxAge:   time.Hour,
		Interval: time.Minute,
	})

	// Override nowFunc to simulate old entries
	oldTime := time.Now().Add(-2 * time.Hour)
	s.nowFunc = func() time.Time { return oldTime }

	tmpFile := filepath.Join(t.TempDir(), "old.txt")
	if err := os.WriteFile(tmpFile, []byte("old"), 0o600); err != nil {
		t.Fatalf("create test file: %v", err)
	}

	ref, _ := s.Store(tmpFile, MediaMeta{Filename: "old.txt", CleanupPolicy: CleanupPolicyDeleteOnCleanup}, "scope1")

	// Restore nowFunc to current time
	s.nowFunc = time.Now

	// Clean expired
	count := s.CleanExpired()
	if count != 1 {
		t.Errorf("CleanExpired = %d, want 1", count)
	}

	// Should no longer resolve
	_, err := s.Resolve(ref)
	if err == nil {
		t.Error("expected error after cleaning expired")
	}
}

func TestFileMediaStore_CleanExpired_NotExpired(t *testing.T) {
	s := NewFileMediaStoreWithCleanup(MediaCleanerConfig{
		Enabled:  true,
		MaxAge:   time.Hour,
		Interval: time.Minute,
	})

	tmpFile := filepath.Join(t.TempDir(), "fresh.txt")
	if err := os.WriteFile(tmpFile, []byte("fresh"), 0o600); err != nil {
		t.Fatalf("create test file: %v", err)
	}

	ref, _ := s.Store(tmpFile, MediaMeta{Filename: "fresh.txt"}, "scope1")

	count := s.CleanExpired()
	if count != 0 {
		t.Errorf("CleanExpired = %d, want 0", count)
	}

	// Should still resolve
	_, err := s.Resolve(ref)
	if err != nil {
		t.Errorf("expected ref to still exist: %v", err)
	}
}

func TestFileMediaStore_StartStop(t *testing.T) {
	s := NewFileMediaStoreWithCleanup(MediaCleanerConfig{
		Enabled:  true,
		MaxAge:   time.Hour,
		Interval: 10 * time.Millisecond,
	})

	s.Start()
	time.Sleep(25 * time.Millisecond) // let it tick a couple times
	s.Stop()

	// Should not panic if Stop called twice
	s.Stop()
}

func TestNormalizeCleanupPolicy(t *testing.T) {
	tests := []struct {
		input, want CleanupPolicy
	}{
		{"", CleanupPolicyDeleteOnCleanup},
		{CleanupPolicyDeleteOnCleanup, CleanupPolicyDeleteOnCleanup},
		{CleanupPolicyForgetOnly, CleanupPolicyForgetOnly},
		{"unknown", CleanupPolicyDeleteOnCleanup},
	}
	for _, tc := range tests {
		got := normalizeCleanupPolicy(tc.input)
		if got != tc.want {
			t.Errorf("normalizeCleanupPolicy(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
