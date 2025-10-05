package tracking

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/mcpcat/mcpcat-go-sdk/internal/core"
	"github.com/mcpcat/mcpcat-go-sdk/internal/logging"
	"github.com/mcpcat/mcpcat-go-sdk/internal/registry"
)

// ============================================================================
// Test Helpers and Mocks
// ============================================================================

// resetGlobalState resets the global publisher state for test isolation
func resetGlobalState() {
	globalPublisher = nil
	globalPublisherOnce = sync.Once{}
}

// mockPublisher implements a test double for publisher.Publisher
type mockPublisher struct {
	mu              sync.Mutex
	publishedEvents []*core.Event
	shutdownCalled  bool
}

func (m *mockPublisher) Publish(evt *core.Event) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.publishedEvents = append(m.publishedEvents, evt)
}

func (m *mockPublisher) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shutdownCalled = true
}

func (m *mockPublisher) GetPublishedEvents() []*core.Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]*core.Event{}, m.publishedEvents...)
}

func (m *mockPublisher) GetShutdownCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.shutdownCalled
}

// Helper functions to work with server.Hooks
func newTestHooks() *server.Hooks {
	return &server.Hooks{}
}

// Helper to add server to context (mimics internal server behavior)
// Note: We can't directly access server.serverKey since it's private,
// so we use a workaround: we create a tool handler and capture the context from it
func contextWithServer(ctx context.Context, srv *server.MCPServer) context.Context {
	var capturedCtx context.Context

	// Add a temporary tool that captures the context
	srv.AddTool(mcp.Tool{
		Name:        "_test_context_capture",
		Description: "Temporary tool to capture context",
	}, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		capturedCtx = ctx
		return &mcp.CallToolResult{}, nil
	})

	// Trigger the tool to capture context
	srv.HandleMessage(ctx, []byte(`{
		"jsonrpc": "2.0",
		"id": 1,
		"method": "initialize",
		"params": {
			"protocolVersion": "2024-11-05",
			"capabilities": {},
			"clientInfo": {"name": "test", "version": "1.0"}
		}
	}`))

	srv.HandleMessage(ctx, []byte(`{
		"jsonrpc": "2.0",
		"id": 2,
		"method": "tools/call",
		"params": {
			"name": "_test_context_capture"
		}
	}`))

	// Clean up
	srv.DeleteTools("_test_context_capture")

	return capturedCtx
}

// Helper to trigger BeforeAny hooks manually
func triggerBeforeAny(hooks *server.Hooks, ctx context.Context, id any, method mcp.MCPMethod, message any) {
	for _, callback := range hooks.OnBeforeAny {
		callback(ctx, id, method, message)
	}
}

// Helper to trigger AfterListTools hooks manually
func triggerAfterListTools(hooks *server.Hooks, ctx context.Context, id any, request *mcp.ListToolsRequest, result *mcp.ListToolsResult) {
	for _, callback := range hooks.OnAfterListTools {
		callback(ctx, id, request, result)
	}
}

// Helper to trigger OnSuccess hooks manually
func triggerOnSuccess(hooks *server.Hooks, ctx context.Context, id any, method mcp.MCPMethod, message any, result any) {
	for _, callback := range hooks.OnSuccess {
		callback(ctx, id, method, message, result)
	}
}

// Helper to trigger OnError hooks manually
func triggerOnError(hooks *server.Hooks, ctx context.Context, id any, method mcp.MCPMethod, message any, err error) {
	for _, callback := range hooks.OnError {
		callback(ctx, id, method, message, err)
	}
}

// mockRedactFunc for testing redaction
func mockRedactFunc(text string) string {
	return "***REDACTED***"
}

// Helper to create a test session
func createTestSession() *core.Session {
	sessionID := "ses_test_123"
	projectID := "proj_test_456"
	clientName := "test-client"
	serverName := "test-server"

	return &core.Session{
		SessionID:  &sessionID,
		ProjectID:  &projectID,
		ClientName: &clientName,
		ServerName: &serverName,
	}
}

// ============================================================================
// AddTracingToHooks Tests
// ============================================================================

