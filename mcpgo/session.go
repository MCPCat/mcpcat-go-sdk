package mcpgo

import (
	"context"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
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

// captureSessionFromContext extracts or creates session metadata from the
// mark3labs/mcp-go context, maintaining a session map keyed by raw session ID.
//
// It returns a *protectedSession that callers must lock before accessing
// sess fields.
func captureSessionFromContext(
	ctx context.Context,
	request any,
	response any,
	sessionMap *sync.Map,
	opts *Options,
	publishFn func(*mcpcat.Event),
) *protectedSession {
	clientSession := server.ClientSessionFromContext(ctx)
	if clientSession == nil {
		return nil
	}

	rawSessionID := clientSession.SessionID()

	// Build a candidate session for LoadOrStore.
	formattedSessionID := mcpcat.NewSessionID()
	newPS := &protectedSession{
		sess: &mcpcat.Session{
			SessionID: &formattedSessionID,
		},
	}

	// LoadOrStore is atomic: if another goroutine stored first, we get that one.
	actual, _ := sessionMap.LoadOrStore(rawSessionID, newPS)
	ps := actual.(*protectedSession)

	// Populate session fields under the lock.
	ps.mu.Lock()

	// Try to get ProjectID from the registry if not already set
	if ps.sess.ProjectID == nil {
		mcpServer := server.ServerFromContext(ctx)
		if mcpServer != nil {
			if tracker := mcpcat.GetInstance(mcpServer); tracker != nil {
				ps.sess.ProjectID = &tracker.ProjectID
			}
		}
	}

	// Set SDK information
	if ps.sess.SdkLanguage == nil {
		ps.sess.SdkLanguage = mcpcat.Ptr("mark3labs/mcp-go")
	}

	if ps.sess.McpcatVersion == nil {
		version := mcpcat.GetDependencyVersion("github.com/mark3labs/mcp-go")
		ps.sess.McpcatVersion = &version
	}

	// Extract client info from SessionWithClientInfo
	if sessionWithInfo, ok := clientSession.(server.SessionWithClientInfo); ok {
		clientInfo := sessionWithInfo.GetClientInfo()

		if clientInfo.Name != "" && ps.sess.ClientName == nil {
			ps.sess.ClientName = mcpcat.Ptr(clientInfo.Name)
		}
		if clientInfo.Version != "" && ps.sess.ClientVersion == nil {
			ps.sess.ClientVersion = mcpcat.Ptr(clientInfo.Version)
		}
	}

	// Extract server info from InitializeResult
	if initializeResult, ok := response.(*mcp.InitializeResult); ok {
		serverInfo := initializeResult.ServerInfo
		if ps.sess.ServerName == nil {
			ps.sess.ServerName = mcpcat.Ptr(serverInfo.Name)
		}
		if ps.sess.ServerVersion == nil {
			ps.sess.ServerVersion = mcpcat.Ptr(serverInfo.Version)
		}
	}

	// Determine if we should attempt identify.
	var shouldIdentify bool
	if opts != nil && opts.Identify != nil {
		if _, ok := request.(*mcp.CallToolRequest); ok {
			shouldIdentify = true
		}
	}

	// Release lock before calling external Identify callback.
	ps.mu.Unlock()

	// Use sync.Once to ensure Identify is called at most once per session.
	if shouldIdentify {
		ps.identifyOnce.Do(func() {
			toolReq := request.(*mcp.CallToolRequest)
			identifyInfo := opts.Identify(ctx, toolReq)
			if identifyInfo == nil {
				return
			}

			ps.mu.Lock()
			ps.sess.IdentifyActorGivenId = &identifyInfo.UserID
			ps.sess.IdentifyActorName = &identifyInfo.UserName
			ps.sess.IdentifyData = identifyInfo.UserData
			identifyEvent := mcpcat.CreateIdentifyEvent(ps.sess)
			ps.mu.Unlock()
			if identifyEvent != nil {
				publishFn(identifyEvent)
			}
		})
	}

	return ps
}
