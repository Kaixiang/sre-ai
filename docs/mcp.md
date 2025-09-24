# MCP Integration Guide

The `sre-ai mcp` subcommands manage local Model Context Protocol (MCP) servers that expose tooling to the CLI and downstream agents. This MVP release focuses on launching local processes (stdio servers) and wiring them into workflows.

## Bundled Node Runtime

Some community MCP servers ship as npm packages (for example, [`firecrawl-mcp`](https://github.com/mendableai/firecrawl-mcp)). The repository includes a portable Node.js runtime under `third_party/node/node-v20.16.0-win-x64`. When you run `sre-ai mcp test`, the CLI automatically prepends this runtime to the `PATH` environment so `node`, `npm`, and `npx` resolve even on machines without Node installed globally.

If you prefer a different Node version or platform, replace the contents of `third_party/node` with the desired distribution. The CLI looks for the first directory containing `node.exe` (or `bin/node` on Unix-like systems) and uses that location.

## Command Overview

| Command | Description |
|---------|-------------|
| `sre-ai mcp ls` | List all configured servers, their source (`embedded`, `config`, or `local`), and launch commands. |
| `sre-ai mcp add <alias=path>` | Parse a definition file and store the server under the provided alias. Updates are idempotent. |
| `sre-ai mcp rm <alias>` | Remove a stored definition. |
| `sre-ai mcp test <alias>` | Launch the server briefly to verify the command, environment, and bundled Node runtime work. |

### Definition File Format

`mcp add` accepts either a single server definition or a JSON file containing the `mcpServers` object (matching the TypeScript SDK conventions):

```json
{
  "mcpServers": {
    "firecrawl-mcp": {
      "command": "npx",
      "args": ["-y", "firecrawl-mcp"],
      "env": {
        "FIRECRAWL_API_KEY": "example-key"
      }
    }
  }
}
```

Save the snippet to `configs/firecrawl.json` and register it:

```powershell
sre-ai mcp add firecrawl=configs/firecrawl.json
sre-ai mcp ls
```

The config is persisted at `~/.config/sre-ai/mcp/servers.json`. You can edit that file manually or re-run `mcp add` to update an entry.

### Testing a Server

`mcp test` starts the configured command with the merged environment (system `PATH`, bundled Node, and custom variables). The CLI kills the process after a short delay—enough to detect missing binaries or misconfigured secrets:

```powershell
sre-ai mcp test firecrawl
```

If the process fails to launch, the command returns an error and prints the captured `stderr` tail for debugging.

### Embedded Servers

The CLI still ships with embedded manifests (`github`, `files`) for quick experiments. These appear in `mcp ls` with the `embedded` source label. Local definitions show `local`, and any manifest paths configured via `config.yaml` appear as `config`.

---
## Using MCP Servers in Workflows

Once an alias is registered, reference it from a workflow tool with `kind: mcp`. The agent runner reuses the stored command, merges any per-step arguments, and exposes the command output back to the workflow so prompts can summarize or post-process the crawl results.

With MCP servers registered, workflows can safely request capabilities (e.g., `--tools firecrawl`) knowing the corresponding process is configured locally.
