package integration

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	mcpcat "github.com/mcpcat/mcpcat-go-sdk"
)

// TestTrackInternal_DefaultOptionsUsedWhenNil verifies that when Track is called
// with nil options, the default options are applied (EnableReportMissing=true,
// EnableToolCallContext=true).
func TestTrackInternal_DefaultOptionsUsedWhenNil(t *testing.T) {
	mcpServer := server.NewMCPServer("test-defaults", "1.0.0", server.WithToolCapabilities(true))

	err := mcpcat.Track(mcpServer, "proj_defaults", nil)
	if err != nil {
		t.Fatalf("Track failed: %v", err)
	}
	defer mcpcat.UnregisterServer(mcpServer)

	instance := mcpcat.GetMCPcat(mcpServer)
	if instance == nil {
		t.Fatal("Expected non-nil MCPcat instance after Track with nil options")
	}

	if !instance.Options.EnableReportMissing {
		t.Error("Expected EnableReportMissing=true from default options, got false")
	}

	if !instance.Options.EnableToolCallContext {
		t.Error("Expected EnableToolCallContext=true from default options, got false")
	}
}

// TestTrackInternal_CustomOptionsPreserved verifies that custom option values
// passed to Track are preserved in the registered MCPcat instance.
func TestTrackInternal_CustomOptionsPreserved(t *testing.T) {
	mcpServer := server.NewMCPServer("test-custom-opts", "1.0.0", server.WithToolCapabilities(true))

	opts := &mcpcat.Options{
		EnableReportMissing:   false,
		EnableToolCallContext: false,
		Debug:                 true,
	}

	err := mcpcat.Track(mcpServer, "proj_custom", opts)
	if err != nil {
		t.Fatalf("Track failed: %v", err)
	}
	defer mcpcat.UnregisterServer(mcpServer)

	instance := mcpcat.GetMCPcat(mcpServer)
	if instance == nil {
		t.Fatal("Expected non-nil MCPcat instance after Track with custom options")
	}

	if instance.Options.EnableReportMissing {
		t.Error("Expected EnableReportMissing=false (custom), got true")
	}

	if instance.Options.EnableToolCallContext {
		t.Error("Expected EnableToolCallContext=false (custom), got true")
	}

	if !instance.Options.Debug {
		t.Error("Expected Debug=true (custom), got false")
	}
}

// TestTrackInternal_RegistersGetMoreToolsWhenEnabled verifies that when
// EnableReportMissing=true, the "get_more_tools" tool is registered on the server.
func TestTrackInternal_RegistersGetMoreToolsWhenEnabled(t *testing.T) {
	mcpServer := server.NewMCPServer("test-get-more-tools", "1.0.0", server.WithToolCapabilities(true))

	opts := &mcpcat.Options{
		EnableReportMissing: true,
	}

	err := mcpcat.Track(mcpServer, "proj_get_more", opts)
	if err != nil {
		t.Fatalf("Track failed: %v", err)
	}
	defer mcpcat.UnregisterServer(mcpServer)

	tools := mcpServer.ListTools()
	found := false
	for _, serverTool := range tools {
		if serverTool.Tool.Name == "get_more_tools" {
			found = true
			break
		}
	}

	if !found {
		t.Error("Expected 'get_more_tools' to be registered when EnableReportMissing=true")
	}
}

// TestTrackInternal_DoesNotRegisterGetMoreToolsWhenDisabled verifies that when
// EnableReportMissing=false, the "get_more_tools" tool is NOT registered.
func TestTrackInternal_DoesNotRegisterGetMoreToolsWhenDisabled(t *testing.T) {
	mcpServer := server.NewMCPServer("test-no-get-more-tools", "1.0.0", server.WithToolCapabilities(true))

	opts := &mcpcat.Options{
		EnableReportMissing: false,
	}

	err := mcpcat.Track(mcpServer, "proj_no_get_more", opts)
	if err != nil {
		t.Fatalf("Track failed: %v", err)
	}
	defer mcpcat.UnregisterServer(mcpServer)

	tools := mcpServer.ListTools()
	for _, serverTool := range tools {
		if serverTool.Tool.Name == "get_more_tools" {
			t.Error("Expected 'get_more_tools' NOT to be registered when EnableReportMissing=false")
		}
	}
}

// TestTrackInternal_HooksMergeWithExisting verifies that when the user provides
// their own hooks with a BeforeAny callback, Track merges its own hooks into the
// same hooks instance, resulting in at least 2 OnBeforeAny entries.
func TestTrackInternal_HooksMergeWithExisting(t *testing.T) {
	mcpServer := server.NewMCPServer("test-hooks-merge", "1.0.0", server.WithToolCapabilities(true))

	hooks := &server.Hooks{}
	hooks.AddBeforeAny(func(ctx context.Context, id any, method mcp.MCPMethod, message any) {
		// User-provided BeforeAny callback (no-op for testing)
	})

	opts := &mcpcat.Options{
		Hooks:               hooks,
		EnableReportMissing: false,
	}

	err := mcpcat.Track(mcpServer, "proj_hooks_merge", opts)
	if err != nil {
		t.Fatalf("Track failed: %v", err)
	}
	defer mcpcat.UnregisterServer(mcpServer)

	// The user added 1 BeforeAny callback, and MCPcat's AddTracingToHooks adds
	// another. So we expect at least 2 entries.
	if len(hooks.OnBeforeAny) < 2 {
		t.Errorf("Expected at least 2 OnBeforeAny callbacks (user's + mcpcat's), got %d", len(hooks.OnBeforeAny))
	}
}

// TestTrackInternal_RegistryContainsCorrectInstance verifies that after Track,
// GetMCPcat returns an instance with the correct ProjectID.
func TestTrackInternal_RegistryContainsCorrectInstance(t *testing.T) {
	mcpServer := server.NewMCPServer("test-registry", "1.0.0", server.WithToolCapabilities(true))

	projectID := "proj_registry_check_123"
	err := mcpcat.Track(mcpServer, projectID, nil)
	if err != nil {
		t.Fatalf("Track failed: %v", err)
	}
	defer mcpcat.UnregisterServer(mcpServer)

	instance := mcpcat.GetMCPcat(mcpServer)
	if instance == nil {
		t.Fatal("Expected non-nil MCPcat instance from GetMCPcat")
	}

	if instance.ProjectID != projectID {
		t.Errorf("Expected ProjectID %q, got %q", projectID, instance.ProjectID)
	}
}
