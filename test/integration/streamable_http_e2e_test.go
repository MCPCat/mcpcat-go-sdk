package integration

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	mcpcat "github.com/mcpcat/mcpcat-go-sdk"
)

// setupStreamableHTTP creates a real HTTP-based MCP client that exercises the
// full session-capture code path (unlike in-process clients).
func setupStreamableHTTP(t *testing.T, opts *mcpcat.Options) *client.Client {
	t.Helper()

	mcpServer, _ := CreateFullServer()

	err := mcpcat.Track(mcpServer, "test_project", opts)
	if err != nil {
		t.Fatalf("setupStreamableHTTP: Track failed: %v", err)
	}

	httpServer := server.NewTestStreamableHTTPServer(mcpServer)

	mcpClient, err := client.NewStreamableHttpClient(httpServer.URL)
	if err != nil {
		httpServer.Close()
		mcpcat.UnregisterServer(mcpServer)
		t.Fatalf("setupStreamableHTTP: NewStreamableHttpClient failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	if err := mcpClient.Start(ctx); err != nil {
		cancel()
		mcpClient.Close()
		httpServer.Close()
		mcpcat.UnregisterServer(mcpServer)
		t.Fatalf("setupStreamableHTTP: client.Start failed: %v", err)
	}

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "e2e-http-client",
		Version: "1.0.0",
	}

	_, err = mcpClient.Initialize(ctx, initReq)
	if err != nil {
		cancel()
		mcpClient.Close()
		httpServer.Close()
		mcpcat.UnregisterServer(mcpServer)
		t.Fatalf("setupStreamableHTTP: Initialize failed: %v", err)
	}

	t.Cleanup(func() {
		mcpClient.Close()
		httpServer.Close()
		cancel()
		mcpcat.UnregisterServer(mcpServer)
	})

	return mcpClient
}

// TestStreamableHTTP_FullPipeline verifies a basic tool call works end-to-end
// over a real HTTP transport.
func TestStreamableHTTP_FullPipeline(t *testing.T) {
	opts := &mcpcat.Options{
		EnableReportMissing:  false,
		EnableToolCallContext: false,
	}

	mcpClient := setupStreamableHTTP(t, opts)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req := mcp.CallToolRequest{}
	req.Params.Name = "add_todo"
	req.Params.Arguments = map[string]any{
		"title": "HTTP e2e todo",
	}

	result, err := mcpClient.CallTool(ctx, req)
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}

	if result.IsError {
		t.Fatalf("CallTool returned error result: %s", resultText(result))
	}

	assertContains(t, resultText(result), "HTTP e2e todo")
}

// TestStreamableHTTP_IdentifyInvoked proves that the Identify callback fires
// when a real HTTP session is present (unlike in-process tests where it is
// skipped because ClientSessionFromContext returns nil).
func TestStreamableHTTP_IdentifyInvoked(t *testing.T) {
	var identifyCount atomic.Int32

	opts := &mcpcat.Options{
		EnableReportMissing:  false,
		EnableToolCallContext: false,
		Identify: func(ctx context.Context, request any) *mcpcat.UserIdentity {
			identifyCount.Add(1)
			return &mcpcat.UserIdentity{
				UserID:   "http-user-1",
				UserName: "HTTP Test User",
			}
		},
	}

	mcpClient := setupStreamableHTTP(t, opts)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req := mcp.CallToolRequest{}
	req.Params.Name = "add_todo"
	req.Params.Arguments = map[string]any{
		"title": "Trigger identify",
	}

	_, err := mcpClient.CallTool(ctx, req)
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}

	if identifyCount.Load() <= 0 {
		t.Error("expected Identify to be called at least once over HTTP transport, but it was not")
	}
}

// TestStreamableHTTP_IdentifyDedup verifies that Identify is called on the
// first request but not on subsequent requests within the same session.
func TestStreamableHTTP_IdentifyDedup(t *testing.T) {
	var identifyCount atomic.Int32

	opts := &mcpcat.Options{
		EnableReportMissing:  false,
		EnableToolCallContext: false,
		Identify: func(ctx context.Context, request any) *mcpcat.UserIdentity {
			identifyCount.Add(1)
			return &mcpcat.UserIdentity{
				UserID:   "http-dedup-user",
				UserName: "Dedup User",
			}
		},
	}

	mcpClient := setupStreamableHTTP(t, opts)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// First call: should trigger Identify
	req1 := mcp.CallToolRequest{}
	req1.Params.Name = "add_todo"
	req1.Params.Arguments = map[string]any{
		"title": "First call",
	}

	_, err := mcpClient.CallTool(ctx, req1)
	if err != nil {
		t.Fatalf("first CallTool failed: %v", err)
	}

	countAfterFirst := identifyCount.Load()
	if countAfterFirst <= 0 {
		t.Fatal("expected Identify to be called after first tool call, but it was not")
	}

	// Second call: Identify should NOT fire again (session already identified)
	req2 := mcp.CallToolRequest{}
	req2.Params.Name = "list_todos"
	req2.Params.Arguments = map[string]any{}

	_, err = mcpClient.CallTool(ctx, req2)
	if err != nil {
		t.Fatalf("second CallTool failed: %v", err)
	}

	countAfterSecond := identifyCount.Load()
	if countAfterSecond != countAfterFirst {
		t.Errorf("expected Identify count to stay at %d after second call, but got %d (dedup failed)",
			countAfterFirst, countAfterSecond)
	}
}

// TestStreamableHTTP_ServerInfo verifies that the server name and version
// returned during initialization match what was configured.
func TestStreamableHTTP_ServerInfo(t *testing.T) {
	mcpServer, _ := CreateFullServer()

	opts := &mcpcat.Options{
		EnableReportMissing:  false,
		EnableToolCallContext: false,
	}
	err := mcpcat.Track(mcpServer, "test_project", opts)
	if err != nil {
		t.Fatalf("Track failed: %v", err)
	}

	httpServer := server.NewTestStreamableHTTPServer(mcpServer)

	mcpClient, err := client.NewStreamableHttpClient(httpServer.URL)
	if err != nil {
		httpServer.Close()
		mcpcat.UnregisterServer(mcpServer)
		t.Fatalf("NewStreamableHttpClient failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	if err := mcpClient.Start(ctx); err != nil {
		cancel()
		mcpClient.Close()
		httpServer.Close()
		mcpcat.UnregisterServer(mcpServer)
		t.Fatalf("client.Start failed: %v", err)
	}

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "info-check-client",
		Version: "3.0.0",
	}

	initResult, err := mcpClient.Initialize(ctx, initReq)
	if err != nil {
		cancel()
		mcpClient.Close()
		httpServer.Close()
		mcpcat.UnregisterServer(mcpServer)
		t.Fatalf("Initialize failed: %v", err)
	}

	t.Cleanup(func() {
		mcpClient.Close()
		httpServer.Close()
		cancel()
		mcpcat.UnregisterServer(mcpServer)
	})

	if initResult.ServerInfo.Name != "todo-server" {
		t.Errorf("expected ServerInfo.Name=%q, got %q", "todo-server", initResult.ServerInfo.Name)
	}
	if initResult.ServerInfo.Version != "1.0.0" {
		t.Errorf("expected ServerInfo.Version=%q, got %q", "1.0.0", initResult.ServerInfo.Version)
	}
}
