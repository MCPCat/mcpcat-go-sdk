package redaction

import (
	"fmt"

	"github.com/mcpcat/mcpcat-go-sdk/internal/core"
)

const redactionErrorPlaceholder = "[REDACTION_ERROR]"

// RedactEvent applies the redaction function to all string values in the event's
// Parameters, Response, and UserIntent fields. It recursively descends into nested maps and slices.
// If the redaction function panics, the string is replaced with [REDACTION_ERROR].
// If redaction fails completely, the Parameters and Response are replaced with error messages
// to prevent publishing unredacted sensitive data.
//
// This function creates a deep copy of the maps to avoid mutating the original event.
func RedactEvent(event *core.Event, redactFn core.RedactFunc) (err error) {
	if event == nil || redactFn == nil {
		return nil
	}

	// Catch any panics during redaction to ensure we never crash the publisher
	// but also ensure we never publish unredacted sensitive data
	defer func() {
		if r := recover(); r != nil {
			// Redaction failed catastrophically - replace with error placeholders for security
			event.Parameters = map[string]any{
				"error": "Failed to redact parameters due to internal error",
			}
			event.Response = map[string]any{
				"error": "Failed to redact response due to internal error",
			}
			if event.UserIntent != nil {
				redactedIntent := redactionErrorPlaceholder
				event.UserIntent = &redactedIntent
			}
			err = fmt.Errorf("redaction panic: %v", r)
		}
	}()

	// Redact Parameters map
	if event.Parameters != nil {
		event.Parameters = redactMap(event.Parameters, redactFn)
	}

	// Redact Response map
	if event.Response != nil {
		event.Response = redactMap(event.Response, redactFn)
	}

	// Redact UserIntent string
	if event.UserIntent != nil && *event.UserIntent != "" {
		redacted := safeRedact(*event.UserIntent, redactFn)
		event.UserIntent = &redacted
	}

	return nil
}

// redactMap recursively processes a map, creating a new map with redacted string values
func redactMap(m map[string]any, redactFn core.RedactFunc) map[string]any {
	if m == nil {
		return nil
	}

	result := make(map[string]any, len(m))
	for k, v := range m {
		result[k] = redactValue(v, redactFn)
	}
	return result
}

// redactValue recursively processes a value based on its type
func redactValue(v any, redactFn core.RedactFunc) any {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case string:
		// Apply redaction function with panic recovery
		return safeRedact(val, redactFn)

	case map[string]any:
		// Recursively redact nested maps
		return redactMap(val, redactFn)

	case []any:
		// Recursively redact slices
		result := make([]any, len(val))
		for i, item := range val {
			result[i] = redactValue(item, redactFn)
		}
		return result

	default:
		// For other types (numbers, bools, etc.), return as-is
		return v
	}
}

// safeRedact applies the redaction function with panic recovery
func safeRedact(s string, redactFn core.RedactFunc) string {
	var result string
	var panicked bool

	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		result = redactFn(s)
	}()

	if panicked {
		return redactionErrorPlaceholder
	}

	return result
}