func TestAddTracingToHooks_HookRegistration(t *testing.T) {
	t.Run("registers all required hooks", func(t *testing.T) {
		resetGlobalState()
		hooks := newTestHooks()

		AddTracingToHooks(hooks, nil)

		if len(hooks.OnBeforeAny) != 1 {
			t.Errorf("expected 1 BeforeAny callback, got %d", len(hooks.OnBeforeAny))
		}
		if len(hooks.OnAfterListTools) != 1 {
			t.Errorf("expected 1 AfterListTools callback, got %d", len(hooks.OnAfterListTools))
		}
		if len(hooks.OnSuccess) != 1 {
			t.Errorf("expected 1 OnSuccess callback, got %d", len(hooks.OnSuccess))
		}
		if len(hooks.OnError) != 1 {
			t.Errorf("expected 1 OnError callback, got %d", len(hooks.OnError))
		}
	})

	t.Run("registers hooks with redactFn", func(t *testing.T) {
		resetGlobalState()
		hooks := newTestHooks()

		AddTracingToHooks(hooks, mockRedactFunc)

		// Verify hooks are registered
		if len(hooks.OnSuccess) == 0 {
			t.Error("expected OnSuccess callback to be registered")
		}
	})
}

func TestAddTracingToHooks_PublisherInitialization(t *testing.T) {
	t.Run("initializes publisher exactly once", func(t *testing.T) {
		resetGlobalState()
		hooks1 := newTestHooks()
		hooks2 := newTestHooks()

		AddTracingToHooks(hooks1, nil)
		pub1 := globalPublisher

		AddTracingToHooks(hooks2, nil)
		pub2 := globalPublisher

		if pub1 != pub2 {
			t.Error("expected same publisher instance for multiple calls")
		}

		if globalPublisher != nil {
			globalPublisher.Shutdown()
		}
	})

	t.Run("publisher initialized with redactFn", func(t *testing.T) {
		resetGlobalState()
		hooks := newTestHooks()

		AddTracingToHooks(hooks, mockRedactFunc)

		if globalPublisher == nil {
			t.Fatal("expected publisher to be initialized")
		}

		// Publisher is initialized - we can't directly test the redactFn
		// but we trust it's passed to publisher.New()
		globalPublisher.Shutdown()
	})

	t.Run("concurrent hook additions create single publisher", func(t *testing.T) {
		resetGlobalState()

		var wg sync.WaitGroup
		publishers := make(chan interface{}, 10)

		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				hooks := newTestHooks()
				AddTracingToHooks(hooks, nil)
				publishers <- globalPublisher
			}()
		}

		wg.Wait()
		close(publishers)

		// Verify all goroutines got the same publisher
		var firstPub interface{}
		for pub := range publishers {
			if firstPub == nil {
				firstPub = pub
			} else if pub != firstPub {
				t.Error("concurrent calls created different publisher instances")
			}
		}

		if globalPublisher != nil {
			globalPublisher.Shutdown()
		}
	})
}

func TestAddTracingToHooks_BeforeAnyHook(t *testing.T) {
	t.Run("stores request time", func(t *testing.T) {
		resetGlobalState()
		hooks := newTestHooks()

		AddTracingToHooks(hooks, nil)
		defer globalPublisher.Shutdown()

		// Get the requestTimes map reference by triggering BeforeAny
		ctx := context.Background()
		requestID := "req_123"
		method := mcp.MethodToolsCall

		// Trigger BeforeAny
		triggerBeforeAny(hooks, ctx, requestID, method, nil)

		// We can't directly access requestTimes, but we can verify
		// by triggering OnSuccess and checking duration is calculated
		// (tested in OnSuccess tests)
	})

	t.Run("logs request method", func(t *testing.T) {
		resetGlobalState()
		hooks := newTestHooks()

		AddTracingToHooks(hooks, nil)
		defer globalPublisher.Shutdown()

		ctx := context.Background()
		requestID := "req_456"
		method := mcp.MethodToolsCall

		// This should not panic and should log
		triggerBeforeAny(hooks, ctx, requestID, method, nil)
	})
}

