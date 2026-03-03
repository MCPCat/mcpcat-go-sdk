package integration

import (
	"context"
	"io"
	"log"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	mcpcat "github.com/mcpcat/mcpcat-go-sdk"
)

// setupStdio creates a real stdio-based MCP client connected to a tracked
// server via io.Pipe pairs. Unlike in-process clients, the stdio transport
// populates server.ClientSessionFromContext(ctx), enabling session capture
// and identify functions to fire.
func setupStdio(t *testing.T, opts *mcpcat.Options) *client.Client {
	t.Helper()

	mcpServer, _ := CreateFullServer()

	err := mcpcat.Track(mcpServer, "test_project", opts)
	if err != nil {
		t.Fatalf("setupStdio: Track failed: %v", err)
	}

	// Two io.Pipe pairs create bidirectional communication:
	//   client writes -> clientToServerWriter -> clientToServerReader -> server reads
	//   server writes -> serverToClientWriter -> serverToClientReader -> client reads
	clientToServerReader, clientToServerWriter := io.Pipe()
	serverToClientReader, serverToClientWriter := io.Pipe()

	stdioServer := server.NewStdioServer(mcpServer)
	stdioServer.SetErrorLogger(log.New(io.Discard, "", 0))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	// Start the server in a goroutine; capture its exit error.
	serverDone := make(chan error, 1)
	go func() {
		serverDone <- stdioServer.Listen(ctx, clientToServerReader, serverToClientWriter)
	}()

	// Build the client side of the pipe.
	trans := transport.NewIO(serverToClientReader, clientToServerWriter, nil)

	// Explicitly start the transport: client.Start skips transport.Start for
	// *transport.Stdio (it assumes the factory already started it), but when
	// wiring manually via NewIO + NewClient we must start it ourselves so the
	// response-reading goroutine is running before we send the first request.
	if err := trans.Start(ctx); err != nil {
		cancel()
		clientToServerWriter.Close()
		serverToClientWriter.Close()
		mcpcat.UnregisterServer(mcpServer)
		t.Fatalf("setupStdio: trans.Start failed: %v", err)
	}

	mcpClient := client.NewClient(trans)

	if err := mcpClient.Start(ctx); err != nil {
		cancel()
		clientToServerWriter.Close()
		serverToClientWriter.Close()
		mcpcat.UnregisterServer(mcpServer)
		t.Fatalf("setupStdio: client.Start failed: %v", err)
	}

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "e2e-stdio-client",
		Version: "1.0.0",
	}

	_, err = mcpClient.Initialize(ctx, initReq)
	if err != nil {
		cancel()
		clientToServerWriter.Close()
		serverToClientWriter.Close()
		mcpcat.UnregisterServer(mcpServer)
		t.Fatalf("setupStdio: Initialize failed: %v", err)
	}

	t.Cleanup(func() {
		// 1. Cancel context -- signals the server to stop.
		cancel()
		// 2. Close pipe writers -- unblocks any blocked reads.
		clientToServerWriter.Close()
		serverToClientWriter.Close()
		// 3. Wait for the server goroutine to exit.
		<-serverDone
		// 4. Close the client and unregister the server.
		mcpClient.Close()
		mcpcat.UnregisterServer(mcpServer)
	})

	return mcpClient
}

// TestStdio_FullPipeline verifies a basic tool call works end-to-end over a
// real stdio transport.
func TestStdio_FullPipeline(t *testing.T) {
	opts := &mcpcat.Options{
		EnableReportMissing:   false,
		EnableToolCallContext: false,
	}

	mcpClient := setupStdio(t, opts)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req := mcp.CallToolRequest{}
	req.Params.Name = "add_todo"
	req.Params.Arguments = map[string]any{
		"title": "Stdio e2e todo",
	}

	result, err := mcpClient.CallTool(ctx, req)
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}

	if result.IsError {
		t.Fatalf("CallTool returned error result: %s", resultText(result))
	}

	assertContains(t, resultText(result), "Stdio e2e todo")
}

