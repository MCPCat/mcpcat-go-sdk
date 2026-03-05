package mcpgo

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	mcpcat "github.com/mcpcat/mcpcat-go-sdk"
)

// captureSessionFromContext extracts or creates session metadata from the
// mark3labs/mcp-go context, maintaining a session map keyed by raw session ID.
//
// It returns a *mcpcat.ProtectedSession that callers must lock before accessing
// Sess fields.
func captureSessionFromContext(
	ctx context.Context,
	request any,
	response any,
	sessionMap *mcpcat.SessionMap,
	opts *Options,
	publishFn func(*mcpcat.Event),
) *mcpcat.ProtectedSession {
	clientSession := server.ClientSessionFromContext(ctx)
	if clientSession == nil {
		return nil
	}

	rawSessionID := clientSession.SessionID()

	formattedSessionID := mcpcat.NewSessionID()
	newPS := &mcpcat.ProtectedSession{
		Sess: &mcpcat.Session{
			SessionID: &formattedSessionID,
		},
	}

	ps, _ := sessionMap.LoadOrStore(rawSessionID, newPS)

	ps.Mu.Lock()

	if ps.Sess.ProjectID == nil {
		mcpServer := server.ServerFromContext(ctx)
		if mcpServer != nil {
			if tracker := mcpcat.GetInstance(mcpServer); tracker != nil {
				ps.Sess.ProjectID = &tracker.ProjectID
			}
		}
	}

	if ps.Sess.SdkLanguage == nil {
		ps.Sess.SdkLanguage = mcpcat.Ptr("mark3labs/mcp-go")
	}

	if ps.Sess.McpcatVersion == nil {
		version := mcpcat.GetDependencyVersion("github.com/mark3labs/mcp-go")
		ps.Sess.McpcatVersion = &version
	}

	if sessionWithInfo, ok := clientSession.(server.SessionWithClientInfo); ok {
		clientInfo := sessionWithInfo.GetClientInfo()

		if clientInfo.Name != "" && ps.Sess.ClientName == nil {
			ps.Sess.ClientName = mcpcat.Ptr(clientInfo.Name)
		}
		if clientInfo.Version != "" && ps.Sess.ClientVersion == nil {
			ps.Sess.ClientVersion = mcpcat.Ptr(clientInfo.Version)
		}
	}

	if initializeResult, ok := response.(*mcp.InitializeResult); ok {
		serverInfo := initializeResult.ServerInfo
		if ps.Sess.ServerName == nil {
			ps.Sess.ServerName = mcpcat.Ptr(serverInfo.Name)
		}
		if ps.Sess.ServerVersion == nil {
			ps.Sess.ServerVersion = mcpcat.Ptr(serverInfo.Version)
		}
	}

	var shouldIdentify bool
	if opts != nil && opts.Identify != nil {
		if _, ok := request.(*mcp.CallToolRequest); ok {
			shouldIdentify = true
		}
	}

	ps.Mu.Unlock()

	if shouldIdentify {
		ps.IdentifyOnce.Do(func() {
			toolReq := request.(*mcp.CallToolRequest)
			identifyInfo := opts.Identify(ctx, toolReq)
			if identifyInfo == nil {
				return
			}

			ps.Mu.Lock()
			ps.Sess.IdentifyActorGivenId = &identifyInfo.UserID
			ps.Sess.IdentifyActorName = &identifyInfo.UserName
			ps.Sess.IdentifyData = identifyInfo.UserData
			identifyEvent := mcpcat.CreateIdentifyEvent(ps.Sess)
			ps.Mu.Unlock()
			if identifyEvent != nil {
				publishFn(identifyEvent)
			}
		})
	}

	ps.Touch()
	return ps
}