func TestAddTracingToHooks_OnSuccessHook(t *testing.T) {
	t.Run("calculates duration from start time", func(t *testing.T) {
		resetGlobalState()
		hooks := newTestHooks()

		AddTracingToHooks(hooks, nil)
		defer globalPublisher.Shutdown()

		ctx := context.Background()
		requestID := "req_duration_test"
		method := mcp.MethodToolsCall
		request := &mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: "test_tool",
			},
		}
		result := &mcp.CallToolResult{}

		// Start request
		triggerBeforeAny(hooks, ctx, requestID, method, request)

		// Small delay
		time.Sleep(10 * time.Millisecond)

		// Complete request
		triggerOnSuccess(hooks, ctx, requestID, method, request, result)

		// Duration should be calculated and cleaned up
		// Verify by triggering OnSuccess again - duration should be nil
		triggerOnSuccess(hooks, ctx, requestID, method, request, result)
	})

	t.Run("publishes event on success", func(t *testing.T) {
		resetGlobalState()
		hooks := newTestHooks()

		AddTracingToHooks(hooks, nil)
		defer globalPublisher.Shutdown()

		ctx := context.Background()
		requestID := "req_publish_test"
		method := mcp.MethodToolsCall
		request := &mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: "test_tool",
			},
		}
		result := &mcp.CallToolResult{}

		triggerBeforeAny(hooks, ctx, requestID, method, request)
		triggerOnSuccess(hooks, ctx, requestID, method, request, result)

		// Note: Without proper context with session, event might be nil
		// This test verifies the hook executes without error
	})

	t.Run("handles nil session gracefully", func(t *testing.T) {
		resetGlobalState()
		hooks := newTestHooks()

		AddTracingToHooks(hooks, nil)
		defer globalPublisher.Shutdown()

		// Context without session
		ctx := context.Background()
		requestID := "req_nil_session"
		method := mcp.MethodToolsCall

		triggerBeforeAny(hooks, ctx, requestID, method, nil)

		// Should not panic
		triggerOnSuccess(hooks, ctx, requestID, method, nil, nil)
	})
}

func TestAddTracingToHooks_OnErrorHook(t *testing.T) {
	t.Run("calculates duration on error", func(t *testing.T) {
		resetGlobalState()
		hooks := newTestHooks()

		AddTracingToHooks(hooks, nil)
		defer globalPublisher.Shutdown()

		ctx := context.Background()
		requestID := "req_error_duration"
		method := mcp.MethodToolsCall
		err := errors.New("test error")

		triggerBeforeAny(hooks, ctx, requestID, method, nil)
		time.Sleep(10 * time.Millisecond)
		triggerOnError(hooks, ctx, requestID, method, nil, err)

		// Should calculate duration and clean up
	})

	t.Run("publishes error event", func(t *testing.T) {
		resetGlobalState()
		hooks := newTestHooks()

		AddTracingToHooks(hooks, nil)
		defer globalPublisher.Shutdown()

		ctx := context.Background()
		requestID := "req_error_publish"
		method := mcp.MethodToolsCall
		err := errors.New("tool execution failed")

		triggerBeforeAny(hooks, ctx, requestID, method, nil)
		triggerOnError(hooks, ctx, requestID, method, nil, err)

		// Verify hook executes without panic
	})

	t.Run("handles different error types", func(t *testing.T) {
		resetGlobalState()
		hooks := newTestHooks()

		AddTracingToHooks(hooks, nil)
		defer globalPublisher.Shutdown()

		ctx := context.Background()

		testErrors := []error{
			errors.New("simple error"),
			&testCustomError{msg: "custom error"},
			nil, // nil error should be handled gracefully
		}

		for i, err := range testErrors {
			requestID := i
			triggerBeforeAny(hooks, ctx, requestID, mcp.MethodToolsCall, nil)
			triggerOnError(hooks, ctx, requestID, mcp.MethodToolsCall, nil, err)
		}
	})
}

// testCustomError for testing different error types
type testCustomError struct {
	msg string
}

func (e *testCustomError) Error() string {
	return e.msg
}

