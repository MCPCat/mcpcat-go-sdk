// Package officialsdk provides MCPCat tracking integration for the official
// Go MCP SDK (github.com/modelcontextprotocol/go-sdk).
//
// It installs receiving middleware on an mcp.Server that automatically captures
// tool calls, resource reads, prompt requests, and other MCP protocol events
// and publishes them to MCPCat.
package officialsdk

import (
	"context"
	"fmt"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	mcpcat "github.com/mcpcat/mcpcat-go-sdk"
)

// Re-export core types so users don't need to import the core module directly.
type (
	UserIdentity   = mcpcat.UserIdentity
	MCPcatInstance = mcpcat.MCPcatInstance
)

// Options configures MCPCat tracking for the official Go MCP SDK.
type Options struct {
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
}

// DefaultOptions returns a new Options with sensible defaults.
// All features are enabled by default (Disable* fields are false).
func DefaultOptions() *Options {
	return &Options{}
}

// Track attaches MCPCat tracking middleware to the given mcp.Server.
// It registers the server in the global registry, initializes the event
// publisher, and installs receiving middleware for request timing, event
// capture, context parameter injection, and the optional get_more_tools tool.
//
// On success it returns a shutdown function that flushes pending events and
// releases resources. The shutdown function is idempotent and safe to call
// multiple times. On error it returns (nil, err).
func Track(mcpServer *mcp.Server, projectID string, opts *Options) (func(context.Context) error, error) {
	if mcpServer == nil {
		return nil, fmt.Errorf("Track: mcpServer must not be nil")
	}
	if projectID == "" {
		return nil, fmt.Errorf("Track: projectID must not be empty")
	}
	if opts == nil {
		opts = DefaultOptions()
	}

	coreOpts := &mcpcat.Options{
		DisableReportMissing:       opts.DisableReportMissing,
		DisableToolCallContext:     opts.DisableToolCallContext,
		Debug:                      opts.Debug,
		RedactSensitiveInformation: opts.RedactSensitiveInformation,
	}

	instance := &mcpcat.MCPcatInstance{
		ProjectID: projectID,
		Options:   coreOpts,
		ServerRef: mcpServer,
	}
	mcpcat.RegisterServer(mcpServer, instance)
	mcpcat.SetDebug(opts.Debug)

	publishFn := mcpcat.InitPublisher(opts.RedactSensitiveInformation)

	// Retrieve the server implementation for session metadata.
	// We store a copy of the implementation info at Track() time.
	serverImpl := getServerImpl(mcpServer)

	middleware := newTrackingMiddleware(projectID, opts, publishFn, serverImpl)
	mcpServer.AddReceivingMiddleware(middleware)

	registerGetMoreToolsIfEnabled(mcpServer, coreOpts)

	var once sync.Once
	shutdownFn := func(ctx context.Context) error {
		var err error
		once.Do(func() {
			err = mcpcat.Shutdown(ctx)
		})
		return err
	}

	return shutdownFn, nil
}

// getMCPcat retrieves the MCPcatInstance associated with the given mcp.Server.
// Returns nil if the server has not been registered via Track.
func getMCPcat(mcpServer *mcp.Server) *mcpcat.MCPcatInstance {
	return mcpcat.GetInstance(mcpServer)
}

// unregisterServer removes the mcp.Server from the global tracking registry.
func unregisterServer(mcpServer *mcp.Server) {
	mcpcat.UnregisterServer(mcpServer)
}

// Shutdown gracefully shuts down the global event publisher.
// The provided context controls the shutdown deadline.
func Shutdown(ctx context.Context) error {
	return mcpcat.Shutdown(ctx)
}

// getServerImpl extracts the Implementation from the server.
// The official SDK does not directly expose the impl field, so we rely on the
// fact that the server was created with NewServer(impl, opts).
// We store the values passed by the caller at Track() time to avoid needing
// reflection.  For now, we return nil and let session.go handle the nil case
// gracefully.  When the SDK exposes an accessor, this can be updated.
func getServerImpl(mcpServer *mcp.Server) *mcp.Implementation {
	// The official SDK does not expose a public accessor for the Implementation.
	// We return nil here; session metadata will be populated from the
	// InitializeParams of the ServerSession instead.
	_ = mcpServer
	return nil
}
