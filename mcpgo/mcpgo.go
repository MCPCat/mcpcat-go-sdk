// Package mcpgo provides MCPCat tracking integration for mark3labs/mcp-go servers.
//
// It wraps an MCPServer with hooks that automatically capture tool calls,
// resource reads, and other MCP protocol events and publishes them to MCPCat.
package mcpgo

import (
	"context"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	mcpcat "github.com/mcpcat/mcpcat-go-sdk"
)

// Re-export core types so users don't need to import the core module directly.
type (
	UserIdentity   = mcpcat.UserIdentity
	MCPcatInstance = mcpcat.MCPcatInstance
)

// Options configures MCPCat tracking for mark3labs/mcp-go servers.
type Options struct {
	// Hooks provides pre-existing server hooks to append MCPCat's hooks to.
	// If nil, a new Hooks struct is created.
	Hooks *server.Hooks

	// DisableReportMissing, when true, prevents the automatic "get_more_tools"
	// tool from being registered. By default (false) the tool is added.
	DisableReportMissing bool

	// DisableToolCallContext, when true, prevents the "context" parameter from
	// being injected into existing tools. By default (false) it is added.
	DisableToolCallContext bool

	// Debug enables debug logging to ~/mcpcat.log. When false, no logging occurs.
	Debug bool

	// Identify is called once per session to identify the actor.
	// It receives the context and the CallToolRequest that triggered the identification.
	// Return nil to skip identification for this session.
	Identify func(ctx context.Context, request *mcp.CallToolRequest) *UserIdentity

	// RedactSensitiveInformation redacts sensitive data before sending to MCPCat.
	RedactSensitiveInformation func(text string) string

	// APIBaseURL overrides the default MCPCat API endpoint.
	// When empty, the SDK falls back to the MCPCAT_API_URL environment variable,
	// and then to the built-in default (https://api.mcpcat.io).
	APIBaseURL string
}

// DefaultOptions returns a new Options with sensible defaults.
// All features are enabled by default (Disable* fields are false).
func DefaultOptions() *Options {
	return &Options{}
}

// Track attaches MCPCat tracking hooks to the given MCPServer.
// It registers the server in the global registry, initializes the event
// publisher, and wires up hooks for request timing, event capture, context
// parameter injection, and the optional get_more_tools tool.
//
// On success it returns a shutdown function that flushes pending events and
// releases resources. The shutdown function is idempotent and safe to call
// multiple times. On error it returns (nil, err).
func Track(mcpServer *server.MCPServer, projectID string, opts *Options) (func(context.Context) error, error) {
	if mcpServer == nil {
		return nil, mcpcat.ErrNilServer
	}
	if projectID == "" {
		return nil, mcpcat.ErrEmptyProjectID
	}
	if opts == nil {
		opts = DefaultOptions()
	}

	hooks := &server.Hooks{}
	if opts.Hooks != nil {
		hooks = opts.Hooks
	}
	server.WithHooks(hooks)(mcpServer)

	apiBaseURL := mcpcat.ResolveAPIBaseURL(opts.APIBaseURL)

	coreOpts := &mcpcat.Options{
		DisableReportMissing:       opts.DisableReportMissing,
		DisableToolCallContext:     opts.DisableToolCallContext,
		Debug:                      opts.Debug,
		RedactSensitiveInformation: opts.RedactSensitiveInformation,
		APIBaseURL:                 apiBaseURL,
	}

	instance := &mcpcat.MCPcatInstance{
		ProjectID: projectID,
		Options:   coreOpts,
		ServerRef: mcpServer,
	}
	mcpcat.RegisterServer(mcpServer, instance)
	mcpcat.SetDebug(opts.Debug)

	publishFn := mcpcat.InitPublisher(opts.RedactSensitiveInformation, apiBaseURL)

	sessionMap := addTracingToHooks(hooks, opts, publishFn)
	registerGetMoreToolsIfEnabled(mcpServer, coreOpts)

	var once sync.Once
	shutdownFn := func(ctx context.Context) error {
		var err error
		once.Do(func() {
			sessionMap.Stop()
			err = mcpcat.Shutdown(ctx)
		})
		return err
	}

	return shutdownFn, nil
}

// getMCPcat retrieves the MCPcatInstance associated with the given MCPServer.
// Returns nil if the server has not been registered via Track.
func getMCPcat(mcpServer *server.MCPServer) *mcpcat.MCPcatInstance {
	return mcpcat.GetInstance(mcpServer)
}

// unregisterServer removes the MCPServer from the global tracking registry.
func unregisterServer(mcpServer *server.MCPServer) {
	mcpcat.UnregisterServer(mcpServer)
}

// Shutdown gracefully shuts down the global event publisher.
// The provided context controls the shutdown deadline.
func Shutdown(ctx context.Context) error {
	return mcpcat.Shutdown(ctx)
}
