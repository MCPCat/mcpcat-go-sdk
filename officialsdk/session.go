package officialsdk

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	mcpcat "github.com/mcpcat/mcpcat-go-sdk"
)

// getOrCreateSession extracts or creates session metadata from the request,
// maintaining a session map keyed by the ServerSession ID.
// It returns a *mcpcat.ProtectedSession that callers must lock before accessing fields.
func getOrCreateSession(
	req mcp.Request,
	sessionMap *mcpcat.SessionMap,
	serverImpl *mcp.Implementation,
	projectID string,
) *mcpcat.ProtectedSession {
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
		rawSessionID = "nosessionid"
	}

	formattedSessionID := mcpcat.NewSessionID()
	newPS := &mcpcat.ProtectedSession{
		Sess: &mcpcat.Session{
			SessionID: &formattedSessionID,
			ProjectID: &projectID,
		},
	}

	ps, _ := sessionMap.LoadOrStore(rawSessionID, newPS)

	ps.Mu.Lock()
	defer ps.Mu.Unlock()

	if ps.Sess.SdkLanguage == nil {
		ps.Sess.SdkLanguage = mcpcat.Ptr("modelcontextprotocol/go-sdk")
	}

	if ps.Sess.McpcatVersion == nil {
		version := mcpcat.GetDependencyVersion("github.com/modelcontextprotocol/go-sdk")
		ps.Sess.McpcatVersion = &version
	}

	if ps.Sess.ClientName == nil {
		initParams := serverSession.InitializeParams()
		if initParams != nil && initParams.ClientInfo != nil {
			if initParams.ClientInfo.Name != "" {
				ps.Sess.ClientName = mcpcat.Ptr(initParams.ClientInfo.Name)
			}
			if initParams.ClientInfo.Version != "" {
				ps.Sess.ClientVersion = mcpcat.Ptr(initParams.ClientInfo.Version)
			}
		}
	}

	if ps.Sess.ServerName == nil && serverImpl != nil {
		if serverImpl.Name != "" {
			ps.Sess.ServerName = mcpcat.Ptr(serverImpl.Name)
		}
		if serverImpl.Version != "" {
			ps.Sess.ServerVersion = mcpcat.Ptr(serverImpl.Version)
		}
	}

	ps.Touch()
	return ps
}

// updateSessionFromInitResult updates the session with server info from the
// initialize result. The caller must hold ps.Mu.
func updateSessionFromInitResult(ps *mcpcat.ProtectedSession, result mcp.Result) {
	if ps == nil || result == nil {
		return
	}
	initResult, ok := result.(*mcp.InitializeResult)
	if !ok || initResult == nil {
		return
	}
	if ps.Sess.ServerName == nil && initResult.ServerInfo != nil {
		if initResult.ServerInfo.Name != "" {
			ps.Sess.ServerName = mcpcat.Ptr(initResult.ServerInfo.Name)
		}
		if initResult.ServerInfo.Version != "" {
			ps.Sess.ServerVersion = mcpcat.Ptr(initResult.ServerInfo.Version)
		}
	}
}
