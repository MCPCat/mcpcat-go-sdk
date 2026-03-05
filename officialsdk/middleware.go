package officialsdk

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	mcpcat "github.com/mcpcat/mcpcat-go-sdk"
)

// newTrackingMiddleware creates a single mcp.Middleware that intercepts all
// incoming requests and captures MCPCat events.
func newTrackingMiddleware(
	projectID string,
	opts *Options,
	publishFn func(*mcpcat.Event),
	serverImpl *mcp.Implementation,
) mcp.Middleware {
	sessionMap := &sync.Map{}

	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			startTime := time.Now()

			// For tools/call requests, strip the injected "context" param
			// BEFORE calling next so the go-sdk schema validation passes.
			// The go-sdk validates against the original struct-derived schema
			// which has additionalProperties: false, so any extra properties
			// (like our injected "context") would be rejected.
			var userIntent string
			if method == "tools/call" && !opts.DisableToolCallContext {
				userIntent = stripContextParam(req)
			}

			result, err := next(ctx, method, req)

			// Cap duration to math.MaxInt32 to prevent int32 overflow.
			ms := time.Since(startTime).Milliseconds()
			if ms > math.MaxInt32 {
				ms = math.MaxInt32
			}
			duration := int32(ms)

			// Inject context params for tools/list BEFORE returning the result
			// so the modified result is sent back to the client.
			if method == "tools/list" && !opts.DisableToolCallContext {
				injectContextParams(result)
			}

			// Capture event asynchronously.
			// Use context.WithoutCancel so the goroutine is not cancelled
			// when the original request context is done.
			detachedCtx := context.WithoutCancel(ctx)
			go captureEvent(detachedCtx, method, req, result, err, &duration, projectID, opts, publishFn, sessionMap, serverImpl, userIntent)

			return result, err
		}
	}
}

// stripContextParam extracts the "context" value from a tools/call request's
// arguments and removes it from the request in-place. This prevents the go-sdk
// schema validation from rejecting the injected parameter. Returns the
// extracted context value (empty string if not present).
//
// Tools that natively define "context" (like get_more_tools) are skipped.
func stripContextParam(req mcp.Request) string {
	toolReq, ok := req.(*mcp.CallToolRequest)
	if !ok || toolReq.Params == nil || len(toolReq.Params.Arguments) == 0 {
		return ""
	}

	// Don't strip context from tools that define it natively.
	if toolReq.Params.Name == "get_more_tools" {
		return ""
	}

	var args map[string]any
	if err := json.Unmarshal(toolReq.Params.Arguments, &args); err != nil {
		return ""
	}

	contextVal, _ := args["context"].(string)
	if contextVal == "" {
		return ""
	}

	delete(args, "context")
	cleaned, err := json.Marshal(args)
	if err != nil {
		return contextVal
	}
	toolReq.Params.Arguments = cleaned
	return contextVal
}

// captureEvent creates and publishes an MCPCat event from the middleware context.
func captureEvent(
	ctx context.Context,
	method string,
	req mcp.Request,
	result mcp.Result,
	callErr error,
	duration *int32,
	projectID string,
	opts *Options,
	publishFn func(*mcpcat.Event),
	sessionMap *sync.Map,
	serverImpl *mcp.Implementation,
	userIntent string,
) {
	// Build session
	ps := getOrCreateSession(req, sessionMap, serverImpl, projectID)
	if ps == nil {
		return
	}

	// Lock the session for all field reads/writes in this function.
	ps.mu.Lock()

	// For initialize responses, capture server info from the result
	if method == "initialize" && result != nil {
		updateSessionFromInitResult(ps, result)
	}

	// Determine error state
	isError := callErr != nil
	var errorDetails error

	if callErr != nil {
		errorDetails = callErr
	}

	// Check CallToolResult.IsError for tool-level errors
	if !isError && result != nil {
		if toolResult, ok := result.(*mcp.CallToolResult); ok && toolResult.IsError {
			isError = true
			var errorMessages []string
			for _, content := range toolResult.Content {
				if textContent, ok := content.(*mcp.TextContent); ok {
					errorMessages = append(errorMessages, textContent.Text)
				}
			}
			if len(errorMessages) > 0 {
				errorDetails = fmt.Errorf("%s", strings.Join(errorMessages, " "))
			}
		}
	}

	// Map MCP method to event type
	eventType := fmt.Sprintf("mcp:%s", method)

	// Create event using core API
	evt := mcpcat.NewEvent(ps.sess, eventType, duration, isError, errorDetails)
	if evt == nil {
		ps.mu.Unlock()
		return
	}

	// Set user intent from context parameter (extracted before schema validation).
	if method == "tools/call" && userIntent != "" {
		evt.UserIntent = &userIntent
	}

	// Extract parameters and response data
	evt.Parameters = extractParameters(method, req)
	if result != nil && !isError {
		evt.Response = extractResponse(method, result)
	}

	// Extract transport-layer metadata (headers, token info).
	if extra := extractExtra(req); extra != nil {
		if evt.Parameters == nil {
			evt.Parameters = make(map[string]any)
		}
		evt.Parameters["extra"] = extra
	}

	// Set resource name for resource-related methods
	if method == "resources/read" {
		resourceURI := extractResourceURI(req)
		if resourceURI != "" {
			evt.ResourceName = &resourceURI
		}
	}

	// Set resource name for tool calls (tool name)
	if method == "tools/call" {
		toolName := extractToolName(req)
		if toolName != "" {
			evt.ResourceName = &toolName
		}
	}

	// Handle identify once per session for tool calls.
	// We need to capture whether identify should run while holding the lock,
	// but we must release the lock before calling the user's Identify callback
	// (which may be slow or block).
	var shouldIdentify bool
	if method == "tools/call" && opts != nil && opts.Identify != nil {
		shouldIdentify = true
	}

	// Release the session lock before calling external callbacks.
	ps.mu.Unlock()

	// Use sync.Once to ensure Identify is called at most once per session.
	if shouldIdentify {
		ps.identifyOnce.Do(func() {
			if toolReq, ok := req.(*mcp.CallToolRequest); ok {
				identifyInfo := opts.Identify(ctx, toolReq)
				if identifyInfo != nil {
					ps.mu.Lock()
					ps.sess.IdentifyActorGivenId = &identifyInfo.UserID
					ps.sess.IdentifyActorName = &identifyInfo.UserName
					ps.sess.IdentifyData = identifyInfo.UserData
					ps.mu.Unlock()

					// Copy updated identity info to this event
					evt.IdentifyActorGivenId = &identifyInfo.UserID
					evt.IdentifyActorName = &identifyInfo.UserName
					evt.IdentifyData = identifyInfo.UserData

					// Publish mcpcat:identify event
					ps.mu.Lock()
					identifyEvent := mcpcat.CreateIdentifyEvent(ps.sess)
					ps.mu.Unlock()
					if identifyEvent != nil {
						publishFn(identifyEvent)
					}
				}
			}
		})
	}

	publishFn(evt)
}
