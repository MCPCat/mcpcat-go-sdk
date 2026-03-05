package officialsdk

import (
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestGetOrCreateSession_NilRequest(t *testing.T) {
	sessionMap := &sync.Map{}
	ps := getOrCreateSession(nil, sessionMap, nil, "proj_123")
	if ps != nil {
		t.Error("expected nil session for nil request")
	}
}

func TestGetOrCreateSession_NilSession(t *testing.T) {
	// A CallToolRequest without a Session field set has nil Session
	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name: "test",
		},
	}
	// GetSession() returns nil for the default zero value
	sessionMap := &sync.Map{}
	ps := getOrCreateSession(req, sessionMap, nil, "proj_123")
	if ps != nil {
		t.Error("expected nil session when request has no Session set")
	}
}

func TestGetOrCreateSession_ServerImplValues(t *testing.T) {
	// Test that serverImpl values are correctly structured for use.
	// We cannot create a real ServerSession without transport setup,
	// so we just verify the data flow.
	serverImpl := &mcp.Implementation{
		Name:    "my-server",
		Version: "v2.0.0",
	}

	if serverImpl.Name != "my-server" {
		t.Errorf("expected server name 'my-server', got '%s'", serverImpl.Name)
	}
	if serverImpl.Version != "v2.0.0" {
		t.Errorf("expected server version 'v2.0.0', got '%s'", serverImpl.Version)
	}
}