// TestStdio_Identify uses ordered subtests to verify both invocation and dedup
// of the Identify callback over the stdio transport.
//
// The mcp-go stdio transport uses a package-level singleton session with a
// fixed session ID ("stdio"). The mcpcat session cache is keyed by that ID,
// so once IdentifyActorGivenId is set it persists across server instances in
// the same process. Using sequential subtests within a single parent test
// guarantees execution order: Invoked runs first (on a clean session), then
// Dedup verifies the callback does not fire again.
func TestStdio_Identify(t *testing.T) {
	var identifyCount atomic.Int32

	// Subtest 1: Identify fires on the first tool call.
	t.Run("Invoked", func(t *testing.T) {
		opts := &mcpcat.Options{
			EnableReportMissing:   false,
			EnableToolCallContext: false,
			Identify: func(ctx context.Context, request any) *mcpcat.UserIdentity {
				identifyCount.Add(1)
				return &mcpcat.UserIdentity{
					UserID:   "stdio-user-1",
					UserName: "Stdio Test User",
				}
			},
		}

		mcpClient := setupStdio(t, opts)

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
			t.Error("expected Identify to be called at least once over stdio transport, but it was not")
		}
	})

	countAfterInvoked := identifyCount.Load()

	// Subtest 2: Identify does NOT fire again (session already identified).
	t.Run("Dedup", func(t *testing.T) {
		opts := &mcpcat.Options{
			EnableReportMissing:   false,
			EnableToolCallContext: false,
			Identify: func(ctx context.Context, request any) *mcpcat.UserIdentity {
				identifyCount.Add(1)
				return &mcpcat.UserIdentity{
					UserID:   "stdio-dedup-user",
					UserName: "Dedup User",
				}
			},
		}

		mcpClient := setupStdio(t, opts)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		req := mcp.CallToolRequest{}
		req.Params.Name = "add_todo"
		req.Params.Arguments = map[string]any{
			"title": "Should not re-identify",
		}

		_, err := mcpClient.CallTool(ctx, req)
		if err != nil {
			t.Fatalf("CallTool failed: %v", err)
		}

		if identifyCount.Load() != countAfterInvoked {
			t.Errorf("expected Identify count to stay at %d (dedup), but got %d",
				countAfterInvoked, identifyCount.Load())
		}
	})
}

// TestStdio_ServerInfo verifies that the server name and version returned
// during initialization match what was configured.
func TestStdio_ServerInfo(t *testing.T) {
	mcpServer, _ := CreateFullServer()

	opts := &mcpcat.Options{
		EnableReportMissing:   false,
		EnableToolCallContext: false,
	}
	err := mcpcat.Track(mcpServer, "test_project", opts)
	if err != nil {
		t.Fatalf("Track failed: %v", err)
	}

	clientToServerReader, clientToServerWriter := io.Pipe()
	serverToClientReader, serverToClientWriter := io.Pipe()

	stdioServer := server.NewStdioServer(mcpServer)
	stdioServer.SetErrorLogger(log.New(io.Discard, "", 0))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	serverDone := make(chan error, 1)
	go func() {
		serverDone <- stdioServer.Listen(ctx, clientToServerReader, serverToClientWriter)
	}()

	trans := transport.NewIO(serverToClientReader, clientToServerWriter, nil)

	if err := trans.Start(ctx); err != nil {
		cancel()
		clientToServerWriter.Close()
		serverToClientWriter.Close()
		mcpcat.UnregisterServer(mcpServer)
		t.Fatalf("trans.Start failed: %v", err)
	}

	mcpClient := client.NewClient(trans)

	if err := mcpClient.Start(ctx); err != nil {
		cancel()
		clientToServerWriter.Close()
		serverToClientWriter.Close()
		mcpcat.UnregisterServer(mcpServer)
		t.Fatalf("client.Start failed: %v", err)
	}

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "info-check-stdio-client",
		Version: "2.0.0",
	}

	initResult, err := mcpClient.Initialize(ctx, initReq)
	if err != nil {
		cancel()
		clientToServerWriter.Close()
		serverToClientWriter.Close()
		mcpcat.UnregisterServer(mcpServer)
		t.Fatalf("Initialize failed: %v", err)
	}

	t.Cleanup(func() {
		cancel()
		clientToServerWriter.Close()
		serverToClientWriter.Close()
		<-serverDone
		mcpClient.Close()
		mcpcat.UnregisterServer(mcpServer)
	})

	if initResult.ServerInfo.Name != "todo-server" {
		t.Errorf("expected ServerInfo.Name=%q, got %q", "todo-server", initResult.ServerInfo.Name)
	}
	if initResult.ServerInfo.Version != "1.0.0" {
		t.Errorf("expected ServerInfo.Version=%q, got %q", "1.0.0", initResult.ServerInfo.Version)
	}
}
