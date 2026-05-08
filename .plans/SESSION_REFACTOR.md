<!-- markdownlint-disable MD013 MD010 -->

# Session Refactor Brainstorm

## Problem Statement

`internal/agent/session.go` has grown into a coordinator plus several embedded subsystems. It now mixes agent construction, session persistence, lifecycle management, turn execution, transcription, media handling, progress reporting, summarization, outbound publishing, and convenience APIs.

The desired refactor is a hybrid service-oriented approach: keep lifecycle interfaces internal for now, but design them cleanly enough that they could become a public extension API later if needed.

## Current Responsibilities in `session.go`

- Agent construction and LLM/provider setup
- Session registry, persistence, TTL eviction, and lineage
- Turn lifecycle and cancellation
- Inbound message normalization
- Audio transcription
- Media reference expansion
- Tool context injection
- Agent execution, streaming, usage extraction, and progress events
- Context overflow summarization
- Outbound success/error publishing
- Skill activation and model/skill introspection

## Target Shape

Keep `SessionManager` as the stable facade used by gateway, chat, and commands. Move internal responsibilities into smaller components:

- `SessionManager`: owns high-level orchestration and preserves the current public API.
- `SessionRegistry`: owns session map, persistence, lineage, TTL eviction, clearing, and closing.
- `TurnRunner`: owns ordered turn execution.
- Services: attach behavior to manager, session, and turn lifecycle hooks.

The refactor should not expose callers to the new internals unless there is a concrete reason. Existing calls like `Dispatch`, `Chat`, `ClearHistory`, `StopTurn`, `ListSkills`, and `GetModelInfo` should remain stable.

## Services Skeleton

The services layer is the architectural spine. It defines how optional behavior attaches to session lifecycle without hard-coding every concern into `SessionManager`.

Prefer small optional interfaces over one large interface. A service should implement only the hooks it needs.

Possible foundation:

```go
type SessionService interface {
	Name() string
}

type ManagerStartService interface {
	Start(ctx context.Context, sm *SessionManager) error
}

type ManagerStopService interface {
	Stop(ctx context.Context, sm *SessionManager) error
}

type SessionCreatedService interface {
	SessionCreated(ctx context.Context, session *Session) error
}

type SessionClosingService interface {
	SessionClosing(ctx context.Context, session *Session) error
}

type BeforeTurnService interface {
	BeforeTurn(ctx context.Context, turn *Turn) error
}

type BeforeAgentRunService interface {
	BeforeAgentRun(ctx context.Context, turn *Turn) error
}

type AfterAgentRunService interface {
	AfterAgentRun(ctx context.Context, turn *Turn) error
}

type TurnFailedService interface {
	TurnFailed(ctx context.Context, turn *Turn) error
}

type AfterTurnService interface {
	AfterTurn(ctx context.Context, turn *Turn) error
}
```

The most important new abstraction is probably `Turn`, because it gives all services a shared typed object to operate on.

```go
type Turn struct {
	Session    *Session
	Message    bus.InboundMessage
	SessionKey string

	Input     string
	Response  string
	Usage     *interfaces.TokenUsage
	ToolCalls int
	Err       error

	StartedAt time.Time
}
```

The exact fields can evolve during planning. The important point is that turn state should stop being scattered across locals in a long `handleInbound` method.

## Session Registry

The session registry is already a subsystem embedded inside `SessionManager`.

Current responsibilities to extract:

- Active session map
- Locking around session lookup and mutation
- TTL eviction loop support
- SQLite memory creation
- Session lineage resolution
- Active backing key lookup
- History clearing
- Closing session memory
- Stopping active turns during eviction, clear, and manager stop

Possible shape:

```go
type SessionRegistry struct {
	cfg             *config.Config
	sessions        map[string]*Session
	factory         SessionFactory
	ttl             time.Duration
	cleanupInterval time.Duration
}
```

Possible methods:

```go
func (r *SessionRegistry) GetOrCreate(ctx context.Context, stableKey string) (*Session, error)
func (r *SessionRegistry) Get(stableKey string) *Session
func (r *SessionRegistry) Clear(ctx context.Context, stableKey string) error
func (r *SessionRegistry) EvictStale(ctx context.Context) error
func (r *SessionRegistry) CloseAll(ctx context.Context) error
```

This gives natural lifecycle hook points:

- `SessionCreated`
- `SessionClosing`
- `SessionEvicted`
- `SessionCleared`

These hooks matter for future services such as metrics, debug state, memory backends, locks, audit trails, or per-session resources.

## Turn Pipeline

The turn pipeline is where most of the monolith pressure lives.

The current `handleInbound` method is a linear workflow, but the workflow is implicit. The refactor should make it explicit:

1. Compute session key.
2. Get or create session.
3. Cancel the previous active turn and create turn context.
4. Build a `Turn`.
5. Preprocess inbound message.
6. Build agent input.
7. Emit turn-start progress.
8. Maybe summarize before the run.
9. Run the agent.
10. Maybe recover from context overflow via summary handoff.
11. Publish response or error.
12. Emit logging/progress summary.
13. Finish cancellation/bookkeeping.

