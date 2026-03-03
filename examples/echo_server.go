package main

import (
	"context"
	"fmt"
	"regexp"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	mcpcat "github.com/mcpcat/mcpcat-go-sdk"
)

func main() {
	// Optional but recommended: ensures graceful shutdown on normal exit
	defer mcpcat.Shutdown()

	s := server.NewMCPServer(
		"Demo 🚀",
		"1.0.0",
		server.WithToolCapabilities(false),
	)

	err := mcpcat.Track(s, "proj_XXXXXX", &mcpcat.Options{
		Debug:                      true,
		Identify:                   identifyUser,
		RedactSensitiveInformation: redactSensitiveData,
	})
	if err != nil {
		fmt.Printf("Failed to setup tracking: %v\n", err)
		return
	}

	// Add tool
	tool := mcp.NewTool("hello_world",
		mcp.WithDescription("Say hello to someone"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the person to greet"),
		),
	)

	// Add tool handler
	s.AddTool(tool, helloHandler)

	// Start the stdio server
	if err := server.ServeStdio(s); err != nil {
		fmt.Printf("Server error: %v\n", err)
	}
}

// identifyUser is an example identify function that attaches user information to sessions
// In a real application, you would extract this from authentication headers, JWT tokens, etc.
func identifyUser(ctx context.Context, request any) *mcpcat.UserIdentity {
	// Example: Extract user info from the request context
	// In production, you might decode a JWT, check session storage, or query a database

	return &mcpcat.UserIdentity{
		UserID:   "user_12345",
		UserName: "demo_user",
		UserData: map[string]any{
			"email":        "demo@example.com",
			"role":         "developer",
			"organization": "Example Corp",
		},
	}
}

// redactSensitiveData is an example redaction function that masks sensitive information
// before events are sent to MCPCat. This runs on all string values in Parameters and Response.
func redactSensitiveData(text string) string {
	// Example: Redact email addresses
	emailRegex := regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`)
	text = emailRegex.ReplaceAllString(text, "[REDACTED_EMAIL]")

	// Example: Redact credit card numbers (simple pattern)
	ccRegex := regexp.MustCompile(`\b\d{4}[\s-]?\d{4}[\s-]?\d{4}[\s-]?\d{4}\b`)
	text = ccRegex.ReplaceAllString(text, "[REDACTED_CC]")

	// Example: Redact API keys (common patterns)
	apiKeyRegex := regexp.MustCompile(`\b[A-Za-z0-9]{32,}\b`)
	text = apiKeyRegex.ReplaceAllString(text, "[REDACTED_KEY]")

	return text
}

func helloHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, err := request.RequireString("name")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Hello, %s!", name)), nil
}
