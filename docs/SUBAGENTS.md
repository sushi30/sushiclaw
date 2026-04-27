# Subagents

Subagents allow the main agent to delegate work to specialized background agents. Each subagent can have its own model, system prompt, and tool whitelist.

## How it works

1. The user asks the main agent to perform a task
2. The main agent can call the `subagent_task` tool to spawn a background subagent
3. The subagent runs asynchronously and its result is sent back to the chat when complete
4. Subagents cannot recursively spawn more subagents (the `subagent_task` tool is filtered out from their tool set)

## Configuration

### Enable the tool

Add `subagent_task` to your `tools` section in `config.json`:

```json
"tools": {
  "subagent_task": {
    "enabled": true
  }
}
```

### Define subagent profiles in the workspace

Subagents are discovered from files in your workspace under `agents/<name>/AGENT.md`.

Example workspace layout:

```
~/.picoclaw/workspace/
  AGENT.md
  SOUL.md
  USER.md
  skills/
    github/SKILL.md
    ...
  agents/              <-- subagent profiles
    coder/AGENT.md
    researcher/AGENT.md
```

### Agent file format

Each `AGENT.md` follows the same frontmatter + body pattern as the main `AGENT.md` and skill `SKILL.md` files:

```yaml
---
description: A coding assistant
model_name: claude-sonnet
tools: read_file, write_file, exec
---

You are a coding assistant. Write clean, well-documented code.
```

| Frontmatter field | Required | Description |
|-------------------|----------|-------------|
| `description` | No | Shown to the main agent to help it choose the right subagent |
| `model_name` | No | Which model from `model_list` to use. Falls back to the default model if omitted |
| `tools` | No | Comma-separated whitelist of tool names. If omitted, the subagent inherits all parent tools |

The **body** (after the frontmatter) becomes the subagent's system prompt.

### Config fallback (optional)

You can also define subagents directly in `config.json` under a `subagents` key. This is useful for programmatic deployments or when you want to keep agent metadata in config rather than files.

```json
"subagents": {
  "coder": {
    "description": "A coding assistant",
    "model_name": "claude-sonnet",
    "system_prompt": "You are a coding assistant."
  }
}
```

**Precedence:** Workspace profiles win over config profiles when both define the same agent name.

## Usage

### Gateway mode (Telegram, WhatsApp, Email)

Subagents work automatically. When a user asks for something like:

> "Research the latest Go 1.24 features and summarize them for me"

The main agent can spawn a `researcher` subagent to do the work in the background. The user gets an immediate `Task N started.` confirmation, and the result arrives later as a new message.

### Chat mode (terminal REPL)

The chat REPL also supports subagents. Results from async tasks are printed to the terminal when they complete:

```
> spawn a coder agent to write a fibonacci function in Go
Task 1 started.
> 
[async result]
Task 1 completed:
Here's a fibonacci function in Go...
```

## Per-subagent tool filtering

Use the `tools` frontmatter field to restrict what a subagent can do:

```yaml
---
description: A web research assistant
model_name: gpt-4o-mini
tools: web_search
---

You are a research assistant. Search the web and summarize findings concisely.
```

This `researcher` agent can only use `web_search` — it cannot read files, execute commands, or access any other tools.

## Listing subagents

Use the `/subagents` command in any channel to see configured subagent profiles:

```
Configured subagents:
• coder
• researcher
```

## Architecture

```
User Message
    |
    v
Main Agent
    |  \
    |   subagent_task(tool_call)
    |          |
    v          v
Reply    TaskExecutor (background)
              |
              v
          Subagent Agent
              |
              v
          Result Published
              |
              v
          MessageBus Outbound
              |
              v
          User gets result
```

## Security notes

- Subagents inherit the main agent's tools **minus** `subagent_task` (recursion is blocked)
- Tool whitelists are applied **before** the `subagent_task` removal
- The `exec` tool respects the same `restrict_to_workspace` and `allow_from` settings as the main agent
- Each subagent runs with its own in-memory context — no shared state with the main agent

## Future improvements

- Task status and cancellation
- Task history and persistence
- Hot-reload of agent profiles without restart
