package mcpcat

import (
	"testing"

	"github.com/mcpcat/mcpcat-go-sdk/internal/registry"
)

// Mock server types for testing
type unsupportedServer struct{}

func TestSetupTracking_NilServer(t *testing.T) {
	_, err := SetupTracking(nil, nil, nil, nil)
	if err == nil {
		t.Error("Expected error when setting up tracking with nil server")
	}

	expectedSubstring := "unsupported server type 'unknown'"
	if !contains(err.Error(), expectedSubstring) {
		t.Errorf("Expected error to contain '%s', got: %v", expectedSubstring, err)
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && stringContains(s, substr))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestSetupTracking_UnsupportedServerType(t *testing.T) {
	mockServer := &unsupportedServer{}
	_, err := SetupTracking(mockServer, nil, nil, nil)

	if err == nil {
		t.Error("Expected error when setting up tracking with unsupported server type")
	}

	expectedSubstring := "unsupported server type 'unknown'"
	if !contains(err.Error(), expectedSubstring) {
		t.Errorf("Expected error to contain '%s', got: %v", expectedSubstring, err)
	}
}

func TestSetupTracking_WithStringType(t *testing.T) {
	_, err := SetupTracking("not a server", nil, nil, nil)

	if err == nil {
		t.Error("Expected error when setting up tracking with string type")
	}

	expectedSubstring := "unsupported server type 'unknown'"
	if !contains(err.Error(), expectedSubstring) {
		t.Errorf("Expected error to contain '%s', got: %v", expectedSubstring, err)
	}
}

func TestGetMCPcat_NotRegistered(t *testing.T) {
	mockServer := &unsupportedServer{}
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
	// Test that UnregisterServer doesn't panic
	mockServer := &unsupportedServer{}

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
	// Test that Shutdown doesn't panic
	// This is a basic smoke test
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Shutdown panicked: %v", r)
		}
	}()

	Shutdown()
}

func TestShutdown_MultipleCalls(t *testing.T) {
	// Test that calling Shutdown multiple times doesn't panic
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
	// Test that MCPcat struct can be created
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
	// Manually register a server to test GetMCPcat retrieval
	mockServer := &struct{ id string }{id: "test-server"}
	projectID := "proj_manual_test"
	opts := DefaultOptions()

	instance := registry.Get(mockServer)
	if instance != nil {
		// Clean up if somehow already registered
		registry.Unregister(mockServer)
	}

	// Manually register
	registry.Register(mockServer, &MCPcatInstance{
		ProjectID: projectID,
		Options:   &opts,
		ServerRef: mockServer,
	})

	// Retrieve and verify
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

	// Clean up
	registry.Unregister(mockServer)
}

func TestGetMCPcat_WithNilOptions(t *testing.T) {
	// Test GetMCPcat when registry instance has nil options
	mockServer := &struct{ id string }{id: "test-server-nil-opts"}

	// Manually register with nil options
	registry.Register(mockServer, &MCPcatInstance{
		ProjectID: "proj_test",
		Options:   nil, // nil options
		ServerRef: mockServer,
	})

	// Should return nil because options are required
	result := GetMCPcat(mockServer)
	if result != nil {
		t.Error("Expected nil when registry instance has nil options")
	}

	// Clean up
	registry.Unregister(mockServer)
}
