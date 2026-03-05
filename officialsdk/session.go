package officialsdk

import (
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	mcpcat "github.com/mcpcat/mcpcat-go-sdk"
)

// protectedSession wraps a *mcpcat.Session with a mutex to protect concurrent
// access to its fields, and a sync.Once to ensure the Identify callback is
// invoked at most once per session.
type protectedSession struct {
	mu           sync.Mutex
	sess         *mcpcat.Session
	identifyOnce sync.Once
}

// getOrCreateSession extracts or creates session metadata from the request,
// maintaining a session map keyed by the ServerSession ID.
// It returns a *protectedSession that callers must lock before accessing fields.
func getOrCreateSession(
	req mcp.Request,
	sessionMap *sync.Map,
	serverImpl *mcp.Implementation,
	projectID string,
) *protectedSession {
	if req == nil {
		return nil
	}

	rawSession := req.GetSession()
	if rawSession == nil {
		return nil
	}

	serverSession, ok := rawSession.(*mcp.ServerSession)
	if !ok || serverSession == nil {
		return nil
	}

	rawSessionID := serverSession.ID()
	if rawSessionID == "" {
		// If no session ID, use a placeholder key based on the pointer address.
		// This ensures we still create a session for transports that don't
		// provide a session ID.
		rawSessionID = "nosessionid"
	}

	// Build a candidate session for LoadOrStore.
	formattedSessionID := mcpcat.NewSessionID()
	newPS := &protectedSession{
		sess: &mcpcat.Session{
			SessionID: &formattedSessionID,
			ProjectID: &projectID,
		},
	}

	// LoadOrStore is atomic: if another goroutine stored first, we get that one.
	actual, _ := sessionMap.LoadOrStore(rawSessionID, newPS)
	ps := actual.(*protectedSession)

	// Populate session fields under the lock.
	ps.mu.Lock()
	defer ps.mu.Unlock()

	// Set SDK information
	if ps.sess.SdkLanguage == nil {
		ps.sess.SdkLanguage = mcpcat.Ptr("modelcontextprotocol/go-sdk")
	}

	if ps.sess.McpcatVersion == nil {
		version := mcpcat.GetDependencyVersion("github.com/modelcontextprotocol/go-sdk")
		ps.sess.McpcatVersion = &version
	}

	// Extract client info from ServerSession.InitializeParams()
	if ps.sess.ClientName == nil {
		initParams := serverSession.InitializeParams()
		if initParams != nil && initParams.ClientInfo != nil {
			if initParams.ClientInfo.Name != "" {
				ps.sess.ClientName = mcpcat.Ptr(initParams.ClientInfo.Name)
			}
			if initParams.ClientInfo.Version != "" {
				ps.sess.ClientVersion = mcpcat.Ptr(initParams.ClientInfo.Version)
			}
		}
	}

	// Extract server info from the Implementation stored at Track() time
	if ps.sess.ServerName == nil && serverImpl != nil {
		if serverImpl.Name != "" {
			ps.sess.ServerName = mcpcat.Ptr(serverImpl.Name)
		}
		if serverImpl.Version != "" {
			ps.sess.ServerVersion = mcpcat.Ptr(serverImpl.Version)
		}
	}

	return ps
}

// updateSessionFromInitResult updates the session with server info from the
// initialize result. This is called when we observe an initialize response
// flowing through the middleware. The caller must hold ps.mu.
func updateSessionFromInitResult(ps *protectedSession, result mcp.Result) {
	if ps == nil || result == nil {
		return
	}
	initResult, ok := result.(*mcp.InitializeResult)
	if !ok || initResult == nil {
		return
	}
	if ps.sess.ServerName == nil && initResult.ServerInfo != nil {
		if initResult.ServerInfo.Name != "" {
			ps.sess.ServerName = mcpcat.Ptr(initResult.ServerInfo.Name)
		}
		if initResult.ServerInfo.Version != "" {
			ps.sess.ServerVersion = mcpcat.Ptr(initResult.ServerInfo.Version)
		}
	}
}