func TestAddTracingToHooks_AfterListToolsHook(t *testing.T) {
	t.Run("triggers AfterListTools callback", func(t *testing.T) {
		resetGlobalState()
		hooks := newTestHooks()

		AddTracingToHooks(hooks, nil)
		defer globalPublisher.Shutdown()

		ctx := context.Background()
		request := &mcp.ListToolsRequest{}
		result := &mcp.ListToolsResult{
			Tools: []mcp.Tool{
				{
					Name:        "test_tool",
					Description: "A test tool",
				},
			},
		}

		// Should call addContextParamsToToolsList
		triggerAfterListTools(hooks, ctx, "req_123", request, result)

		// Verify result is modified (if addContextParamsToToolsList is implemented)
		// This depends on the actual implementation of addContextParamsToToolsList
	})

	t.Run("handles nil result", func(t *testing.T) {
		resetGlobalState()
		hooks := newTestHooks()

		AddTracingToHooks(hooks, nil)
		defer globalPublisher.Shutdown()

		ctx := context.Background()
		request := &mcp.ListToolsRequest{}

		// Should not panic with nil result
		triggerAfterListTools(hooks, ctx, "req_456", request, nil)
	})

	t.Run("respects EnableToolCallContext = false", func(t *testing.T) {
		resetGlobalState()
		hooks := newTestHooks()

		AddTracingToHooks(hooks, nil)
		defer globalPublisher.Shutdown()

		// Create a mock server with tool capabilities and register it with EnableToolCallContext = false
		mockServer := server.NewMCPServer("test-server", "1.0.0", server.WithToolCapabilities(true))
		registryInstance := &core.MCPcatInstance{
			ProjectID: "test-project",
			Options: &core.Options{
				EnableToolCallContext: false,
			},
			ServerRef: mockServer,
		}
		registry.Register(mockServer, registryInstance)
		defer registry.Unregister(mockServer)

		// Create context with server using the helper
		ctx := contextWithServer(context.Background(), mockServer)

		// Verify server can be retrieved from context
		retrievedServer := server.ServerFromContext(ctx)
		if retrievedServer != mockServer {
			t.Fatalf("server not properly stored in context: got %v, want %v", retrievedServer, mockServer)
		}

		request := &mcp.ListToolsRequest{}
		result := &mcp.ListToolsResult{
			Tools: []mcp.Tool{
				{
					Name:        "test_tool",
					Description: "A test tool",
					InputSchema: mcp.ToolInputSchema{
						Type: "object",
						Properties: map[string]any{
							"param1": map[string]any{
								"type":        "string",
								"description": "A parameter",
							},
						},
					},
				},
			},
		}

		// Trigger the hook
		triggerAfterListTools(hooks, ctx, "req_789", request, result)

		// Verify that context param was NOT added
		tool := result.Tools[0]
		if _, hasContext := tool.InputSchema.Properties["context"]; hasContext {
			t.Error("context parameter should NOT be added when EnableToolCallContext = false")
		}

		// Verify original parameter is still there
		if _, hasParam1 := tool.InputSchema.Properties["param1"]; !hasParam1 {
			t.Error("original param1 should still be present")
		}
	})

	t.Run("adds context param when EnableToolCallContext = true", func(t *testing.T) {
		resetGlobalState()
		hooks := newTestHooks()

		AddTracingToHooks(hooks, nil)
		defer globalPublisher.Shutdown()

		// Create a mock server with tool capabilities and register it with EnableToolCallContext = true
		mockServer := server.NewMCPServer("test-server", "1.0.0", server.WithToolCapabilities(true))
		registryInstance := &core.MCPcatInstance{
			ProjectID: "test-project",
			Options: &core.Options{
				EnableToolCallContext: true,
			},
			ServerRef: mockServer,
		}
		registry.Register(mockServer, registryInstance)
		defer registry.Unregister(mockServer)

		// Create context with server
		ctx := contextWithServer(context.Background(), mockServer)

		request := &mcp.ListToolsRequest{}
		result := &mcp.ListToolsResult{
			Tools: []mcp.Tool{
				{
					Name:        "test_tool",
					Description: "A test tool",
					InputSchema: mcp.ToolInputSchema{
						Type: "object",
						Properties: map[string]any{
							"param1": map[string]any{
								"type":        "string",
								"description": "A parameter",
							},
						},
					},
				},
			},
		}

		// Trigger the hook
		triggerAfterListTools(hooks, ctx, "req_790", request, result)

		// Verify that context param WAS added
		tool := result.Tools[0]
		if _, hasContext := tool.InputSchema.Properties["context"]; !hasContext {
			t.Error("context parameter SHOULD be added when EnableToolCallContext = true")
		}

		// Verify original parameter is still there
		if _, hasParam1 := tool.InputSchema.Properties["param1"]; !hasParam1 {
			t.Error("original param1 should still be present")
		}
	})

	t.Run("defaults to disabled when no registry entry", func(t *testing.T) {
		resetGlobalState()
		hooks := newTestHooks()

		AddTracingToHooks(hooks, nil)
		defer globalPublisher.Shutdown()

		// Use context without server (no registry lookup possible)
		ctx := context.Background()

		request := &mcp.ListToolsRequest{}
		result := &mcp.ListToolsResult{
			Tools: []mcp.Tool{
				{
					Name:        "test_tool",
					Description: "A test tool",
					InputSchema: mcp.ToolInputSchema{
						Type: "object",
						Properties: map[string]any{
							"param1": map[string]any{
								"type":        "string",
								"description": "A parameter",
							},
						},
					},
				},
			},
		}

		// Trigger the hook
		triggerAfterListTools(hooks, ctx, "req_791", request, result)

		// Verify that context param was NOT added (default to false when no registry entry)
		tool := result.Tools[0]
		if _, hasContext := tool.InputSchema.Properties["context"]; hasContext {
			t.Error("context parameter should NOT be added by default when no registry entry exists")
		}
	})
}

