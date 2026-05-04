# Agent-to-Agent Capabilities

Living brainstorm and delivery tracker for A2A support in sushiclaw.

## Goal

Let Alice's sushiclaw agent coordinate directly with Bob's agent for practical collaboration:

- Scheduling and availability exchange
- Coordinating plans, errands, reminders, and commitments
- Exchanging ideas, drafts, constraints, and preferences
- Negotiating next steps without forcing users to manually relay every message

The core product requirement is transparency: users should understand when their agent is talking to
another agent, what was shared, what was agreed, and what still needs approval.

## Product Principles

- User agency first: an agent may propose, draft, and negotiate, but user-impacting commitments need
  explicit approval unless the user has granted a narrow standing permission.
- Transparent by default: A2A activity should produce a concise user-visible summary, not hidden
  background behavior.
- Minimal disclosure: share only the context required for the remote agent to help.
- Revocable trust: users can disable a remote agent, revoke credentials, or clear a relationship.
- Protocol at the edge: keep the internal agent/session architecture stable and adapt A2A at the
  boundary.

## Current Repo Fit

Useful existing seams:

- `pkg/bus` already normalizes inbound and outbound messages.
- `internal/agent.SessionManager` already isolates conversations with `SessionKey`.
- `pkg/channels/websocket` and `pkg/channels/websocket_client` already prove bidirectional agent-like
  communication over a network transport.
- `pkg/tools` already supports gateway-only tools, which is the right place for outbound delegation.
- `pkg/commands` already reserves `/subagents`, but it is not yet a complete user-facing management
  surface.

Likely direction:

- Add an A2A adapter around the existing bus/session layer rather than building a second agent
  runtime.
- Treat inbound A2A requests as a channel-like source.
- Treat outbound A2A delegation as a tool the local agent can call.

## Protocol Baseline

The A2A protocol concepts to align with:

- Agent discovery through `/.well-known/agent-card.json`
- Agent cards that describe identity, capabilities, skills, supported interfaces, and auth schemes
- Message/task endpoints, including text messages and task state
- Protocol bindings such as `HTTP+JSON`, `JSONRPC`, or `GRPC`
- Optional streaming and push notifications

Seed implementation should prefer a narrow text-only subset before adding richer task lifecycle,
streaming, media, or artifacts.

## Phased Delivery

### Phase 0: Product Shape and Safety Model

Status: In progress

Define the user experience and safety envelope before implementing transport.

Deliverables:

- Define what users see when their agent contacts another agent.
- Define what requires explicit approval.
- Define what can happen under standing permission.
- Define what data is allowed to leave the local agent.
- Define audit log requirements.

Open decisions:

- Should first contact with a new remote agent always require approval?
- Should every outbound A2A message be visible to the user, or only summarized?
- What is the minimum useful audit log: raw transcript, summaries, metadata, or all three?

### Phase 1: Agent Card and Inbound Text Endpoint

Status: Proposed

Make sushiclaw callable by another A2A-aware agent.

Deliverables:

- Config section for local A2A identity and public endpoint metadata.
- `GET /.well-known/agent-card.json` endpoint.
- Authenticated text-only inbound message endpoint.
- Map inbound A2A calls into `bus.InboundMessage`.
- Stable session key format, for example `a2a:<remote-agent-id>:<task-id>`.
- Tests for auth, card generation, request parsing, and session routing.

Non-goals:

- No remote discovery crawling.
- No artifacts/media.
- No streaming unless trivial through existing infrastructure.

### Phase 2: Outbound A2A Tool

Status: Proposed

Let sushiclaw call known remote agents.

Deliverables:

- Config section for trusted remote agents.
- `a2a_send` tool for text-only delegation.
- Remote agent card fetch and validation.
- Per-remote-agent auth configuration.
- Timeout and retry behavior.
- Tests for tool arguments, auth headers, error handling, and response parsing.

Example tool intent:

```text
Ask Bob's agent whether Bob is free Tuesday afternoon and whether he prefers coffee or a call.
```

### Phase 3: Transparent User Workflow

Status: Proposed

Make A2A behavior understandable and controllable from user channels.

Deliverables:

- User-visible "I am contacting Bob's agent" progress messages.
- Final summaries that separate facts, proposals, agreements, and pending approvals.
- Approval prompts for commitments.
- `/subagents` or `/a2a` commands to list, inspect, enable, disable, and revoke remote agents.
- Conversation transcript or audit log storage.

Success criteria:

- Alice can ask: "Coordinate lunch with Bob next week."
- Alice's agent contacts Bob's agent.
- Bob's agent checks or asks Bob according to its own rules.
- Alice sees what was asked, what Bob's agent replied, and what requires Alice's approval.

### Phase 4: Scheduling Capability

Status: Proposed

Turn generic A2A messaging into a useful coordination feature.

Deliverables:

- Structured availability exchange.
- Proposed meeting windows.
- Conflict and preference handling.
- Calendar integration boundary: draft events first, commit only after approval.
- User preference handling for working hours, travel time, meeting types, and default locations.

Non-goals:

- Do not auto-book calendar events without explicit user permission.
- Do not expose full calendar details when busy/free blocks are sufficient.

### Phase 5: Richer Collaboration

Status: Future

Expand beyond text coordination.

Candidate capabilities:

- Streaming updates for long-running negotiations.
- Artifacts: documents, images, calendar invites, structured proposals.
- Group coordination among more than two agents.
- Capability negotiation based on agent cards.
- Signed agent cards or stronger trust verification.
- Relationship memory: "Alice trusts Bob's agent for scheduling, but not purchases."

## Data Model Sketch

Potential config shape:

```json
{
  "a2a": {
    "enabled": true,
    "public_base_url": "https://alice-agent.example.com",
    "agent_id": "alice-agent",
    "display_name": "Alice's Agent",
    "token": "env://A2A_TOKEN",
    "trusted_agents": {
      "bob": {
        "agent_card_url": "https://bob-agent.example.com/.well-known/agent-card.json",
        "token": "env://BOB_A2A_TOKEN",
        "allow_outbound": true,
        "allow_inbound": true
      }
    }
  }
}
```

Potential session key format:

```text
a2a:<remote-agent-id>:<task-id>
```

Potential audit event types:

- `a2a.agent_card_fetched`
- `a2a.outbound_message_sent`
- `a2a.inbound_message_received`
- `a2a.remote_response_received`
- `a2a.user_approval_requested`
- `a2a.user_approval_granted`
- `a2a.user_approval_denied`

## MVP Scenario

Alice says:

```text
Ask Bob if he wants to discuss the house project this weekend. Find a time that works for both of us.
```

Expected behavior:

1. Alice's agent identifies Bob as a trusted A2A contact.
2. Alice's agent asks for approval to contact Bob's agent if no standing permission exists.
3. Alice's agent sends a concise request to Bob's agent.
4. Bob's agent handles Bob-side policy and context.
5. Alice's agent receives Bob-side availability or a counterproposal.
6. Alice sees a summary and approves any commitment before calendar changes are made.

## Working Success Criteria

- A remote agent can discover this agent's card.
- A remote agent can send a text request and receive a useful text response.
- This agent can send a text request to a configured remote agent.
- Users get visible summaries of A2A activity.
- No calendar event, message to a human, or durable commitment is made without permission.

## Open Questions

- What should the user command be: `/subagents`, `/a2a`, or both?
- Should A2A be modeled internally as a channel, a tool, or a small service with both adapters?
- Where should audit history live: memory files, logs, a JSON store, or a future database?
- What is the first real transport binding to implement: `HTTP+JSON` or `JSONRPC`?
- How much of the official task lifecycle should the first version support?
