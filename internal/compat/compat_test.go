package compat

import (
	"testing"
)

// Mock types to simulate different server types
type mockMark3LabsServer struct{}
type mockOtherServer struct{}
type mockGenericServer struct{}

func TestDetectServerType_NilServer(t *testing.T) {
	result := DetectServerType(nil)
	if result != ServerTypeUnknown {
		t.Errorf("Expected ServerTypeUnknown for nil server, got %s", result)
	}
}

func TestDetectServerType_UnknownServer(t *testing.T) {
	tests := []struct {
		name   string
		server interface{}
	}{
		{
			name:   "Mock generic server",
			server: &mockGenericServer{},
		},
		{
			name:   "Mock other server",
			server: &mockOtherServer{},
		},
		{
			name:   "String type",
			server: "not a server",
		},
		{
			name:   "Integer type",
			server: 42,
		},
		{
			name:   "Map type",
			server: map[string]string{"key": "value"},
		},
		{
			name:   "Slice type",
			server: []string{"a", "b", "c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectServerType(tt.server)
			if result != ServerTypeUnknown {
				t.Errorf("Expected ServerTypeUnknown for %s, got %s", tt.name, result)
			}
		})
	}
}

func TestDetectServerType_WithRealTypes(t *testing.T) {
	// Note: We can't directly test mark3labs server without importing it,
	// but we can test that the function properly examines type names
	tests := []struct {
		name     string
		server   interface{}
		expected ServerType
	}{
		{
			name:     "Generic mock server should be unknown",
			server:   &mockMark3LabsServer{},
			expected: ServerTypeUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectServerType(tt.server)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestServerTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant ServerType
		expected string
	}{
		{
			name:     "ServerTypeUnknown",
			constant: ServerTypeUnknown,
			expected: "unknown",
		},
		{
			name:     "ServerTypeMark3Labs",
			constant: ServerTypeMark3Labs,
			expected: "mark3labs",
		},
		{
			name:     "ServerTypeModelContext",
			constant: ServerTypeModelContext,
			expected: "modelcontextprotocol",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.constant) != tt.expected {
				t.Errorf("Expected %s to equal '%s', got '%s'", tt.name, tt.expected, string(tt.constant))
			}
		})
	}
}

func TestServerTypeString(t *testing.T) {
	// Test that ServerType can be converted to string and compared
	unknown := ServerTypeUnknown
	mark3labs := ServerTypeMark3Labs
	modelcontext := ServerTypeModelContext

	if unknown != "unknown" {
		t.Errorf("Expected ServerTypeUnknown to equal 'unknown'")
	}

	if mark3labs != "mark3labs" {
		t.Errorf("Expected ServerTypeMark3Labs to equal 'mark3labs'")
	}

	if modelcontext != "modelcontextprotocol" {
		t.Errorf("Expected ServerTypeModelContext to equal 'modelcontextprotocol'")
	}
}

func TestDetectServerType_TypeReflection(t *testing.T) {
	// Test that the function properly uses reflection
	// Even though we can't create the actual mark3labs server,
	// we can verify the function works with various Go types

	tests := []struct {
		name     string
		input    interface{}
		wantType ServerType
	}{
		{
			name:     "Pointer to struct",
			input:    &struct{ Field string }{Field: "test"},
			wantType: ServerTypeUnknown,
		},
		{
			name:     "Non-pointer struct",
			input:    struct{ Field string }{Field: "test"},
			wantType: ServerTypeUnknown,
		},
		{
			name:     "Interface value",
			input:    interface{}("test"),
			wantType: ServerTypeUnknown,
		},
		{
			name:     "Function",
			input:    func() {},
			wantType: ServerTypeUnknown,
		},
		{
			name:     "Channel",
			input:    make(chan int),
			wantType: ServerTypeUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectServerType(tt.input)
			if result != tt.wantType {
				t.Errorf("DetectServerType(%s) = %v, want %v", tt.name, result, tt.wantType)
			}
		})
	}
}
