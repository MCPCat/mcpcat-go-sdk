package mcpcat

import (
	"github.com/mark3labs/mcp-go/server"
	"github.com/mcpcat/mcpcat-go-sdk/internal/core"
	"github.com/mcpcat/mcpcat-go-sdk/internal/tracking"
)

// AddTracingToHooks is a public wrapper for the internal tracking function.
// It adds MCPcat tracking to existing server hooks.
// Deprecated: Use SetupTracking instead, which handles both hooks and configuration.
func AddTracingToHooks(hooks *server.Hooks) {
	tracking.AddTracingToHooks(hooks, nil)
}

// AddTracingToHooksWithRedaction adds MCPcat tracking to existing server hooks with
// an optional redaction function for sensitive data.
func AddTracingToHooksWithRedaction(hooks *server.Hooks, redactFn core.RedactFunc) {
	tracking.AddTracingToHooks(hooks, redactFn)
}
