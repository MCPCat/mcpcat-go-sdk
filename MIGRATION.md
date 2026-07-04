# Migrating from `github.com/mcpcat/mcpcat-go-sdk` to `go.agentcat.com/sdk`

MCPcat is now AgentCat — same team, same product, new name. This repository is
**frozen**: it receives no new features or dependency updates. All new
development happens in [`go.agentcat.com/sdk`](https://github.com/agentcathq/agentcat-go-sdk).

## Nothing breaks if you stay

You do not have to migrate on any timeline:

- The old module stays published and is **not retracted**.
- Existing version pins — `v0.1.x` through `v0.3.x`, `mcpgo/v0.3.0`, and
  `officialsdk/v0.3.0` — keep resolving via the Go module proxy and direct
  fetch, forever.
- Runtime behavior is unchanged: the `MCPCAT_API_URL` environment variable,
  the default endpoint `api.mcpcat.io`, and the `~/mcpcat.log` debug log all
  work exactly as before.

## What changed

| Before | After |
| --- | --- |
| `github.com/mcpcat/mcpcat-go-sdk` (module path) | `go.agentcat.com/sdk` |
| `mcpcat.` (package name in code) | `agentcat.` |
| `github.com/mcpcat/mcpcat-go-sdk/mcpgo` | `go.agentcat.com/sdk/mcpgo` |
| `github.com/mcpcat/mcpcat-go-sdk/officialsdk` | `go.agentcat.com/sdk/officialsdk` |
| `github.com/mcpcat/mcpcat-go-api` (generated client) | `go.agentcat.com/api` |

## How to migrate

1. Get the new module (pick the submodule matching your MCP library):

   ```bash
   go get go.agentcat.com/sdk/mcpgo        # mark3labs/mcp-go users
   go get go.agentcat.com/sdk/officialsdk  # modelcontextprotocol/go-sdk users
   ```

2. Update your imports:

   ```diff
   -import "github.com/mcpcat/mcpcat-go-sdk/mcpgo"
   +import "go.agentcat.com/sdk/mcpgo"
   ```

   and update package references from `mcpcat.` to `agentcat.` where you use
   the root package.

3. Tidy:

   ```bash
   go mod tidy
   ```

## Using a coding agent?

Paste this prompt into your coding agent to do the migration for you:

```text
Migrate this Go project from the frozen module github.com/mcpcat/mcpcat-go-sdk
to its successor go.agentcat.com/sdk:

1. Run `go get go.agentcat.com/sdk/mcpgo` or
   `go get go.agentcat.com/sdk/officialsdk` depending on which submodule the
   project currently imports (check go.mod / imports first).
2. Rewrite all imports: github.com/mcpcat/mcpcat-go-sdk -> go.agentcat.com/sdk
   (including the /mcpgo and /officialsdk submodule paths), and any reference
   to github.com/mcpcat/mcpcat-go-api -> go.agentcat.com/api.
3. Rename package qualifiers from mcpcat. to agentcat. where the root package
   is used.
4. Run `go mod tidy`, then `go build ./...` and the test suite to confirm
   everything compiles and passes.
```

## Questions?

Open an issue on
[agentcathq/agentcat-go-sdk](https://github.com/agentcathq/agentcat-go-sdk/issues).
