<div align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://mcpcat.io/mcpcat-dark.svg">
    <source media="(prefers-color-scheme: light)" srcset="https://mcpcat.io/mcpcat-light.svg">
    <img alt="MCPcat logo" src="https://mcpcat.io/mcpcat-light.svg" width="400">
  </picture>
</div>

# MCPcat Go SDK

**MCPcat** is an analytics platform for MCP server owners. MCPcat helps developers and product owners build, improve, and monitor their MCP servers by capturing user analytics and tracing tool calls.

## Features

### 🎬 User Session Replay
Replay user sessions to understand how users interact with your MCP server and identify patterns in tool usage.

### 🔍 Trace Debugging
Debug issues by tracing tool calls and analyzing execution paths with detailed telemetry data.

### 📊 Telemetry Support
Forward telemetry to your existing observability platforms including OpenTelemetry, Datadog, and Sentry.

## Installation

```bash
go get github.com/mcpcat/mcpcat-go-sdk
```

## Quick Start

Add MCPcat tracking to your MCP server with just a few lines:

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

Attach user information to sessions for better analytics:

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

Automatically redact sensitive information before sending to MCPcat:

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

## Open Source Support

MCPcat offers free analytics for qualified open source projects. Apply by emailing [hi@mcpcat.io](mailto:hi@mcpcat.io).

## Community

Share your cat photos and join the MCPcat community!

---

<div align="center">
  <p>
    <a href="https://mcpcat.io">Website</a> •
    <a href="https://mcpcat.io/demo">Schedule a Demo</a> •
    <a href="https://docs.mcpcat.io">Documentation</a>
  </p>
</div>
