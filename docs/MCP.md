# MCP Server Support

Sushiclaw can connect to [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) servers, giving the agent access to external tools such as filesystem access, GitHub operations, databases, and more.

## Overview

MCP configuration lives in your `config.json` under the `mcp` key. When the agent starts, it reads `mcpServers`, connects to each server, and makes the server's tools available to the agent automatically.

## Quick Start

Add an `mcp` section to `~/.picoclaw/config.json`:

```json
{
  "mcp": {
    "mcpServers": {
      "filesystem": {
        "command": "npx",
        "args": ["-y", "@modelcontextprotocol/server-filesystem", "/home/user/workspace"]
      }
    }
  }
}
```

Restart sushiclaw. The agent will discover and use the filesystem tools automatically.

## Configuration Format

### Top-level structure

```json
{
  "mcp": {
    "mcpServers": {
      "<server-name>": { ... },
      "<server-name>": { ... }
    }
  }
}
```

Each entry under `mcpServers` is a named MCP server. The name is arbitrary and used only for logging.

### Server types

#### Stdio servers

Local command-line MCP servers communicate over standard input/output.

```json
{
  "command": "npx",
  "args": ["-y", "@modelcontextprotocol/server-github"],
  "env": {
    "GITHUB_PERSONAL_ACCESS_TOKEN": "env://GITHUB_TOKEN"
  },
  "allowedTools": ["search_issues", "create_issue"]
}
```

| Field | Type | Description |
|-------|------|-------------|
| `command` | string | Executable to run |
| `args` | string[] | Arguments passed to the executable |
| `env` | object | Environment variables (`KEY: VALUE`) |
| `allowedTools` | string[] | *(Optional)* Whitelist tools from this server |

#### HTTP servers

Remote MCP servers exposed over HTTP.

```json
{
  "url": "http://localhost:3000/mcp",
  "token": "env://MCP_REMOTE_TOKEN",
  "allowedTools": ["query_database"]
}
```

| Field | Type | Description |
|-------|------|-------------|
| `url` | string | HTTP(S) endpoint of the MCP server |
| `token` | string | Bearer token for authentication |
| `allowedTools` | string[] | *(Optional)* Whitelist tools from this server |

## Environment Variables

Use the `env://VAR_NAME` syntax for secrets. Sushiclaw resolves these at load time from the process environment.

```json
{
  "token": "env://MCP_TOKEN"
}
```

If the environment variable is not set, the literal string `env://VAR_NAME` is kept as-is and passed to the underlying SDK.

## Tool Filtering

The optional `allowedTools` array restricts which tools from a server are exposed to the agent. If omitted, all tools are available.

```json
{
  "github": {
    "command": "npx",
    "args": ["-y", "@modelcontextprotocol/server-github"],
    "allowedTools": ["search_issues", "get_issue"]
  }
}
```

## How It Works

1. At startup, `BuildAgent` reads `cfg.MCP.MCPServers`.
2. The config is converted to `agent-sdk-go`'s `MCPConfiguration`.
3. `agentsdk.WithMCPConfig(...)` registers the servers with the agent.
4. `agent-sdk-go` connects lazily and discovers available tools.
5. Discovered tools are merged with native tools (e.g., `exec`) and passed to the LLM.

## Example Configurations

See [`examples/config/mcp.json`](../examples/config/mcp.json) for a complete example with stdio and HTTP servers.

## Troubleshooting

**Agent fails to start after adding MCP config**
- Check that the `command` exists in `$PATH`.
- Verify `env://` variables are exported in the environment.
- Look for MCP initialization warnings in the logs.

**Tools from MCP server are not available**
- Ensure the server process starts successfully (stdio servers).
- Verify the HTTP endpoint is reachable.
- Check `allowedTools` is not accidentally filtering out the tool you need.

**Secrets exposed in logs**
- `token` values using `env://` are resolved at load time. The raw config file should never contain plaintext secrets.
