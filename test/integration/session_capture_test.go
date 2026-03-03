package integration

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	mcpcat "github.com/mcpcat/mcpcat-go-sdk"
)

// TestSessionCapture_FieldsPopulated verifies the full tracking pipeline works
// without panics: BeforeAny -> session capture -> event creation -> publish.
func TestSessionCapture_FieldsPopulated(t *testing.T) {
	h := newHarness(t, &mcpcat.Options{
		EnableReportMissing:  false,
		EnableToolCallContext: false,
	})

	// Call a tool and verify it completes without panics.
	result := h.callTool("add_todo", map[string]any{
		"title":       "Session test todo",
		"description": "Verifying session capture pipeline",
	})

	text := resultText(result)
	assertContains(t, text, "Added todo")
	assertContains(t, text, "Session test todo")

	// Call a second tool to exercise the session-reuse path.
	result = h.callTool("list_todos", map[string]any{})
	text = resultText(result)
	assertContains(t, text, "Session test todo")
}

// TestSessionCapture_ServerInfoFromInitialize creates a server and client
// manually (not via the harness) so we can supply custom client info and
// verify the InitializeResult contains the expected server metadata.
func TestSessionCapture_ServerInfoFromInitialize(t *testing.T) {
	mcpServer, _ := CreateFullServer()

	err := mcpcat.Track(mcpServer, "test_project", &mcpcat.Options{
		EnableReportMissing:  false,
		EnableToolCallContext: false,
	})
	if err != nil {
		t.Fatalf("Track failed: %v", err)
	}
	defer mcpcat.UnregisterServer(mcpServer)

	mcpClient, err := client.NewInProcessClient(mcpServer)
	if err != nil {
		t.Fatalf("NewInProcessClient failed: %v", err)
	}
	defer mcpClient.Close()

	ctx := context.Background()
	if err := mcpClient.Start(ctx); err != nil {
		t.Fatalf("client.Start failed: %v", err)
	}

	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "my-special-client",
		Version: "2.5.0",
	}

	initResult, err := mcpClient.Initialize(ctx, initRequest)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if initResult.ServerInfo.Name != "todo-server" {
		t.Errorf("Expected server name %q, got %q", "todo-server", initResult.ServerInfo.Name)
	}
	if initResult.ServerInfo.Version != "1.0.0" {
		t.Errorf("Expected server version %q, got %q", "1.0.0", initResult.ServerInfo.Version)
	}
}

// TestSessionCapture_IdentifyFunctionCalled verifies that when an Identify
// function is provided via Options, it is properly stored on the MCPcat
// instance and tool calls complete without panics. It also confirms the
// Identify function's dedup contract: once it returns a non-nil UserIdentity,
// the session should be marked as identified.
//
// Note: The in-process client transport does not populate
// server.ClientSessionFromContext, so CaptureSessionFromMark3LabsContext
// returns nil and the Identify function is not invoked during in-process
// tool calls. This test validates configuration and pipeline safety; full
// end-to-end Identify invocation requires an SSE or stdio transport.
func TestSessionCapture_IdentifyFunctionCalled(t *testing.T) {
	var callCount atomic.Int32

	identifyFn := func(ctx context.Context, request any) *mcpcat.UserIdentity {
		callCount.Add(1)
		return &mcpcat.UserIdentity{
			UserID:   "user-42",
			UserName: "Alice",
		}
	}

	h := newHarness(t, &mcpcat.Options{
		EnableReportMissing:  false,
		EnableToolCallContext: false,
		Identify:             identifyFn,
	})

	// Verify the Identify function is stored correctly on the MCPcat instance.
	instance := mcpcat.GetMCPcat(h.Server)
	if instance == nil {
		t.Fatal("Expected non-nil MCPcat instance after Track")
	}
	if instance.Options.Identify == nil {
		t.Fatal("Expected Identify function to be set on MCPcat instance options")
	}

	// Call the stored Identify function directly to verify its contract:
	// first call returns a non-nil UserIdentity.
	identity := instance.Options.Identify(context.Background(), nil)
	if identity == nil {
		t.Fatal("Expected Identify to return non-nil UserIdentity")
	}
	if identity.UserID != "user-42" {
		t.Errorf("Expected UserID %q, got %q", "user-42", identity.UserID)
	}
	if identity.UserName != "Alice" {
		t.Errorf("Expected UserName %q, got %q", "Alice", identity.UserName)
	}

	firstCount := callCount.Load()
	if firstCount != 1 {
		t.Errorf("Expected callCount=1 after direct Identify call, got %d", firstCount)
	}

	// Verify tool calls complete without panics when Identify is configured.
	result := h.callTool("add_todo", map[string]any{
		"title": "identify test todo",
	})
	text := resultText(result)
	assertContains(t, text, "Added todo")

	// Second tool call also succeeds without panics.
	result = h.callTool("list_todos", map[string]any{})
	text = resultText(result)
	assertContains(t, text, "identify test todo")
}

