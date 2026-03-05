package event

import (
	"encoding/json"
	"fmt"
	"time"

	mcpcatapi "github.com/mcpcat/mcpcat-go-api"
	"github.com/mcpcat/mcpcat-go-sdk/internal/core"
	"github.com/mcpcat/mcpcat-go-sdk/internal/exceptions"
	"github.com/mcpcat/mcpcat-go-sdk/internal/logging"
	"github.com/segmentio/ksuid"
)

// NewEventID generates a new unique event ID with the MCPCat prefix.
func NewEventID() string {
	return fmt.Sprintf("%s_%s", core.PrefixEvent, ksuid.New().String())
}

// NewEvent creates an SDK-agnostic Event from session data and basic metadata.
func NewEvent(session *core.Session, eventType string, duration *int32, isError bool, errorDetails error) *Event {
	if session == nil {
		return nil
	}

	eventID := NewEventID()

	sessionID := ""
	if session.SessionID != nil {
		sessionID = *session.SessionID
	}

	event := &Event{
		PublishEventRequest: mcpcatapi.PublishEventRequest{
			Id:        &eventID,
			ProjectId: session.ProjectID,
			SessionId: sessionID,
			EventType: &eventType,
			Duration:  duration,
			Timestamp: Ptr(time.Now()),
		},
	}

	if isError {
		event.IsError = &isError
		if errorDetails != nil {
			event.Error = exceptions.CaptureException(errorDetails)
		}
	}

	CopySessionToEvent(session, event)

	return event
}

// ConvertToMap converts any value (including structs, slices of structs) to map[string]any or []any
// by marshaling to JSON and unmarshaling back. This ensures the redactor can process all fields.
// If conversion fails, the original value is returned to avoid impacting the server.
func ConvertToMap(v any) any {
	if v == nil {
		return nil
	}

	// Use defer/recover to ensure any panics don't crash the server
	defer func() {
		if r := recover(); r != nil {
			logger := logging.New()
			logger.Debugf("ConvertToMap: panic recovered during conversion: %v (value type: %T)", r, v)
		}
	}()

	// Marshal to JSON and unmarshal to generic types
	data, err := json.Marshal(v)
	if err != nil {
		// If marshaling fails, return the original value silently
		return v
	}

	var result any
	if err := json.Unmarshal(data, &result); err != nil {
		// If unmarshaling fails, return the original value silently
		return v
	}

	return result
}

// CopySessionToEvent copies session metadata fields to the event.
func CopySessionToEvent(session *core.Session, event *Event) {
	if session == nil || event == nil {
		return
	}

	// Copy all session fields to the event
	event.IpAddress = session.IpAddress
	event.SdkLanguage = session.SdkLanguage
	event.McpcatVersion = session.McpcatVersion
	event.ServerName = session.ServerName
	event.ServerVersion = session.ServerVersion
	event.ClientName = session.ClientName
	event.ClientVersion = session.ClientVersion
	event.IdentifyActorGivenId = session.IdentifyActorGivenId
	event.IdentifyActorName = session.IdentifyActorName
	event.IdentifyData = session.IdentifyData
}

// CreateIdentifyEvent creates an Event for mcpcat:identify event type.
func CreateIdentifyEvent(session *core.Session) *Event {
	if session == nil {
		return nil
	}

	eventID := NewEventID()

	sessionID := ""
	if session.SessionID != nil {
		sessionID = *session.SessionID
	}

	eventType := "mcpcat:identify"
	event := &Event{
		PublishEventRequest: mcpcatapi.PublishEventRequest{
			Id:        &eventID,
			ProjectId: session.ProjectID,
			SessionId: sessionID,
			EventType: &eventType,
			Timestamp: Ptr(time.Now()),
		},
	}

	CopySessionToEvent(session, event)

	return event
}

// Ptr is a helper function to get a pointer to a value.
func Ptr[T any](v T) *T {
	return &v
}

// LogEvent logs an event in a formatted, human-readable way for debugging.
func LogEvent(logger interface{ Infof(string, ...any) }, evt *Event, title string) {
	if evt == nil {
		logger.Infof("%s: <nil event>", title)
		return
	}

	logger.Infof("=== %s ===", title)

	// Basic event info
	if evt.Id != nil {
		logger.Infof("  Event ID: %s", *evt.Id)
	}
	if evt.EventType != nil {
		logger.Infof("  Event Type: %s", *evt.EventType)
	}
	if evt.ProjectId != nil {
		logger.Infof("  Project ID: %s", *evt.ProjectId)
	}
	logger.Infof("  Session ID: %s", evt.SessionId)

	// Timing info
	if evt.Timestamp != nil {
		logger.Infof("  Timestamp: %s", evt.Timestamp.Format(time.RFC3339))
	}
	if evt.Duration != nil {
		logger.Infof("  Duration: %d ms", *evt.Duration)
	}

	// Error status
	if evt.IsError != nil && *evt.IsError {
		logger.Infof("  Is Error: true")
		if evt.Error != nil {
			logger.Infof("  Error Details: %v", evt.Error)
		}
	}

	// User intent
	if evt.UserIntent != nil {
		logger.Infof("  User Intent: %s", *evt.UserIntent)
	}

	// Resource name (for resource events)
	if evt.ResourceName != nil {
		logger.Infof("  Resource Name: %s", *evt.ResourceName)
	}

	// Parameters
	if len(evt.Parameters) > 0 {
		logger.Infof("  Parameters:")
		for k, v := range evt.Parameters {
			logger.Infof("    %s: %v", k, v)
		}
	}

	// Response
	if len(evt.Response) > 0 {
		logger.Infof("  Response:")
		for k, v := range evt.Response {
			// Truncate long values for readability
			valStr := fmt.Sprintf("%v", v)
			if len(valStr) > 200 {
				valStr = valStr[:197] + "..."
			}
			logger.Infof("    %s: %s", k, valStr)
		}
	}

	// Session metadata
	logger.Infof("  Session Metadata:")
	if evt.ClientName != nil {
		logger.Infof("    Client: %s", *evt.ClientName)
		if evt.ClientVersion != nil {
			logger.Infof("    Client Version: %s", *evt.ClientVersion)
		}
	}
	if evt.ServerName != nil {
		logger.Infof("    Server: %s", *evt.ServerName)
		if evt.ServerVersion != nil {
			logger.Infof("    Server Version: %s", *evt.ServerVersion)
		}
	}
	if evt.SdkLanguage != nil {
		logger.Infof("    SDK Language: %s", *evt.SdkLanguage)
	}
	if evt.McpcatVersion != nil {
		logger.Infof("    MCPCat Version: %s", *evt.McpcatVersion)
	}
	if evt.IpAddress != nil {
		logger.Infof("    IP Address: %s", *evt.IpAddress)
	}

	// Identity info
	if evt.IdentifyActorGivenId != nil || evt.IdentifyActorName != nil {
		logger.Infof("  Identity:")
		if evt.IdentifyActorGivenId != nil {
			logger.Infof("    Actor ID: %s", *evt.IdentifyActorGivenId)
		}
		if evt.IdentifyActorName != nil {
			logger.Infof("    Actor Name: %s", *evt.IdentifyActorName)
		}
		if len(evt.IdentifyData) > 0 {
			logger.Infof("    Additional Data:")
			for k, v := range evt.IdentifyData {
				logger.Infof("      %s: %v", k, v)
			}
		}
	}

	logger.Infof("=== End %s ===", title)
}
