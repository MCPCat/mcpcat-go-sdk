package event

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	mcpcatapi "github.com/mcpcat/mcpcat-go-api"
	"github.com/mcpcat/mcpcat-go-sdk/internal/core"
	"github.com/mcpcat/mcpcat-go-sdk/internal/logging"
	"github.com/segmentio/ksuid"
)

// CreateEventForSession creates an Event struct from session data and request/response information
func CreateEventForSession(
	session *core.Session,
	method mcp.MCPMethod,
	request any,
	response any,
	duration *int32,
	isError bool,
	errorDetails error,
) *Event {
	if session == nil {
		return nil
	}

	// Generate unique event ID with MCPCat prefix using KSUID
	eventID := fmt.Sprintf("%s_%s", core.PrefixEvent, ksuid.New().String())

	// Use session ID from the session
	sessionID := ""
	if session.SessionID != nil {
		sessionID = *session.SessionID
	}

	// Map MCP method to event type
	eventType := mapMethodToEventType(method)

	// Create the event
	event := &Event{
		PublishEventRequest: mcpcatapi.PublishEventRequest{
			Id:        &eventID,
			ProjectId: session.ProjectID, // Use ProjectID from session
			SessionId: sessionID,
			EventType: &eventType,
			Duration:  duration,
			Timestamp: ptr(time.Now()),
		},
	}

	// Set error status if applicable
	if isError {
		event.IsError = &isError
		if errorDetails != nil {
			event.Error = map[string]any{
				"message": errorDetails.Error(),
			}
		}
	}

	// Extract user intent from context parameter for tool calls
	if method == mcp.MethodToolsCall {
		userIntent := extractUserIntentFromRequest(request)
		if userIntent != "" {
			event.UserIntent = &userIntent
		}
	}

	// Extract parameters and response data
	event.Parameters = extractParameters(request)
	if response != nil && !isError {
		event.Response = extractResponse(response)
	}

	// Copy session metadata to event
	copySessionToEvent(session, event)

	// Set resource name for resource-related methods
	if method == mcp.MethodResourcesRead {
		resourceName := extractResourceName(request)
		if resourceName != "" {
			event.ResourceName = &resourceName
		}
	}

	// Set resource name for tool calls (tool name)
	if method == mcp.MethodToolsCall {
		toolName := extractToolName(request)
		if toolName != "" {
			event.ResourceName = &toolName
		}
	}

	return event
}

// mapMethodToEventType converts an MCP method to MCPCat event type format
func mapMethodToEventType(method mcp.MCPMethod) string {
	// MCPCat uses "mcp:" prefix for MCP protocol events
	return fmt.Sprintf("mcp:%s", string(method))
}

// extractUserIntentFromRequest extracts the context parameter from a tool call request
func extractUserIntentFromRequest(request any) string {
	// Try to cast to CallToolRequest
	if toolReq, ok := request.(*mcp.CallToolRequest); ok {
		// Extract the context parameter from arguments
		if args := toolReq.GetArguments(); args != nil {
			if context, ok := args["context"].(string); ok {
				return context
			}
		}
	}
	return ""
}

// extractParameters extracts parameters from the request
func extractParameters(request any) map[string]any {
	params := make(map[string]any)

	switch req := request.(type) {
	case *mcp.CallToolRequest:
		params["name"] = req.Params.Name
		if args := req.GetArguments(); args != nil {
			// Filter out the context parameter since it's stored in UserIntent
			filteredArgs := make(map[string]any)
			for k, v := range args {
				if k != "context" {
					filteredArgs[k] = v
				}
			}
			if len(filteredArgs) > 0 {
				params["arguments"] = filteredArgs
			}
		}
	case *mcp.ReadResourceRequest:
		params["uri"] = req.Params.URI
	case *mcp.GetPromptRequest:
		params["name"] = req.Params.Name
		if len(req.Params.Arguments) > 0 {
			params["arguments"] = req.Params.Arguments
		}
	case *mcp.InitializeRequest:
		params["protocolVersion"] = req.Params.ProtocolVersion
		params["clientInfo"] = req.Params.ClientInfo
	}

	if len(params) == 0 {
		return nil
	}
	return params
}

