package mcpcat

import (
	"fmt"

	"github.com/mark3labs/mcp-go/server"
	"github.com/mcpcat/mcpcat-go-sdk/internal/core"
	"github.com/mcpcat/mcpcat-go-sdk/internal/logging"
	"github.com/mcpcat/mcpcat-go-sdk/internal/registry"
	"github.com/mcpcat/mcpcat-go-sdk/internal/tracking"
)

// MCPcat represents the tracking configuration for a server
type MCPcat struct {
	ProjectID string
	Options   Options

	// Internal fields (unexported)
	serverRef any // Reference to the tracked server
}

// Track enables MCPcat tracking on an MCP server.
// It appends MCPCat's hooks to the server. If options.Hooks is provided,
// MCPCat appends its hooks to that instance (preserving any existing hooks).
// Otherwise, MCPCat creates and applies new hooks automatically.
func Track(mcpServer *server.MCPServer, projectID string, options *Options) error {
	if mcpServer == nil {
		return fmt.Errorf("Track: mcpServer must not be nil")
	}
	if projectID == "" {
		return fmt.Errorf("Track: projectID must not be empty")
	}

	hooks := &server.Hooks{}
	if options != nil && options.Hooks != nil {
		hooks = options.Hooks
	}
	server.WithHooks(hooks)(mcpServer)

	return trackInternal(mcpServer, hooks, projectID, options)
}

func trackInternal(mcpServer *server.MCPServer, hooks *server.Hooks, projectID string, options *Options) error {
	opts := DefaultOptions()
	if options != nil {
		opts = *options
	}

	instance := &MCPcat{
		ProjectID: projectID,
		Options:   opts,
		serverRef: mcpServer,
	}

	registryInstance := &core.MCPcatInstance{
		ProjectID: projectID,
		Options:   &instance.Options,
		ServerRef: mcpServer,
	}
	registry.Register(mcpServer, registryInstance)

	logging.SetGlobalDebug(opts.Debug)
	logger := logging.New()
	logger.Info("Initializing MCPCat tracking")

	tracking.AddTracingToHooks(hooks, opts.RedactSensitiveInformation)
	tracking.RegisterGetMoreToolsIfEnabled(mcpServer, &instance.Options)

	return nil
}

// GetMCPcat retrieves the MCPcat instance for a given server
func GetMCPcat(server any) *MCPcat {
	instance := registry.Get(server)
	if instance == nil {
		return nil
	}

	// Convert from registry instance to MCPcat
	if instance.Options == nil {
		return nil
	}

	return &MCPcat{
		ProjectID: instance.ProjectID,
		Options:   *instance.Options,
		serverRef: instance.ServerRef,
	}
}

// UnregisterServer removes a server from the registry
func UnregisterServer(server any) {
	registry.Unregister(server)
}

// Shutdown gracefully shuts down the event publisher
// This should be called when the application is shutting down to ensure
// all queued events are published before exit
func Shutdown() {
	tracking.ShutdownPublisher()
}
