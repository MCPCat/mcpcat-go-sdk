<div align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/static/logo-dark.svg">
    <source media="(prefers-color-scheme: light)" srcset="docs/static/logo-light.svg">
    <img alt="MCPcat Logo" src="docs/static/logo-light.svg" width="80%">
  </picture>
</div>
<h3 align="center">
    <a href="#getting-started">Getting Started</a>
    <span> · </span>
    <a href="#why-use-mcpcat-">Features</a>
    <span> · </span>
    <a href="https://docs.mcpcat.io">Docs</a>
    <span> · </span>
    <a href="https://mcpcat.io">Website</a>
    <span> · </span>
    <a href="#free-for-open-source">Open Source</a>
    <span> · </span>
    <a href="https://meet.mcpcat.io/meet">Schedule a Demo</a>
</h3>
<p align="center">
  <a href="https://pkg.go.dev/github.com/mcpcat/mcpcat-go-sdk"><img src="https://pkg.go.dev/badge/github.com/mcpcat/mcpcat-go-sdk.svg" alt="Go Reference"></a>
  <a href="https://goreportcard.com/report/github.com/mcpcat/mcpcat-go-sdk"><img src="https://goreportcard.com/badge/github.com/mcpcat/mcpcat-go-sdk" alt="Go Report Card"></a>
  <a href="https://opensource.org/licenses/MIT"><img src="https://img.shields.io/badge/License-MIT-blue.svg" alt="License: MIT"></a>
  <a href="https://go.dev/"><img src="https://img.shields.io/badge/Go-1.23+-00ADD8?logo=go" alt="Go Version"></a>
  <a href="https://github.com/mcpcat/mcpcat-go-sdk/issues"><img src="https://img.shields.io/github/issues/mcpcat/mcpcat-go-sdk.svg" alt="GitHub issues"></a>
  <a href="https://github.com/mcpcat/mcpcat-go-sdk/actions"><img src="https://github.com/mcpcat/mcpcat-go-sdk/workflows/CI/badge.svg" alt="CI"></a>
</p>