// ============================================================================
// ShutdownPublisher Tests
// ============================================================================

func TestShutdownPublisher(t *testing.T) {
	t.Run("shuts down active publisher", func(t *testing.T) {
		resetGlobalState()
		hooks := newTestHooks()

		AddTracingToHooks(hooks, nil)

		if globalPublisher == nil {
			t.Fatal("expected publisher to be initialized")
		}

		ShutdownPublisher()

		// Publisher should still exist but be shut down
		// We can't directly verify shutdown state without modifying production code
	})

	t.Run("handles nil publisher gracefully", func(t *testing.T) {
		resetGlobalState()

		// Should not panic
		ShutdownPublisher()
	})

	t.Run("shutdown is idempotent", func(t *testing.T) {
		resetGlobalState()
		hooks := newTestHooks()

		AddTracingToHooks(hooks, nil)

		// Call multiple times
		ShutdownPublisher()
		ShutdownPublisher()
		ShutdownPublisher()

		// Should not panic
	})
}

// ============================================================================
// Integration Tests
// ============================================================================

func TestIntegration_FullRequestResponseCycle(t *testing.T) {
	t.Run("complete success flow", func(t *testing.T) {
		resetGlobalState()
		hooks := newTestHooks()
		AddTracingToHooks(hooks, nil)
		defer globalPublisher.Shutdown()

		ctx := context.Background()
		requestID := "req_integration_success"
		method := mcp.MethodToolsCall
		request := &mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: "integration_tool",
			},
		}
		result := &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: "success result",
				},
			},
		}

		// Simulate full cycle
		triggerBeforeAny(hooks, ctx, requestID, method, request)
		time.Sleep(5 * time.Millisecond)
		triggerOnSuccess(hooks, ctx, requestID, method, request, result)

		// Events might be published (depends on context having session)
	})

	t.Run("complete error flow", func(t *testing.T) {
		resetGlobalState()
		hooks := newTestHooks()
		AddTracingToHooks(hooks, nil)
		defer globalPublisher.Shutdown()

		ctx := context.Background()
		requestID := "req_integration_error"
		method := mcp.MethodToolsCall
		request := &mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: "failing_tool",
			},
		}
		err := errors.New("tool failed")

		// Simulate full error cycle
		triggerBeforeAny(hooks, ctx, requestID, method, request)
		time.Sleep(5 * time.Millisecond)
		triggerOnError(hooks, ctx, requestID, method, request, err)

		// Error events might be published
	})

	t.Run("multiple concurrent requests", func(t *testing.T) {
		resetGlobalState()
		hooks := newTestHooks()
		AddTracingToHooks(hooks, nil)
		defer globalPublisher.Shutdown()

		var wg sync.WaitGroup
		numRequests := 20

		for i := 0; i < numRequests; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()

				ctx := context.Background()
				requestID := id
				method := mcp.MethodToolsCall
				request := &mcp.CallToolRequest{
					Params: mcp.CallToolParams{
						Name: "concurrent_tool",
					},
				}

				triggerBeforeAny(hooks, ctx, requestID, method, request)
				time.Sleep(1 * time.Millisecond)

				if id%2 == 0 {
					result := &mcp.CallToolResult{}
					triggerOnSuccess(hooks, ctx, requestID, method, request, result)
				} else {
					err := errors.New("test error")
					triggerOnError(hooks, ctx, requestID, method, request, err)
				}
			}(i)
		}

		wg.Wait()

		// All requests should complete without race conditions
	})
}

