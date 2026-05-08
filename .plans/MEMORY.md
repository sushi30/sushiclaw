# Memory Provider Experiments

## Current State

Sushiclaw should keep its current short-term transcript store in place. The `agent-sdk-go` memory
interface is intentionally small:

- `AddMessage(ctx, message)`
- `GetMessages(ctx, options...)`
- `Clear(ctx)`

The repo already uses `SQLiteSessionMemory` for persisted chronological history, active session
heads, and overflow summary handoffs. That layer is useful for replaying recent conversation state
and should not be replaced directly by a semantic memory product.

The better target architecture is layered:

```text
SQLiteSessionMemory = recent transcript / replay / summary handoff
MemoryProvider     = durable semantic recall / learned preferences / facts
```

## Replacement Shape

Introduce a separate long-term memory provider boundary instead of forcing external memory systems
to implement `interfaces.Memory`.

Possible local interface:

```go
type LongTermMemoryProvider interface {
	Recall(ctx context.Context, req RecallRequest) ([]MemoryHit, error)
	RetainTurn(ctx context.Context, req RetainTurnRequest) error
	Clear(ctx context.Context, scope MemoryScope) error
}
```

`SQLiteSessionMemory` remains responsible for exact turn history. The long-term provider receives
completed turns and returns relevant durable context before a new turn runs.

Suggested runtime flow:

1. Inbound message resolves to stable session key.
2. Recall long-term memories using the current user input and session/user metadata.
3. Inject the top relevant memories into the agent context as a bounded "Relevant long-term memory"
   block.
4. Run the existing agent with `SQLiteSessionMemory`.
5. After successful completion, retain the turn in the configured provider.
6. `/clear` clears the transcript and should optionally clear long-term memory for the same scope.

## Candidate Assessment

### Hindsight

Repository: <https://github.com/vectorize-io/hindsight>

Best first experiment. Hindsight is explicitly designed for agents that learn from interactions and
provides primitives like retain, recall, and reflect. Its bank-based model maps cleanly to a stable
session key, user identity, or channel-specific memory scope.

Why it fits:

- Good conceptual match for durable agent recall.
- HTTP/service boundary should be easy to wrap from Go.
- Supports a provider-style implementation without disturbing the current transcript database.

Risks to test:

- Operational burden of running the service.
- Recall quality under short, chatty personal-assistant turns.
- How well corrections and outdated facts are handled.

### Mem0

Repository: <https://github.com/mem0ai/mem0>

Best second experiment. Mem0 is mature and broadly used as a memory layer for AI assistants. Current
docs show a common pattern of adding memory-search and memory-save tools, or using memory as an
automatic extraction and retrieval layer. Context7 resolved the official project as `/mem0ai/mem0`.

Why it fits:

- Mature ecosystem and documentation.
- Supports hosted and self-hosted-style memory workflows.
- The search/add pattern is straightforward to expose behind a Go provider or as explicit tools.

Risks to test:

- Python/platform bias may make a pure Go integration less direct.
- Cost and latency if using hosted or LLM-backed extraction.
- Whether automatic memory extraction creates noisy or over-eager memories.

### Memvid

Repository: <https://github.com/memvid/memvid>

Good third experiment if local, portable memory archives matter. It looks better suited for
conversation archives and knowledge retrieval than for adaptive user preference learning.

Why it fits:

- Local-first archival memory is attractive for personal infrastructure.
- Could store summarized historical conversations in portable capsules.
- Useful for "what did we discuss about X?" style recall.

Risks to test:

- May be weaker for live preference learning and correction handling.
- Might fit better as an archive/search backend than as the primary long-term memory.
- Need to validate update costs and retrieval latency over growing history.

### Beads

Repository: <https://github.com/gastownhall/beads>

Do not use Beads as chat memory. It is better treated as project/task memory for coding-agent
workflows: decisions, discovered gaps, dependencies, and implementation constraints.

Why it still matters:

- Useful for tracking durable development context.
- Better fit for plans, issue decomposition, and work history than for conversational recall.
- Could complement sushiclaw as a developer workflow tool, but not replace session memory.

## Recommended Experiment Set

Run two initial experiments, with a third optional follow-up:

1. Hindsight provider.
2. Mem0 provider.
3. Memvid archive provider, only if local/offline portability is a priority.

Beads should be evaluated separately as project/task memory, not as an agent conversation memory
replacement.

## Success Criteria

Evaluate each candidate against the same categories.

### Recall Quality

- Recalls durable user preferences after restart.
- Answers paraphrased follow-up questions using prior memory.
- Retrieves prior outcomes, not just surface text.

Example test:

1. User says: "I prefer vegetarian restaurants and I dislike noisy places."
2. Restart the gateway.
3. User asks: "Where should I book dinner?"
4. Expected: response uses vegetarian and quiet-place preferences without needing the original
   transcript.

### Precision

- Does not inject unrelated memories into unrelated turns.
- Keeps recall block small and relevant.
- Avoids over-personalizing mundane answers.

### Correction Handling

- User corrections should supersede stale memories.
- Example: "Actually, I eat fish now" should stop strict vegetarian memories from dominating future
  restaurant suggestions.

### Isolation

- No cross-user leakage.
- No cross-channel leakage unless explicitly configured.
- Stable session key, sender identity, and channel should be included in metadata.

### Operations

- Easy local startup.
- Clear config shape.
- Understandable backup/export story.
- Does not make gateway startup fragile if the memory service is unavailable.

### Latency And Cost

- Recall should be fast enough to feel invisible in chat.
- Suggested target: under 1 second p50 recall latency.
- Memory extraction should not dominate turn time.
- Provider errors should degrade gracefully to normal SQLite-only behavior.

### Clear Semantics

`/clear` currently clears conversation history. Long-term memory needs explicit semantics:

- Default: clear transcript only.
- Optional command/config: clear transcript plus durable memory for the session/user scope.
- The user-facing behavior should make it clear when durable memory remains.

## Implementation Notes

Add provider configuration under a new memory section, for example:

```json
{
  "memory": {
    "provider": "hindsight",
    "enabled": true,
    "recall_top_k": 5,
    "inject_max_chars": 2000,
    "retain_completed_turns": true,
    "hindsight": {
      "base_url": "http://localhost:8080"
    }
  }
}
```

Keep provider failures non-fatal at first. Log them, emit debug progress if enabled, and continue
with only `SQLiteSessionMemory`.

Avoid retaining tool internals blindly. The first pass should retain:

- User message.
- Final assistant response.
- Channel/session metadata.
- Optional compact summary of the turn.

Do not retain:

- Raw tool call payloads by default.
- Secrets or credentials.
- Large media transcripts without an explicit policy.

## First Implementation Milestone

Build the provider abstraction and one provider behind a feature flag.

Scope:

- Add config types.
- Add `LongTermMemoryProvider` interface.
- Add a no-op provider.
- Add recall injection before `RunDetailed` / `RunStream`.
- Add post-turn retention after successful responses.
- Add tests proving SQLite history still works when the provider is disabled or failing.

The first real provider should be Hindsight unless setup friction is higher than expected. If so,
switch the first implementation to Mem0 and keep the same internal provider boundary.