// extractResponse extracts response data
func extractResponse(response any) map[string]any {
	resp := make(map[string]any)

	switch r := response.(type) {
	case *mcp.CallToolResult:
		// Include the entire CallToolResult structure
		if r.StructuredContent != nil {
			resp["structuredContent"] = convertToMap(r.StructuredContent)
		}
		if len(r.Content) > 0 {
			resp["content"] = convertToMap(r.Content)
		}
		resp["isError"] = r.IsError
	case *mcp.ReadResourceResult:
		if len(r.Contents) > 0 {
			resp["contents"] = convertToMap(r.Contents)
		}
	case *mcp.GetPromptResult:
		resp["description"] = r.Description
		if len(r.Messages) > 0 {
			resp["messages"] = convertToMap(r.Messages)
		}
	case *mcp.InitializeResult:
		resp["protocolVersion"] = r.ProtocolVersion
		resp["serverInfo"] = convertToMap(r.ServerInfo)
	case *mcp.ListToolsResult:
		if len(r.Tools) > 0 {
			resp["tools"] = convertToMap(r.Tools)
		}
	}

	if len(resp) == 0 {
		return nil
	}
	return resp
}

// convertToMap converts any value (including structs, slices of structs) to map[string]any or []any
// by marshaling to JSON and unmarshaling back. This ensures the redactor can process all fields.
// If conversion fails, the original value is returned to avoid impacting the server.
func convertToMap(v any) any {
	if v == nil {
		return nil
	}

	// Use defer/recover to ensure any panics don't crash the server
	defer func() {
		if r := recover(); r != nil {
			logger := logging.New()
			logger.Debugf("convertToMap: panic recovered during conversion: %v (value type: %T)", r, v)
		}
	}()

	// Marshal to JSON and unmarshal to generic types
	data, err := json.Marshal(v)
	if err != nil {
		// If marshaling fails, return the original value silently
		return v
	}

	var result any
	if err := json.Unmarshal(data, &result); err != nil {
		// If unmarshaling fails, return the original value silently
		return v
	}

	return result
}

// extractResourceName extracts the resource URI from a resource read request
func extractResourceName(request any) string {
	if resourceReq, ok := request.(*mcp.ReadResourceRequest); ok {
		return resourceReq.Params.URI
	}
	return ""
}

// extractToolName extracts the tool name from a tool call request
func extractToolName(request any) string {
	if toolReq, ok := request.(*mcp.CallToolRequest); ok {
		return toolReq.Params.Name
	}
	return ""
}

// copySessionToEvent copies session metadata fields to the event
func copySessionToEvent(session *core.Session, event *Event) {
	if session == nil || event == nil {
		return
	}

	// Copy all session fields to the event
	event.IpAddress = session.IpAddress
	event.SdkLanguage = session.SdkLanguage
	event.McpcatVersion = session.McpcatVersion
	event.ServerName = session.ServerName
	event.ServerVersion = session.ServerVersion
	event.ClientName = session.ClientName
	event.ClientVersion = session.ClientVersion
	event.IdentifyActorGivenId = session.IdentifyActorGivenId
	event.IdentifyActorName = session.IdentifyActorName
	event.IdentifyData = session.IdentifyData
}

// CreateIdentifyEvent creates an Event for mcpcat:identify event type
func CreateIdentifyEvent(session *core.Session) *Event {
	if session == nil {
		return nil
	}

	// Generate unique event ID with MCPCat prefix using KSUID
	eventID := fmt.Sprintf("%s_%s", core.PrefixEvent, ksuid.New().String())

	// Use session ID from the session
	sessionID := ""
	if session.SessionID != nil {
		sessionID = *session.SessionID
	}

	// Create the identify event
	eventType := "mcpcat:identify"
	event := &Event{
		PublishEventRequest: mcpcatapi.PublishEventRequest{
			Id:        &eventID,
			ProjectId: session.ProjectID,
			SessionId: sessionID,
			EventType: &eventType,
			Timestamp: ptr(time.Now()),
		},
	}

	// Copy session metadata to event
	copySessionToEvent(session, event)

	return event
}