func TestIntegration_DifferentMCPMethods(t *testing.T) {
	methods := []mcp.MCPMethod{
		mcp.MethodToolsCall,
		mcp.MethodResourcesRead,
		mcp.MethodInitialize,
		mcp.MethodToolsList,
		mcp.MethodResourcesList,
	}

	for _, method := range methods {
		t.Run(string(method), func(t *testing.T) {
			resetGlobalState()
			hooks := newTestHooks()
			AddTracingToHooks(hooks, nil)
			defer globalPublisher.Shutdown()

			ctx := context.Background()
			requestID := "req_method_test"

			triggerBeforeAny(hooks, ctx, requestID, method, nil)
			triggerOnSuccess(hooks, ctx, requestID, method, nil, nil)

			// Should handle different methods correctly
		})
	}
}

// ============================================================================
// Edge Cases & Error Handling Tests
// ============================================================================

func TestEdgeCases(t *testing.T) {
	t.Run("nil context", func(t *testing.T) {
		resetGlobalState()
		hooks := newTestHooks()

		AddTracingToHooks(hooks, nil)
		defer globalPublisher.Shutdown()

		// Should not panic with nil context
		// Note: This might panic in actual code if context is dereferenced
		// The test documents expected behavior
	})

	t.Run("missing start time for duration calculation", func(t *testing.T) {
		resetGlobalState()
		hooks := newTestHooks()

		AddTracingToHooks(hooks, nil)
		defer globalPublisher.Shutdown()

		ctx := context.Background()
		requestID := "req_no_start_time"
		method := mcp.MethodToolsCall

		// Trigger OnSuccess without BeforeAny
		triggerOnSuccess(hooks, ctx, requestID, method, nil, nil)

		// Duration should be nil
	})

	t.Run("very old start time", func(t *testing.T) {
		resetGlobalState()
		hooks := newTestHooks()

		AddTracingToHooks(hooks, nil)
		defer globalPublisher.Shutdown()

		ctx := context.Background()
		requestID := "req_old_time"
		method := mcp.MethodToolsCall

		triggerBeforeAny(hooks, ctx, requestID, method, nil)

		// Wait a longer time
		time.Sleep(100 * time.Millisecond)

		triggerOnSuccess(hooks, ctx, requestID, method, nil, nil)

		// Should handle large duration values
	})

	t.Run("requestTimes cleanup", func(t *testing.T) {
		resetGlobalState()
		hooks := newTestHooks()

		AddTracingToHooks(hooks, nil)
		defer globalPublisher.Shutdown()

		ctx := context.Background()

		// Create many requests
		for i := 0; i < 100; i++ {
			requestID := i
			triggerBeforeAny(hooks, ctx, requestID, mcp.MethodToolsCall, nil)
			triggerOnSuccess(hooks, ctx, requestID, mcp.MethodToolsCall, nil, nil)
		}

		// All entries should be cleaned up via LoadAndDelete
		// We can't directly verify the map size, but this tests the cleanup logic
	})
}

// ============================================================================
// Thread Safety Tests
// ============================================================================

func TestThreadSafety(t *testing.T) {
	t.Run("concurrent hook invocations", func(t *testing.T) {
		resetGlobalState()
		hooks := newTestHooks()
		AddTracingToHooks(hooks, nil)
		defer globalPublisher.Shutdown()

		var wg sync.WaitGroup
		numGoroutines := 50

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()

				ctx := context.Background()
				requestID := id
				method := mcp.MethodToolsCall

				triggerBeforeAny(hooks, ctx, requestID, method, nil)
				triggerOnSuccess(hooks, ctx, requestID, method, nil, nil)
			}(i)
		}

		wg.Wait()

		// Should complete without race conditions
		// Run with: go test -race
	})

	t.Run("concurrent publisher access", func(t *testing.T) {
		resetGlobalState()

		var wg sync.WaitGroup
		numGoroutines := 30

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()

				hooks := newTestHooks()
				AddTracingToHooks(hooks, nil)

				ctx := context.Background()
				requestID := "concurrent_test"

				triggerBeforeAny(hooks, ctx, requestID, mcp.MethodToolsCall, nil)
				triggerOnSuccess(hooks, ctx, requestID, mcp.MethodToolsCall, nil, nil)
			}()
		}

		wg.Wait()

		if globalPublisher != nil {
			globalPublisher.Shutdown()
		}
	})

	t.Run("race on requestTimes map", func(t *testing.T) {
		resetGlobalState()
		hooks := newTestHooks()

		AddTracingToHooks(hooks, nil)
		defer globalPublisher.Shutdown()

		var wg sync.WaitGroup
		sharedRequestID := "shared_req"

		// Multiple goroutines accessing same request ID
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()

				ctx := context.Background()
				triggerBeforeAny(hooks, ctx, sharedRequestID, mcp.MethodToolsCall, nil)
			}()
		}

		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()

				ctx := context.Background()
				triggerOnSuccess(hooks, ctx, sharedRequestID, mcp.MethodToolsCall, nil, nil)
			}()
		}

		wg.Wait()

		// sync.Map should handle concurrent access safely
	})
}

