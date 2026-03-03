package mcpcat

import (
	"testing"

	"github.com/mcpcat/mcpcat-go-sdk/internal/registry"
)

func TestTrack_NilServer(t *testing.T) {
	err := Track(nil, "proj_id", nil)
	if err == nil {
		t.Error("Expected error when tracking with nil server")
	}
}

func TestTrack_EmptyProjectID(t *testing.T) {
	// Can't easily create a real *server.MCPServer in internal tests
	// without importing mcp-go, so just test nil + empty project
	err := Track(nil, "", nil)
	if err == nil {
		t.Error("Expected error for nil server or empty project ID")
	}
}

func TestTrack_WithHooksOption(t *testing.T) {
	// Verify that passing Hooks via Options doesn't error
	// (can't create a real server here without full mcp-go setup)
	err := Track(nil, "proj_id", &Options{Hooks: nil})
	if err == nil {
		t.Error("Expected error for nil server even with options")
	}
}

func TestGetMCPcat_NotRegistered(t *testing.T) {
	mockServer := &struct{ id string }{id: "test"}
	result := GetMCPcat(mockServer)

	if result != nil {
		t.Error("Expected nil when getting MCPcat for unregistered server")
	}
}

func TestGetMCPcat_NilServer(t *testing.T) {
	result := GetMCPcat(nil)

	if result != nil {
		t.Error("Expected nil when getting MCPcat for nil server")
	}
}

func TestUnregisterServer(t *testing.T) {
	mockServer := &struct{ id string }{id: "test-server"}

	// This should not panic even if server is not registered
	UnregisterServer(mockServer)

	// Verify the server is not in registry
	instance := registry.Get(mockServer)
	if instance != nil {
		t.Error("Expected server to not be in registry after unregister")
	}
}

func TestUnregisterServer_NilServer(t *testing.T) {
	// Test that UnregisterServer doesn't panic with nil
	UnregisterServer(nil)
}

func TestShutdown(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Shutdown panicked: %v", r)
		}
	}()

	Shutdown()
}

func TestShutdown_MultipleCalls(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Multiple Shutdown calls panicked: %v", r)
		}
	}()

	Shutdown()
	Shutdown()
	Shutdown()
}

func TestMCPcat_Struct(t *testing.T) {
	mcpcat := &MCPcat{
		ProjectID: "proj_test",
		Options:   DefaultOptions(),
		serverRef: "test-ref",
	}

	if mcpcat.ProjectID != "proj_test" {
		t.Errorf("Expected ProjectID 'proj_test', got '%s'", mcpcat.ProjectID)
	}

	if mcpcat.Options.EnableReportMissing != true {
		t.Error("Expected EnableReportMissing to be true from DefaultOptions")
	}

	if mcpcat.serverRef != "test-ref" {
		t.Errorf("Expected serverRef 'test-ref', got '%v'", mcpcat.serverRef)
	}
}

func TestGetMCPcat_AfterManualRegistration(t *testing.T) {
	mockServer := &struct{ id string }{id: "test-server"}
	projectID := "proj_manual_test"
	opts := DefaultOptions()

	instance := registry.Get(mockServer)
	if instance != nil {
		registry.Unregister(mockServer)
	}

	registry.Register(mockServer, &MCPcatInstance{
		ProjectID: projectID,
		Options:   &opts,
		ServerRef: mockServer,
	})

	result := GetMCPcat(mockServer)
	if result == nil {
		t.Fatal("Expected non-nil MCPcat after manual registration")
	}

	if result.ProjectID != projectID {
		t.Errorf("Expected ProjectID '%s', got '%s'", projectID, result.ProjectID)
	}

	if !result.Options.EnableReportMissing {
		t.Error("Expected EnableReportMissing to be true")
	}

	registry.Unregister(mockServer)
}

func TestGetMCPcat_WithNilOptions(t *testing.T) {
	mockServer := &struct{ id string }{id: "test-server-nil-opts"}

	registry.Register(mockServer, &MCPcatInstance{
		ProjectID: "proj_test",
		Options:   nil,
		ServerRef: mockServer,
	})

	result := GetMCPcat(mockServer)
	if result != nil {
		t.Error("Expected nil when registry instance has nil options")
	}

	registry.Unregister(mockServer)
}
