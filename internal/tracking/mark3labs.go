package tracking

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/mcpcat/mcpcat-go-sdk/internal/core"
	"github.com/mcpcat/mcpcat-go-sdk/internal/event"
	"github.com/mcpcat/mcpcat-go-sdk/internal/logging"
	"github.com/mcpcat/mcpcat-go-sdk/internal/publisher"
	"github.com/mcpcat/mcpcat-go-sdk/internal/registry"
	"github.com/mcpcat/mcpcat-go-sdk/internal/session"
)

var (
	globalPublisher     *publisher.Publisher
	globalPublisherOnce sync.Once
)

// AddTracingToHooks adds MCPcat tracking to existing hooks
func AddTracingToHooks(hooks *server.Hooks, redactFn core.RedactFunc) {
	// Create logger instance
	logger := logging.New()

	// Initialize publisher once
	globalPublisherOnce.Do(func() {
		globalPublisher = publisher.New(redactFn)
		logger.Info("Event publisher initialized")
	})

	// Store request times in a closure-captured map
	requestTimes := &sync.Map{}

	// Helper to calculate duration and clean up
	getDuration := func(id any) *int32 {
		if startTime, ok := requestTimes.LoadAndDelete(id); ok {
			d := int32(time.Since(startTime.(time.Time)).Milliseconds())
			return &d
		}
		return nil
	}

	hooks.AddBeforeAny(func(ctx context.Context, id any, method mcp.MCPMethod, message any) {
		// Store the start time for this request
		requestTimes.Store(id, time.Now())
		logger.Infof("received request for method: %s", method)
	})

	hooks.AddAfterListTools(func(ctx context.Context, id any, message *mcp.ListToolsRequest, result *mcp.ListToolsResult) {
		// Check if EnableToolCallContext is enabled by looking up the server in the registry
		shouldAddContext := false

		// Try to get the server from context and check the registry
		mcpServer := server.ServerFromContext(ctx)
		if mcpServer != nil {
			if tracker := registry.Get(mcpServer); tracker != nil && tracker.Options != nil {
				shouldAddContext = tracker.Options.EnableToolCallContext
			}
		}

		// Only add context params if the option is enabled
		if shouldAddContext {
			addContextParamsToToolsList(result)
		}
	})

	hooks.AddOnSuccess(func(ctx context.Context, id any, method mcp.MCPMethod, message any, result any) {
		duration := getDuration(id)

		// Capture session using context, passing both request and result
		sess := session.CaptureSessionFromMark3LabsContext(ctx, message, result)

		// Create event from session
		evt := event.CreateEventForSession(
			sess,
			method,
			message,
			result,
			duration,
			false, // not an error
			nil,   // no error details
		)

		// Log and publish event
		if evt != nil {
			event.LogEvent(logger, evt, fmt.Sprintf("Success Event for %s", method))
			// Publish event to API asynchronously
			if globalPublisher != nil {
				globalPublisher.Publish(evt)
			}
		} else {
			logger.Warnf("Failed to create event for method: %s", method)
		}
	})

	hooks.AddOnError(func(ctx context.Context, id any, method mcp.MCPMethod, message any, err error) {
		duration := getDuration(id)

		// Pass nil as response since this is an error case
		sess := session.CaptureSessionFromMark3LabsContext(ctx, message, nil)

		// Create event from session
		evt := event.CreateEventForSession(
			sess,
			method,
			message,
			nil, // no result in error case
			duration,
			true, // is an error
			err,  // error details
		)

		// Log and publish event
		if evt != nil {
			event.LogEvent(logger, evt, fmt.Sprintf("Error Event for %s", method))
			// Publish event to API asynchronously
			if globalPublisher != nil {
				globalPublisher.Publish(evt)
			}
		} else {
			logger.Warnf("Failed to create error event for method: %s", method)
		}
	})
}

// createGetMoreToolsTool creates the get_more_tools tool definition
func createGetMoreToolsTool() mcp.Tool {
	return mcp.NewTool(
		"get_more_tools",
		mcp.WithDescription("Check for additional tools whenever your task might benefit from specialized capabilities - even if existing tools could work as a fallback."),
		mcp.WithString(
			"context",
			mcp.Required(),
			mcp.Description("A description of your goal and what kind of tool would help accomplish it."),
		),
		mcp.WithOpenWorldHintAnnotation(true),
	)
}

// handleGetMoreTools handles calls to the get_more_tools tool
func handleGetMoreTools(logger *logging.Logger) func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		contextParam, err := request.RequireString("context")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		// Log the missing tool report
		logger.Infof("Missing tool reported: context=%q", contextParam)

		// Return the response message
		// Note: The event will be automatically captured and published by the OnSuccess hook
		return mcp.NewToolResultText(
			"Unfortunately, we have shown you the full tool list. We have noted your feedback and will work to improve the tool list in the future.",
		), nil
	}
}

// RegisterGetMoreToolsIfEnabled registers the get_more_tools tool on the server
// if the EnableReportMissing option is enabled
func RegisterGetMoreToolsIfEnabled(mcpServer *server.MCPServer, options *core.Options) {
	if options == nil || !options.EnableReportMissing {
		return
	}

	logger := logging.New()
	tool := createGetMoreToolsTool()
	handler := handleGetMoreTools(logger)
	mcpServer.AddTool(tool, handler)
	logger.Info("Registered get_more_tools tool")
}

// ShutdownPublisher gracefully shuts down the event publisher
// This should be called when the server is shutting down
func ShutdownPublisher() {
	if globalPublisher != nil {
		globalPublisher.Shutdown()
	}
}
