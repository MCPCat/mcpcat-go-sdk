package mcpcat

import (
	"fmt"

	"github.com/mark3labs/mcp-go/server"
	"github.com/mcpcat/mcpcat-go-sdk/internal/compat"
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

// SetupTracking configures MCPcat tracking for a server
func SetupTracking(mcpServer any, hooks *server.Hooks, projectID *string, options *Options) (*MCPcat, error) {
	// Validate server type
	serverType := compat.DetectServerType(mcpServer)
	if serverType != compat.ServerTypeMark3Labs {
		return nil, fmt.Errorf("SetupTracking: unsupported server type '%s' (expected '%s'), server: %T",
			serverType, compat.ServerTypeMark3Labs, mcpServer)
	}

	// Create MCPcat instance
	mcpcat := &MCPcat{
		serverRef: mcpServer,
		Options:   DefaultOptions(),
	}

	// Set project ID if provided
	if projectID != nil {
		mcpcat.ProjectID = *projectID
	}

	// Override with user options if provided
	if options != nil {
		mcpcat.Options = *options
	}

	// Register in global map via internal registry
	registryInstance := &core.MCPcatInstance{
		ProjectID: mcpcat.ProjectID,
		Options:   &mcpcat.Options,
		ServerRef: mcpServer,
	}
	registry.Register(mcpServer, registryInstance)

	// Configure logging based on debug setting
	logging.SetGlobalDebug(mcpcat.Options.Debug)

	// Initialize logging
	logger := logging.New()
	logger.Info("Initializing MCPCat tracking")

	// Add tracking hooks - no server parameter needed!
	tracking.AddTracingToHooks(hooks, mcpcat.Options.RedactSensitiveInformation)

	// Register get_more_tools tool if enabled
	if srv, ok := mcpServer.(*server.MCPServer); ok {
		tracking.RegisterGetMoreToolsIfEnabled(srv, &mcpcat.Options)
	}

	// TODO: Setup exporters if configured
	// Initialize exporters from mcpcat.Options.Exporters

	return mcpcat, nil
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
