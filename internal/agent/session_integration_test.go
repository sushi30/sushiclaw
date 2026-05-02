package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sushi30/sushiclaw/pkg/bus"
	"github.com/sushi30/sushiclaw/pkg/config"
)

// TestSessionIsolation verifies that two sessions with different keys do not
// share conversation memory.
func TestSessionIsolation(t *testing.T) {
	ws := t.TempDir()
	skillsDir := filepath.Join(ws, "skills", "python")
	require.NoError(t, os.MkdirAll(skillsDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(skillsDir, "SKILL.md"), []byte("You are a Python expert."), 0644))

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{ModelName: "test-model"},
		},
		ModelList: []config.ModelConfig{
			{
				ModelName: "test-model",
				Model:     "gpt-4o",
				APIKey:    config.NewSecureString("test-key"),
			},
		},
	}
	cfg.Agents.Defaults.Workspace = ws
	cfg.Sessions.Directory = t.TempDir()

	sm, err := NewSessionManager(cfg, bus.NewMessageBus(), nil, nil)
	require.NoError(t, err)

	// Activate a skill in session A.
	err = sm.ActivateSkill("session-a", "python")
	require.NoError(t, err)

	// Session A should have the skill in memory.
	msgsA, err := sm.GetMessages("session-a", context.Background())
	require.NoError(t, err)
	require.Len(t, msgsA, 1)
	assert.Equal(t, "You are a Python expert.", msgsA[0].Content)

	// Session B should have no messages.
	msgsB, err := sm.GetMessages("session-b", context.Background())
	require.NoError(t, err)
	assert.Empty(t, msgsB)

	// Activating the same skill in session B should succeed (independent state).
	err = sm.ActivateSkill("session-b", "python")
	require.NoError(t, err)

	msgsB, err = sm.GetMessages("session-b", context.Background())
	require.NoError(t, err)
	require.Len(t, msgsB, 1)
	assert.Equal(t, "You are a Python expert.", msgsB[0].Content)

	// Session A should still only have one message.
	msgsA, err = sm.GetMessages("session-a", context.Background())
	require.NoError(t, err)
	require.Len(t, msgsA, 1)
}

// TestClearHistoryEvictsSession verifies that ClearHistory removes the session
// from the registry so a subsequent dispatch creates a fresh empty session.
func TestClearHistoryEvictsSession(t *testing.T) {
	ws := t.TempDir()
	skillsDir := filepath.Join(ws, "skills", "python")
	require.NoError(t, os.MkdirAll(skillsDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(skillsDir, "SKILL.md"), []byte("You are a Python expert."), 0644))

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{ModelName: "test-model"},
		},
		ModelList: []config.ModelConfig{
			{
				ModelName: "test-model",
				Model:     "gpt-4o",
				APIKey:    config.NewSecureString("test-key"),
			},
		},
	}
	cfg.Agents.Defaults.Workspace = ws
	cfg.Sessions.Directory = t.TempDir()

	sm, err := NewSessionManager(cfg, bus.NewMessageBus(), nil, nil)
	require.NoError(t, err)

	// Load a skill into the session.
	err = sm.ActivateSkill("telegram:chat1", "python")
	require.NoError(t, err)

	msgs, err := sm.GetMessages("telegram:chat1", context.Background())
	require.NoError(t, err)
	require.Len(t, msgs, 1)

	// Clear history should evict the session entirely.
	err = sm.ClearHistory("telegram:chat1")
	require.NoError(t, err)

	msgs, err = sm.GetMessages("telegram:chat1", context.Background())
	require.NoError(t, err)
	assert.Empty(t, msgs)

	// Re-activating the skill should work again (fresh session).
	err = sm.ActivateSkill("telegram:chat1", "python")
	require.NoError(t, err)

	msgs, err = sm.GetMessages("telegram:chat1", context.Background())
	require.NoError(t, err)
	require.Len(t, msgs, 1)
}