// ptr is a helper function to get a pointer to a value
func ptr[T any](v T) *T {
	return &v
}

// LogEvent logs an event in a formatted, human-readable way for debugging
func LogEvent(logger interface{ Infof(string, ...any) }, evt *Event, title string) {
	if evt == nil {
		logger.Infof("%s: <nil event>", title)
		return
	}

	logger.Infof("=== %s ===", title)

	// Basic event info
	if evt.Id != nil {
		logger.Infof("  Event ID: %s", *evt.Id)
	}
	if evt.EventType != nil {
		logger.Infof("  Event Type: %s", *evt.EventType)
	}
	if evt.ProjectId != nil {
		logger.Infof("  Project ID: %s", *evt.ProjectId)
	}
	logger.Infof("  Session ID: %s", evt.SessionId)

	// Timing info
	if evt.Timestamp != nil {
		logger.Infof("  Timestamp: %s", evt.Timestamp.Format(time.RFC3339))
	}
	if evt.Duration != nil {
		logger.Infof("  Duration: %d ms", *evt.Duration)
	}

	// Error status
	if evt.IsError != nil && *evt.IsError {
		logger.Infof("  Is Error: true")
		if evt.Error != nil {
			logger.Infof("  Error Details: %v", evt.Error)
		}
	}

	// User intent
	if evt.UserIntent != nil {
		logger.Infof("  User Intent: %s", *evt.UserIntent)
	}

	// Resource name (for resource events)
	if evt.ResourceName != nil {
		logger.Infof("  Resource Name: %s", *evt.ResourceName)
	}

	// Parameters
	if len(evt.Parameters) > 0 {
		logger.Infof("  Parameters:")
		for k, v := range evt.Parameters {
			logger.Infof("    %s: %v", k, v)
		}
	}

	// Response
	if len(evt.Response) > 0 {
		logger.Infof("  Response:")
		for k, v := range evt.Response {
			// Truncate long values for readability
			valStr := fmt.Sprintf("%v", v)
			if len(valStr) > 200 {
				valStr = valStr[:197] + "..."
			}
			logger.Infof("    %s: %s", k, valStr)
		}
	}

	// Session metadata
	logger.Infof("  Session Metadata:")
	if evt.ClientName != nil {
		logger.Infof("    Client: %s", *evt.ClientName)
		if evt.ClientVersion != nil {
			logger.Infof("    Client Version: %s", *evt.ClientVersion)
		}
	}
	if evt.ServerName != nil {
		logger.Infof("    Server: %s", *evt.ServerName)
		if evt.ServerVersion != nil {
			logger.Infof("    Server Version: %s", *evt.ServerVersion)
		}
	}
	if evt.SdkLanguage != nil {
		logger.Infof("    SDK Language: %s", *evt.SdkLanguage)
	}
	if evt.McpcatVersion != nil {
		logger.Infof("    MCPCat Version: %s", *evt.McpcatVersion)
	}
	if evt.IpAddress != nil {
		logger.Infof("    IP Address: %s", *evt.IpAddress)
	}

	// Identity info
	if evt.IdentifyActorGivenId != nil || evt.IdentifyActorName != nil {
		logger.Infof("  Identity:")
		if evt.IdentifyActorGivenId != nil {
			logger.Infof("    Actor ID: %s", *evt.IdentifyActorGivenId)
		}
		if evt.IdentifyActorName != nil {
			logger.Infof("    Actor Name: %s", *evt.IdentifyActorName)
		}
		if len(evt.IdentifyData) > 0 {
			logger.Infof("    Additional Data:")
			for k, v := range evt.IdentifyData {
				logger.Infof("      %s: %v", k, v)
			}
		}
	}

	logger.Infof("=== End %s ===", title)
}