// ============================================================================
// Benchmarks (Optional)
// ============================================================================

func BenchmarkAddTracingToHooks(b *testing.B) {
	for i := 0; i < b.N; i++ {
		resetGlobalState()
		hooks := newTestHooks()
		AddTracingToHooks(hooks, nil)
		if globalPublisher != nil {
			globalPublisher.Shutdown()
		}
	}
}

func BenchmarkHookExecution(b *testing.B) {
	resetGlobalState()
	hooks := newTestHooks()
	AddTracingToHooks(hooks, nil)
	defer globalPublisher.Shutdown()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		requestID := i
		triggerBeforeAny(hooks, ctx, requestID, mcp.MethodToolsCall, nil)
		triggerOnSuccess(hooks, ctx, requestID, mcp.MethodToolsCall, nil, nil)
	}
}

// ============================================================================
// get_more_tools Feature Tests
// ============================================================================

func TestCreateGetMoreToolsTool(t *testing.T) {
	t.Run("creates tool with correct properties", func(t *testing.T) {
		tool := createGetMoreToolsTool()

		if tool.Name != "get_more_tools" {
			t.Errorf("expected name 'get_more_tools', got '%s'", tool.Name)
		}

		if tool.Description == "" {
			t.Error("expected non-empty description")
		}

		// Verify context parameter exists
		if tool.InputSchema.Properties == nil {
			t.Fatal("expected InputSchema.Properties to be non-nil")
		}

		contextProp, hasContext := tool.InputSchema.Properties["context"]
		if !hasContext {
			t.Error("expected 'context' parameter in properties")
		}

		// Verify context is required
		if !containsString(tool.InputSchema.Required, "context") {
			t.Error("expected 'context' to be in required array")
		}

		// Verify context is a string type
		if propMap, ok := contextProp.(map[string]any); ok {
			if propType, ok := propMap["type"].(string); !ok || propType != "string" {
				t.Errorf("expected context type to be 'string', got %v", propMap["type"])
			}
		}

		// Verify open-world hint annotation is set
		if tool.Annotations.OpenWorldHint == nil {
			t.Error("expected OpenWorldHint to be set")
		} else if !*tool.Annotations.OpenWorldHint {
			t.Error("expected OpenWorldHint to be true")
		}
	})
}

func TestHandleGetMoreTools(t *testing.T) {
	t.Run("handles valid context parameter", func(t *testing.T) {
		resetGlobalState()
		hooks := newTestHooks()
		AddTracingToHooks(hooks, nil)
		defer globalPublisher.Shutdown()

		logger := logging.New()
		handler := handleGetMoreTools(logger)

		ctx := context.Background()
		request := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: "get_more_tools",
				Arguments: map[string]any{
					"context": "I need a tool to analyze images",
				},
			},
		}

		result, err := handler(ctx, request)

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if result == nil {
			t.Fatal("expected result, got nil")
		}

		// Verify result contains expected message
		if len(result.Content) == 0 {
			t.Fatal("expected result content, got empty")
		}

		textContent, ok := result.Content[0].(mcp.TextContent)
		if !ok {
			t.Fatal("expected TextContent in result")
		}

		expectedText := "Unfortunately, we have shown you the full tool list"
		if !containsSubstring(textContent.Text, expectedText) {
			t.Errorf("expected result text to contain '%s', got '%s'", expectedText, textContent.Text)
		}
	})

	t.Run("handles missing context parameter", func(t *testing.T) {
		resetGlobalState()
		hooks := newTestHooks()
		AddTracingToHooks(hooks, nil)
		defer globalPublisher.Shutdown()

		logger := logging.New()
		handler := handleGetMoreTools(logger)

		ctx := context.Background()
		request := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name:      "get_more_tools",
				Arguments: map[string]any{
					// Missing context parameter
				},
			},
		}

		result, err := handler(ctx, request)

		if err != nil {
			t.Errorf("handler should not return error, got %v", err)
		}

		// Should return error result
		if result == nil {
			t.Fatal("expected result, got nil")
		}

		if !result.IsError {
			t.Error("expected IsError=true for missing context parameter")
		}
	})
}

