package session

import (
	"context"
	"runtime/debug"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/mcpcat/mcpcat-go-sdk/internal/core"
	"github.com/mcpcat/mcpcat-go-sdk/internal/event"
	"github.com/mcpcat/mcpcat-go-sdk/internal/logging"
	"github.com/mcpcat/mcpcat-go-sdk/internal/publisher"
	"github.com/mcpcat/mcpcat-go-sdk/internal/registry"
)

// sessionMap stores sessions by their ID
var (
	sessionMap           = make(map[string]*core.Session)
	sessionMu            sync.RWMutex
	sessionPublisher     *publisher.Publisher
	sessionPublisherMu   sync.Mutex
	sessionPublisherInit sync.Once
)

// getPublisher returns the session publisher, initializing it if needed
func getPublisher(redactFn core.RedactFunc) *publisher.Publisher {
	sessionPublisherInit.Do(func() {
		sessionPublisherMu.Lock()
		defer sessionPublisherMu.Unlock()
		sessionPublisher = publisher.New(redactFn)
	})
	return sessionPublisher
}

func CaptureSessionFromMark3LabsContext(ctx context.Context, request any, response any) *core.Session {
	clientSession := server.ClientSessionFromContext(ctx)
	if clientSession == nil {
		return nil
	}

	rawSessionID := clientSession.SessionID()

	// Get or create session
	sessionMu.Lock()
	session, exists := sessionMap[rawSessionID]
	if !exists {
		// Generate a properly formatted session ID with MCPCat prefix
		formattedSessionID := generateSessionID()
		session = &core.Session{
			SessionID: &formattedSessionID,
		}
		sessionMap[rawSessionID] = session
	}
	sessionMu.Unlock()

	// Always try to populate/update session fields
	// Try to get ProjectID from the registry if not already set
	if session.ProjectID == nil {
		mcpServer := server.ServerFromContext(ctx)
		if mcpServer != nil {
			if tracker := registry.Get(mcpServer); tracker != nil {
				session.ProjectID = &tracker.ProjectID
			}
		}
	}

	// Set SDK information (these are constants)
	if session.SdkLanguage == nil {
		sdkLang := "golang"
		session.SdkLanguage = &sdkLang
	}

	if session.McpcatVersion == nil {
		version := "dev" // Default version for local development
		if debugInfo, ok := debug.ReadBuildInfo(); ok && debugInfo.Main.Version != "" {
			version = debugInfo.Main.Version
		}
		session.McpcatVersion = &version
	}

	// Try to cast to SessionWithClientInfo to extract client details
	if sessionWithInfo, ok := clientSession.(server.SessionWithClientInfo); ok {
		clientInfo := sessionWithInfo.GetClientInfo()

		// Update client name if available and not already set
		if clientInfo.Name != "" && session.ClientName == nil {
			name := clientInfo.Name
			session.ClientName = &name
		}

		// Update client version if available and not already set
		if clientInfo.Version != "" && session.ClientVersion == nil {
			version := clientInfo.Version
			session.ClientVersion = &version
		}
	}

	// If the response is an InitializeResult, extract server info
	if initializeResult, ok := response.(*mcp.InitializeResult); ok {
		serverInfo := initializeResult.ServerInfo
		if session.ServerName == nil {
			name := serverInfo.Name
			session.ServerName = &name
		}
		if session.ServerVersion == nil {
			version := serverInfo.Version
			session.ServerVersion = &version
		}
	}

	// Attempt identify IF session is NOT already identified
	mcpServer := server.ServerFromContext(ctx)
	if mcpServer == nil || session.IdentifyActorGivenId != nil {
		return session
	}

	tracker := registry.Get(mcpServer)
	if tracker == nil || tracker.Options.Identify == nil {
		return session
	}

	logger := logging.New()
	logger.Debugf("[IDENTIFY] Calling identify function for session: %s, request type: %T", rawSessionID, request)
	identifyInfo := tracker.Options.Identify(ctx, request)
	if identifyInfo == nil {
		logger.Debugf("[IDENTIFY] Identify function returned nil for session: %s", rawSessionID)
		return session
	}
	logger.Infof("[IDENTIFY] Identify function successful for session: %s, UserID: %s, UserName: %s",
		rawSessionID, identifyInfo.UserID, identifyInfo.UserName)

	// Update session with identify info
	session.IdentifyActorGivenId = &identifyInfo.UserID
	session.IdentifyActorName = &identifyInfo.UserName
	session.IdentifyData = identifyInfo.UserData

	// Publish mcpcat:identify event
	identifyEvent := event.CreateIdentifyEvent(session)
	if identifyEvent != nil {
		// No redaction needed for identify events - pass nil
		pub := getPublisher(nil)
		if pub != nil {
			logger.Debugf("[IDENTIFY] Publishing mcpcat:identify event for session: %s", rawSessionID)
			pub.Publish(identifyEvent)
		}
	}

	return session
}