func TestSessionManagerPersistsSessionAcrossManagers(t *testing.T) {
	ws := t.TempDir()
	sessionsDir := t.TempDir()
	skillsDir := filepath.Join(ws, "skills", "python")
	require.NoError(t, os.MkdirAll(skillsDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(skillsDir, "SKILL.md"), []byte("Persistent skill."), 0644))

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{ModelName: "test-model", Workspace: ws},
		},
		ModelList: []config.ModelConfig{
			{
				ModelName: "test-model",
				Model:     "gpt-4o",
				APIKey:    config.NewSecureString("test-key"),
			},
		},
		Sessions: config.SessionsConfig{Directory: sessionsDir},
	}

	sm, err := NewSessionManager(cfg, bus.NewMessageBus(), nil, nil)
	require.NoError(t, err)
	require.NoError(t, sm.ActivateSkill("telegram:chat1", "python"))
	sm.Stop()

	reopened, err := NewSessionManager(cfg, bus.NewMessageBus(), nil, nil)
	require.NoError(t, err)
	defer reopened.Stop()

	session, err := reopened.getOrCreateSession("telegram:chat1")
	require.NoError(t, err)
	msgs, err := session.mem.GetMessages(context.Background())
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "Persistent skill.", msgs[0].Content)
}

func TestClearHistoryDeletesPersistedSession(t *testing.T) {
	ws := t.TempDir()
	sessionsDir := t.TempDir()
	skillsDir := filepath.Join(ws, "skills", "python")
	require.NoError(t, os.MkdirAll(skillsDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(skillsDir, "SKILL.md"), []byte("Clear me."), 0644))

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{ModelName: "test-model", Workspace: ws},
		},
		ModelList: []config.ModelConfig{
			{
				ModelName: "test-model",
				Model:     "gpt-4o",
				APIKey:    config.NewSecureString("test-key"),
			},
		},
		Sessions: config.SessionsConfig{Directory: sessionsDir},
	}

	sm, err := NewSessionManager(cfg, bus.NewMessageBus(), nil, nil)
	require.NoError(t, err)
	require.NoError(t, sm.ActivateSkill("telegram:chat1", "python"))
	sm.Stop()

	clearer, err := NewSessionManager(cfg, bus.NewMessageBus(), nil, nil)
	require.NoError(t, err)
	require.NoError(t, clearer.ClearHistory("telegram:chat1"))
	clearer.Stop()

	reopened, err := NewSessionManager(cfg, bus.NewMessageBus(), nil, nil)
	require.NoError(t, err)
	defer reopened.Stop()

	session, err := reopened.getOrCreateSession("telegram:chat1")
	require.NoError(t, err)
	msgs, err := session.mem.GetMessages(context.Background())
	require.NoError(t, err)
	assert.Empty(t, msgs)
}

// TestSessionManagerEviction verifies that stale sessions are evicted by the
// background cleanup goroutine without deleting persisted history.
func TestSessionManagerEviction(t *testing.T) {
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{ModelName: "test-model"},
		},
		ModelList: []config.ModelConfig{
			{
				ModelName: "test-model",
				Model:     "gpt-4o",
				APIKey:    config.NewSecureString("test-key"),
			},
		},
	}
	cfg.Sessions.Directory = t.TempDir()

	sm, err := NewSessionManager(cfg, bus.NewMessageBus(), nil, nil)
	require.NoError(t, err)

	// Force a very short TTL and cleanup interval for testing.
	sm.ttl = 50 * time.Millisecond
	sm.cleanupInterval = 50 * time.Millisecond
	sm.Start()
	defer sm.Stop()

	// Create a session by activating a skill.
	ws := t.TempDir()
	skillsDir := filepath.Join(ws, "skills", "python")
	require.NoError(t, os.MkdirAll(skillsDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(skillsDir, "SKILL.md"), []byte("Skill content."), 0644))
	cfg.Agents.Defaults.Workspace = ws

	err = sm.ActivateSkill("old-session", "python")
	require.NoError(t, err)

	// Verify session exists.
	msgs, err := sm.GetMessages("old-session", context.Background())
	require.NoError(t, err)
	require.Len(t, msgs, 1)

	// Wait for the session to become stale and be evicted.
	time.Sleep(200 * time.Millisecond)

	msgs, err = sm.GetMessages("old-session", context.Background())
	require.NoError(t, err)
	assert.Empty(t, msgs)

	session, err := sm.getOrCreateSession("old-session")
	require.NoError(t, err)
	msgs, err = session.mem.GetMessages(context.Background())
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "Skill content.", msgs[0].Content)
}