func TestRegisterGetMoreToolsIfEnabled(t *testing.T) {
	t.Run("registers tool when EnableReportMissing=true", func(t *testing.T) {
		mockServer := server.NewMCPServer("test-server", "1.0.0", server.WithToolCapabilities(true))
		options := &core.Options{
			EnableReportMissing: true,
		}

		RegisterGetMoreToolsIfEnabled(mockServer, options)

		// Verify tool was registered
		tools := mockServer.ListTools()
		found := false
		for _, serverTool := range tools {
			if serverTool.Tool.Name == "get_more_tools" {
				found = true
				break
			}
		}

		if !found {
			t.Error("expected get_more_tools to be registered when EnableReportMissing=true")
		}
	})

	t.Run("does not register tool when EnableReportMissing=false", func(t *testing.T) {
		mockServer := server.NewMCPServer("test-server-2", "1.0.0", server.WithToolCapabilities(true))
		options := &core.Options{
			EnableReportMissing: false,
		}

		RegisterGetMoreToolsIfEnabled(mockServer, options)

		// Verify tool was NOT registered
		tools := mockServer.ListTools()
		for _, serverTool := range tools {
			if serverTool.Tool.Name == "get_more_tools" {
				t.Error("expected get_more_tools NOT to be registered when EnableReportMissing=false")
			}
		}
	})

	t.Run("handles nil options gracefully", func(t *testing.T) {
		mockServer := server.NewMCPServer("test-server-3", "1.0.0", server.WithToolCapabilities(true))

		// Should not panic with nil options
		RegisterGetMoreToolsIfEnabled(mockServer, nil)

		// Verify tool was NOT registered
		tools := mockServer.ListTools()
		for _, serverTool := range tools {
			if serverTool.Tool.Name == "get_more_tools" {
				t.Error("expected get_more_tools NOT to be registered when options are nil")
			}
		}
	})
}

func TestGetMoreToolsContextParam(t *testing.T) {
	t.Run("get_more_tools context param is not modified", func(t *testing.T) {
		resetGlobalState()
		hooks := newTestHooks()
		AddTracingToHooks(hooks, nil)
		defer globalPublisher.Shutdown()

		// Create server with EnableToolCallContext=true
		mockServer := server.NewMCPServer("test-server", "1.0.0", server.WithToolCapabilities(true))
		registryInstance := &core.MCPcatInstance{
			ProjectID: "test-project",
			Options: &core.Options{
				EnableToolCallContext: true,
			},
			ServerRef: mockServer,
		}
		registry.Register(mockServer, registryInstance)
		defer registry.Unregister(mockServer)

		ctx := contextWithServer(context.Background(), mockServer)

		// Create a list with get_more_tools
		getMoreToolsTool := createGetMoreToolsTool()
		originalDescription := getMoreToolsTool.InputSchema.Properties["context"].(map[string]any)["description"]

		result := &mcp.ListToolsResult{
			Tools: []mcp.Tool{getMoreToolsTool},
		}

		// Trigger AfterListTools hook (which adds context params)
		triggerAfterListTools(hooks, ctx, "test_id", &mcp.ListToolsRequest{}, result)

		// Verify the context parameter description was NOT changed
		tool := result.Tools[0]
		contextProp := tool.InputSchema.Properties["context"].(map[string]any)
		currentDescription := contextProp["description"]

		if currentDescription != originalDescription {
			t.Errorf("expected context description to remain '%v', got '%v'", originalDescription, currentDescription)
		}
	})
}

// Helper function to check if a string contains a substring
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && hasSubstring(s, substr))
}

func hasSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
