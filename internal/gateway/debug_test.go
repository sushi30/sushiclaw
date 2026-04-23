package gateway

import (
	"strings"
	"testing"

	"github.com/sushi30/sushiclaw/pkg/bus"
)

// TestDebugManager_ToggleOnOff verifies that the first Toggle enables debug
// mode and the second disables it, with distinct reply strings each time.
func TestDebugManager_ToggleOnOff(t *testing.T) {
	extBus := bus.NewMessageBus()
	mgr := NewDebugManager(extBus)

	ctx := t.Context()

	reply1 := mgr.Toggle(ctx, "telegram", "chat1")
	if !mgr.active {
		t.Fatal("expected active=true after first Toggle")
	}
	if !strings.Contains(reply1, "enabled") {
		t.Fatalf("first Toggle reply should contain 'enabled', got %q", reply1)
	}

	reply2 := mgr.Toggle(ctx, "telegram", "chat1")
	if mgr.active {
		t.Fatal("expected active=false after second Toggle")
	}
	if !strings.Contains(reply2, "disabled") {
		t.Fatalf("second Toggle reply should contain 'disabled', got %q", reply2)
	}
}

// TestDebugManager_NoEventsWhenInactive verifies that when debug mode has not
// been toggled on, the manager is inactive.
func TestDebugManager_NoEventsWhenInactive(t *testing.T) {
	extBus := bus.NewMessageBus()
	mgr := NewDebugManager(extBus)

	if mgr.active {
		t.Fatal("expected active=false by default")
	}
}