// TestDispatchUsesSessionKey verifies that Dispatch routes to the correct
// session based on the inbound message's SessionKey.
func TestDispatchUsesSessionKey(t *testing.T) {
	extBus := bus.NewMessageBus()
	progress := &collectingProgress{}

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{ModelName: "test-model"},
		},
		ModelList: []config.ModelConfig{
			{
				ModelName: "test-model",
				Model:     "gpt-4o",
				APIKey:    config.NewSecureString("test-key"),
			},
		},
	}
	cfg.Sessions.Directory = t.TempDir()

	sm, err := NewSessionManager(cfg, extBus, nil, nil, WithProgressSink(progress))
	require.NoError(t, err)

	// Use a mock session directly to avoid real LLM calls.
	sm.mu.Lock()
	sessionA := &Session{
		agent:           &mockRunner{runResult: "response-a"},
		mem:             NewInMemoryMemory(),
		activatedSkills: make(map[string]bool),
		lastUsed:        time.Now(),
		mgr:             sm,
	}
	sm.sessions["email:thread-1"] = sessionA

	sessionB := &Session{
		agent:           &mockRunner{runResult: "response-b"},
		mem:             NewInMemoryMemory(),
		activatedSkills: make(map[string]bool),
		lastUsed:        time.Now(),
		mgr:             sm,
	}
	sm.sessions["email:thread-2"] = sessionB
	sm.mu.Unlock()

	// Dispatch to session A.
	sm.Dispatch(context.Background(), bus.InboundMessage{
		Channel:    "email",
		ChatID:     "user@example.com",
		Content:    "hello",
		SessionKey: "email:thread-1",
	})

	msgA := requireOutboundMessage(t, extBus)
	assert.Equal(t, "response-a", msgA.Content)
	assert.Equal(t, "email:thread-1", msgA.SessionKey)

	// Dispatch to session B.
	sm.Dispatch(context.Background(), bus.InboundMessage{
		Channel:    "email",
		ChatID:     "user@example.com",
		Content:    "world",
		SessionKey: "email:thread-2",
	})

	msgB := requireOutboundMessage(t, extBus)
	assert.Equal(t, "response-b", msgB.Content)
	assert.Equal(t, "email:thread-2", msgB.SessionKey)
}

// TestComputeSessionKey verifies the session key computation logic.
func TestComputeSessionKey(t *testing.T) {
	cases := []struct {
		name     string
		msg      bus.InboundMessage
		expected string
	}{
		{
			name:     "explicit SessionKey",
			msg:      bus.InboundMessage{Channel: "telegram", ChatID: "123", SessionKey: "custom:key"},
			expected: "custom:key",
		},
		{
			name:     "fallback to channel:chat_id",
			msg:      bus.InboundMessage{Channel: "telegram", ChatID: "123"},
			expected: "telegram:123",
		},
		{
			name:     "empty SessionKey falls back",
			msg:      bus.InboundMessage{Channel: "whatsapp", ChatID: "456", SessionKey: ""},
			expected: "whatsapp:456",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := computeSessionKey(c.msg)
			assert.Equal(t, c.expected, got)
		})
	}
}

// TestOutboundMessageIncludesSessionKey verifies that handleInbound propagates
// the session key to outbound messages.
func TestOutboundMessageIncludesSessionKey(t *testing.T) {
	extBus := bus.NewMessageBus()
	progress := &collectingProgress{}
	sm := &SessionManager{bus: extBus, progress: progress}
	session := &Session{agent: &mockRunner{runResult: "hi"}, mgr: sm}

	session.handleInbound(context.Background(), bus.InboundMessage{
		Channel: "telegram",
		ChatID:  "chat1",
		Content: "hello",
	}, "telegram:chat1")

	msg := requireOutboundMessage(t, extBus)
	assert.Equal(t, "hi", msg.Content)
	assert.Equal(t, "telegram:chat1", msg.SessionKey)
}

// mockRunner is reused from session_debug_test.go.
var _ agentRunner = (*mockRunner)(nil)

// collectingProgress is reused from session_debug_test.go.
var _ ProgressSink = (*collectingProgress)(nil)

// requireOutboundMessage is reused from session_debug_test.go.