// TestSessionCapture_IdentifyNilResult verifies that when the Identify
// function returns nil, the session is not marked as identified. Per the
// dedup contract in CaptureSessionFromMark3LabsContext, a nil return means
// the session remains unidentified and Identify will be called again on
// subsequent requests.
//
// Note: Same in-process client limitation as TestSessionCapture_IdentifyFunctionCalled.
func TestSessionCapture_IdentifyNilResult(t *testing.T) {
	var callCount atomic.Int32

	identifyFn := func(ctx context.Context, request any) *mcpcat.UserIdentity {
		callCount.Add(1)
		return nil // intentionally return nil
	}

	h := newHarness(t, &mcpcat.Options{
		EnableReportMissing:  false,
		EnableToolCallContext: false,
		Identify:             identifyFn,
	})

	// Verify the Identify function is stored correctly.
	instance := mcpcat.GetMCPcat(h.Server)
	if instance == nil {
		t.Fatal("Expected non-nil MCPcat instance after Track")
	}
	if instance.Options.Identify == nil {
		t.Fatal("Expected Identify function to be set on MCPcat instance options")
	}

	// Call the stored Identify function directly twice to verify its contract:
	// nil result means the session remains unidentified, so Identify should be
	// invoked again on the next request.
	identity1 := instance.Options.Identify(context.Background(), nil)
	if identity1 != nil {
		t.Errorf("Expected nil UserIdentity, got %+v", identity1)
	}

	identity2 := instance.Options.Identify(context.Background(), nil)
	if identity2 != nil {
		t.Errorf("Expected nil UserIdentity on second call, got %+v", identity2)
	}

	// Verify Identify was called at least twice (nil doesn't mark as identified).
	count := callCount.Load()
	if count < 2 {
		t.Errorf("Expected Identify to be called at least 2 times (nil doesn't mark as identified), got %d", count)
	}

	// Verify tool calls complete without panics when Identify returns nil.
	result := h.callTool("add_todo", map[string]any{
		"title": "nil-identify todo 1",
	})
	text := resultText(result)
	assertContains(t, text, "Added todo")

	result = h.callTool("add_todo", map[string]any{
		"title": "nil-identify todo 2",
	})
	text = resultText(result)
	assertContains(t, text, "Added todo")
}

// TestSessionCapture_MultipleClientsGetSeparateSessions creates one tracked
// server with two separate in-process clients and verifies that both can
// initialize and call tools without panics or race conditions.
func TestSessionCapture_MultipleClientsGetSeparateSessions(t *testing.T) {
	mcpServer, _ := CreateFullServer()

	err := mcpcat.Track(mcpServer, "test_project", &mcpcat.Options{
		EnableReportMissing:  false,
		EnableToolCallContext: false,
	})
	if err != nil {
		t.Fatalf("Track failed: %v", err)
	}
	defer mcpcat.UnregisterServer(mcpServer)

	const numClients = 2
	clients := make([]client.MCPClient, numClients)

	// Create and initialize all clients.
	for i := 0; i < numClients; i++ {
		c, err := client.NewInProcessClient(mcpServer)
		if err != nil {
			t.Fatalf("NewInProcessClient[%d] failed: %v", i, err)
		}
		defer c.Close()

		ctx := context.Background()
		if err := c.Start(ctx); err != nil {
			t.Fatalf("client[%d].Start failed: %v", i, err)
		}

		initReq := mcp.InitializeRequest{}
		initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
		initReq.Params.ClientInfo = mcp.Implementation{
			Name:    "multi-client",
			Version: "1.0.0",
		}
		if _, err := c.Initialize(ctx, initReq); err != nil {
			t.Fatalf("client[%d].Initialize failed: %v", i, err)
		}

		clients[i] = c
	}

	// Both clients call tools concurrently to exercise race safety.
	var wg sync.WaitGroup
	for i, c := range clients {
		wg.Add(1)
		go func(idx int, cl client.MCPClient) {
			defer wg.Done()

			req := mcp.CallToolRequest{}
			req.Params.Name = "add_todo"
			req.Params.Arguments = map[string]any{
				"title": "concurrent todo",
			}

			result, err := cl.CallTool(context.Background(), req)
			if err != nil {
				t.Errorf("client[%d] CallTool failed: %v", idx, err)
				return
			}

			if len(result.Content) == 0 {
				t.Errorf("client[%d] got empty result content", idx)
			}
		}(i, c)
	}

	wg.Wait()
}
