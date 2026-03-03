package integration

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	mcpcat "github.com/mcpcat/mcpcat-go-sdk"
)

// TestContextParam_InjectedIntoToolSchemas verifies that when
// EnableToolCallContext is true, every tool exposed by ListTools includes
// a "context" property in its InputSchema.
func TestContextParam_InjectedIntoToolSchemas(t *testing.T) {
	h := newHarness(t, &mcpcat.Options{
		EnableToolCallContext: true,
		EnableReportMissing:   false,
	})

	ctx := context.Background()
	toolsResult, err := h.Client.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}

	if len(toolsResult.Tools) == 0 {
		t.Fatal("Expected at least one tool, got 0")
	}

	for _, tool := range toolsResult.Tools {
		props := tool.InputSchema.Properties
		if props == nil {
			t.Errorf("Tool %q has nil Properties; expected context property to be present", tool.Name)
			continue
		}
		if _, ok := props["context"]; !ok {
			t.Errorf("Tool %q is missing the injected \"context\" property", tool.Name)
		}
	}
}

// TestContextParam_NotInjectedWhenDisabled verifies that when
// EnableToolCallContext is false, no tool has a "context" property injected
// by MCPCat.
func TestContextParam_NotInjectedWhenDisabled(t *testing.T) {
	h := newHarness(t, &mcpcat.Options{
		EnableToolCallContext: false,
		EnableReportMissing:   false,
	})

	ctx := context.Background()
	toolsResult, err := h.Client.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}

	if len(toolsResult.Tools) == 0 {
		t.Fatal("Expected at least one tool, got 0")
	}

	for _, tool := range toolsResult.Tools {
		props := tool.InputSchema.Properties
		if props == nil {
			// No properties at all means no context param -- that is fine.
			continue
		}
		if _, ok := props["context"]; ok {
			t.Errorf("Tool %q should NOT have a \"context\" property when EnableToolCallContext is false", tool.Name)
		}
	}
}

// TestContextParam_IntentPassedThroughToolCall verifies that when a caller
// supplies both a regular argument and the injected "context" argument, the
// tool handler still receives the regular argument and produces the correct
// result (the context param is stripped before reaching the handler).
func TestContextParam_IntentPassedThroughToolCall(t *testing.T) {
	h := newHarness(t, &mcpcat.Options{
		EnableToolCallContext: true,
		EnableReportMissing:   false,
	})

	result := h.callTool("add_todo", map[string]any{
		"title":   "Buy milk",
		"context": "User wants to add grocery item",
	})

	text := resultText(result)
	assertContains(t, text, "Buy milk")
}

// TestContextParam_ToolCallWorksWithoutContextArg verifies that a tool call
// succeeds even when the caller does NOT supply the "context" argument,
// despite it being declared in the schema.
func TestContextParam_ToolCallWorksWithoutContextArg(t *testing.T) {
	h := newHarness(t, &mcpcat.Options{
		EnableToolCallContext: true,
		EnableReportMissing:   false,
	})

	result := h.callTool("add_todo", map[string]any{
		"title": "No context here",
	})

	text := resultText(result)
	assertContains(t, text, "No context here")
}