> [!NOTE]
> Looking for the Python SDK? Check it out here [mcpcat-python](https://github.com/mcpcat/mcpcat-python-sdk).

## Why use MCPcat? 🤔

MCPcat helps developers and product owners build, improve, and monitor their MCP servers by capturing user analytics and tracing tool calls.

Use MCPcat for:

- **User session replay** 🎬. Follow alongside your users to understand why they're using your MCP servers, what functionality you're missing, and what clients they're coming from.
- **Trace debugging** 🔍. See where your users are getting stuck, track and find when LLMs get confused by your API, and debug sessions across all deployments of your MCP server.
- **Existing platform support** 📊. Get logging and tracing out of the box for your existing observability platforms (OpenTelemetry, Datadog, Sentry) — eliminating the tedious work of implementing telemetry yourself.


<img width="1274" height="770" alt="mcpcat-diagram" src="https://github.com/user-attachments/assets/2d75de19-5b69-4f8b-aea9-43161de5a2ba" />

## Supported MCP Libraries

MCPcat provides first-class support for the two most popular Go MCP libraries:

| Library | Install |
|---------|---------|
| [mcp-go](https://github.com/mark3labs/mcp-go) (mark3labs) | `go get github.com/mcpcat/mcpcat-go-sdk/mcpgo` |
| [go-sdk](https://github.com/modelcontextprotocol/go-sdk) (official) | `go get github.com/mcpcat/mcpcat-go-sdk/officialsdk` |

Import the package that matches the MCP library you're already using. Both expose the same `Track()` API and share the same feature set.

## Getting Started

Create an account and obtain your project ID at [mcpcat.io](https://mcpcat.io). For detailed setup instructions visit our [documentation](https://docs.mcpcat.io).

Add one `Track()` call before starting your server:

**mark3labs/mcp-go:**
```go
import mcpcat "github.com/mcpcat/mcpcat-go-sdk/mcpgo"

shutdown, err := mcpcat.Track(mcpServer, "proj_YOUR_PROJECT_ID", nil)
if err != nil { /* handle error */ }
defer shutdown(context.Background())
```

**Official go-sdk:**
```go
import mcpcat "github.com/mcpcat/mcpcat-go-sdk/officialsdk"

shutdown, err := mcpcat.Track(mcpServer, "proj_YOUR_PROJECT_ID", nil)
if err != nil { /* handle error */ }
defer shutdown(context.Background())
```

`Track()` returns a shutdown function — call it before your application exits to flush all queued events.

## Advanced Features

### User Identification

Identify your user sessions with a callback to attach user information to every event in a session.

**mark3labs/mcp-go:**
```go
import (
    "github.com/mark3labs/mcp-go/mcp"
    mcpcat "github.com/mcpcat/mcpcat-go-sdk/mcpgo"
)

shutdown, err := mcpcat.Track(s, "proj_YOUR_PROJECT_ID", &mcpcat.Options{
    Identify: func(ctx context.Context, req *mcp.CallToolRequest) *mcpcat.UserIdentity {
        return &mcpcat.UserIdentity{
            UserID: "user_12345", UserName: "demo_user",
            UserData: map[string]any{"email": "demo@example.com"},
        }
    },
})
```

**Official go-sdk:**
```go
import (
    "github.com/modelcontextprotocol/go-sdk/mcp"
    mcpcat "github.com/mcpcat/mcpcat-go-sdk/officialsdk"
)

shutdown, err := mcpcat.Track(s, "proj_YOUR_PROJECT_ID", &mcpcat.Options{
    Identify: func(ctx context.Context, req *mcp.CallToolRequest) *mcpcat.UserIdentity {
        return &mcpcat.UserIdentity{
            UserID: "user_12345", UserName: "demo_user",
            UserData: map[string]any{"email": "demo@example.com"},
        }
    },
})
```

### Sensitive Data Redaction

MCPcat redacts all data sent to its servers and encrypts at rest, but for additional security, it offers a hook to do your own redaction on all text data before it leaves your server.

```go
shutdown, err := mcpcat.Track(s, "proj_YOUR_PROJECT_ID", &mcpcat.Options{
    RedactSensitiveInformation: func(text string) string {
        return emailRegex.ReplaceAllString(text, "[REDACTED]")
    },
})
```

### Debug Mode

Enable debug logging for troubleshooting. Debug logs are written to `~/mcpcat.log`.

```go
shutdown, err := mcpcat.Track(s, "proj_YOUR_PROJECT_ID", &mcpcat.Options{Debug: true})
```

### Using with Existing Hooks (mcp-go only)

If your server already uses mcp-go hooks, pass them via `Options.Hooks` and MCPCat will append its hooks alongside yours:

```go
shutdown, err := mcpcat.Track(s, "proj_YOUR_PROJECT_ID", &mcpcat.Options{Hooks: hooks})
```

## Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `DisableReportMissing` | `bool` | `false` | When `true`, prevents the `get_more_tools` tool from being registered |
| `DisableToolCallContext` | `bool` | `false` | When `true`, prevents the `context` parameter from being injected on tool calls |
| `Debug` | `bool` | `false` | Enable debug logging to `~/mcpcat.log` |
| `RedactSensitiveInformation` | `func(string) string` | `nil` | Custom redaction applied to all text data before sending |
| `Identify` | callback | `nil` | Attach user information to sessions |
| `Hooks` | `*server.Hooks` | `nil` | Pre-existing hooks to merge with (mcp-go only) |

## Free for open source

MCPcat is free for qualified open source projects. We believe in supporting the ecosystem that makes MCP possible. If you maintain an open source MCP server, you can access our full analytics platform at no cost.

**How to apply**: Email hi@mcpcat.io with your repository link

_Already using MCPcat? We'll upgrade your account immediately._

## Community Cats 🐱

Meet the cats behind MCPcat! Add your cat to our community by submitting a PR with your cat's photo in the `docs/cats/` directory.

<div align="left">
  <img src="docs/cats/bibi.png" alt="bibi" width="80" height="80">
  <img src="docs/cats/zelda.jpg" alt="zelda" width="80" height="80">
</div>

_Want to add your cat? Create a PR adding your cat's photo to `docs/cats/` and update this section!_
