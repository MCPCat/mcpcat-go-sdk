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
  <a href="https://go.dev/"><img src="https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go" alt="Go Version"></a>
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


## Getting Started

To get started with MCPcat, first create an account and obtain your project ID by signing up at [mcpcat.io](https://mcpcat.io). For detailed setup instructions visit our [documentation](https://docs.mcpcat.io).

Once you have your project ID, integrate MCPcat into your MCP server:

```go
package main

import (
    "context"
    "fmt"

    "github.com/mark3labs/mcp-go/mcp"
    "github.com/mark3labs/mcp-go/server"
    mcpcat "github.com/mcpcat/mcpcat-go-sdk"
)

func main() {
    // Optional but recommended: ensures graceful shutdown on normal exit
    defer mcpcat.Shutdown()

    hooks := &server.Hooks{}
    s := server.NewMCPServer(
        "Demo Server",
        "1.0.0",
        server.WithToolCapabilities(false),
        server.WithHooks(hooks),
    )

    // Track your MCP server with MCPcat
    projectID := "proj_YOUR_PROJECT_ID"
    _, err := mcpcat.SetupTracking(s, hooks, &projectID, nil)
    if err != nil {
        fmt.Printf("Failed to setup tracking: %v\n", err)
        return
    }

    // Add your tools and start the server
    tool := mcp.NewTool("hello_world",
        mcp.WithDescription("Say hello to someone"),
        mcp.WithString("name",
            mcp.Required(),
            mcp.Description("Name of the person to greet"),
        ),
    )

    s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        name, _ := request.RequireString("name")
        return mcp.NewToolResultText(fmt.Sprintf("Hello, %s!", name)), nil
    })

    if err := server.ServeStdio(s); err != nil {
        fmt.Printf("Server error: %v\n", err)
    }
}
```

Get your project ID by signing up at [mcpcat.io](https://mcpcat.io).

## Advanced Features

### User Identification

You can identify your user sessions with a simple callback MCPcat exposes, called `Identify`.

```go
func identifyUser(ctx context.Context, request any) *mcpcat.UserIdentity {
    // Extract user info from your authentication system
    return &mcpcat.UserIdentity{
        UserID:   "user_12345",
        UserName: "demo_user",
        UserData: map[string]any{
            "email":        "demo@example.com",
            "role":         "developer",
            "organization": "Example Corp",
        },
    }
}

options := mcpcat.DefaultOptions()
options.Identify = identifyUser
_, err := mcpcat.SetupTracking(s, hooks, &projectID, &options)
```

### Sensitive Data Redaction

MCPcat redacts all data sent to its servers and encrypts at rest, but for additional security, it offers a hook to do your own redaction on all text data returned back to our servers.

```go
import "regexp"

func redactSensitiveData(text string) string {
    // Redact email addresses
    emailRegex := regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`)
    text = emailRegex.ReplaceAllString(text, "[REDACTED_EMAIL]")

    // Redact credit card numbers
    ccRegex := regexp.MustCompile(`\b\d{4}[\s-]?\d{4}[\s-]?\d{4}[\s-]?\d{4}\b`)
    text = ccRegex.ReplaceAllString(text, "[REDACTED_CC]")

    // Redact API keys
    apiKeyRegex := regexp.MustCompile(`\b[A-Za-z0-9]{32,}\b`)
    text = apiKeyRegex.ReplaceAllString(text, "[REDACTED_KEY]")

    return text
}

options := mcpcat.DefaultOptions()
options.RedactSensitiveInformation = redactSensitiveData
_, err := mcpcat.SetupTracking(s, hooks, &projectID, &options)
```

### Debug Mode

Enable debug logging for troubleshooting:

```go
options := mcpcat.DefaultOptions()
options.Debug = true
_, err := mcpcat.SetupTracking(s, hooks, &projectID, &options)
```

Debug logs are written to `~/mcpcat.log`.

### Graceful Shutdown

Ensure all queued events are published before your application exits:

```go
func main() {
    defer mcpcat.Shutdown()

    // Your server setup and execution...
}
```

## Configuration Options

```go
type Options struct {
    // Enable debug logging to ~/mcpcat.log
    Debug bool

    // Identify function to attach user information to sessions
    Identify func(ctx context.Context, request any) *UserIdentity

    // Redact function to sanitize sensitive data before sending
    RedactSensitiveInformation func(text string) string

    // Future: Exporters for telemetry (OpenTelemetry, Datadog, Sentry)
    // Exporters []ExporterConfig
}
```
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