Possible coordinator:

```go
type TurnRunner struct {
	services []SessionService
}

func (r *TurnRunner) Run(ctx context.Context, turn *Turn) {
	r.beforeTurn(ctx, turn)
	r.beforeAgentRun(ctx, turn)
	r.runAgent(ctx, turn)
	r.afterAgentRun(ctx, turn)
	r.publish(ctx, turn)
	r.afterTurn(ctx, turn)
}
```

The implementation should remain deterministic: synchronous, ordered, and explicit. Avoid a generic event bus like `On(event string, payload any)`. Typed hooks are easier to test, easier to reason about, and preserve compile-time safety.

## Candidate Services

### Transcription Service

Owns audio media handling:

- Resolve media refs with metadata.
- Detect audio files.
- Run ASR transcription.
- Replace `[voice]` or `[audio]` annotations with transcription tags.
- Preserve non-audio media refs.
- Produce user-facing failure behavior when audio was present but could not be transcribed.

Current code candidate: `transcribeAudioInMessage`.

### Media Input Service

Owns media refs in agent input:

- Resolve `media://` refs to local paths when possible.
- Preserve unresolved refs.
- Append attached file descriptions to `turn.Input`.
- Keep media input formatting consistent.

Current code candidate: media block inside `handleInbound`.

### Tool Context Service

Owns context enrichment before agent execution:

- Attach chat ID for exec/tool use.
- Attach channel.
- Attach sender ID.
- Attach inbound context.

Current code candidate: `exec.WithChatID`, `toolctx.WithChannel`, `toolctx.WithSenderID`, and `toolctx.WithInboundContext` block.

### Progress Service

Owns progress events:

- Turn started.
- First activity.
- Tool call started.
- Tool call finished.
- Heartbeat.
- Fallback.
- Completed.
- Failed.
- Final progress summary.

Current code candidates: `emitProgress`, `emitSummary`, heartbeat handling in streaming, and completion/failure blocks in `handleInbound`.

### Summarization Service

Owns context-window recovery and proactive summary handoff:

- Decide whether to summarize before a turn.
- Generate summaries.
- Chunk oversized transcripts.
- Create new backing session keys.
- Seed new memory with summary.
- Swap session memory and agent.
- Recover from context overflow errors.

Current code candidates: `shouldSummarizeBeforeTurn`, `handoffToSummarySession`, `generateSummary`, `chunkedSummary`, `buildSummaryInput`, `structuredSummaryPrompt`, `isContextOverflowError`, `nextSummarySessionKey`, `approximateSessionTokens`, `approximateTokenCount`, and `splitTranscriptForSummary`.

### Outbound Service

Owns bus publishing for turn results:

- Publish successful agent response.
- Publish system error response.
- Publish transcription failure response.
- Preserve channel, chat ID, and session key.

Current code candidate: outbound publish blocks inside `handleInbound` and session creation error handling in `Dispatch`.

### Turn Logging Service

Owns structured turn logs:

- Duration.
- Tool calls.
- Response bytes.
- Token usage.
- Error state.

Current code candidate: `logTurnSummary`.

### Agent Run Service

Owns agent execution details:

- Detailed turn execution.
- Streaming turn execution.
- Fallback from streaming to detailed execution.
- Usage extraction.
- Tool call counting.

Current code candidates: `runDetailedTurn`, `runStreamingTurn`, `tokenUsageFromMetadata`, `meaningfulUsage`, `safeToolName`, and `resetTimer`.

This service may be left partially inside `TurnRunner` at first if extracting it creates too much churn.

## Design Constraints

- Keep `SessionManager` as the facade.
- Keep lifecycle services internal initially.
- Design hook interfaces with future externalization in mind.
- Prefer small optional interfaces to a giant service interface.
- Keep service execution ordered and synchronous.
- Avoid a vague event bus.
- Preserve current behavior before broadening the design.
- Do not force gateway, chat, or commands to know about the new internals.
- Keep tests focused around existing behavior while adding unit tests for extracted components.

## Success Criteria

- `SessionManager` remains the facade used by gateway, chat, and commands.
- Session registry concerns move out of `session.go`.
- Turn processing becomes an explicit pipeline with a shared `Turn` object.
- Transcription, media expansion, progress reporting, summarization, outbound publishing, and logging have clear service boundaries.
- Existing behavior remains intact.
- Existing tests continue to pass.
- The design makes adding future lifecycle behavior additive rather than requiring edits to a monolithic `handleInbound`.

## Open Planning Topics

- Exact hook names and ordering.
- Whether `Turn` should carry both original and normalized inbound messages.
- Whether services should be allowed to short-circuit a turn directly, or only by setting `turn.Err` / `turn.Response`.
- Whether outbound publishing should be a hook or a fixed final stage.
- How much of streaming should be extracted in the first implementation pass.
- Whether skill activation belongs in a service, registry-adjacent helper, or separate session capability.
