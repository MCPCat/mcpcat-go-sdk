// Package compat provides compatibility layer for different MCP server implementations.
// It detects and handles differences between mark3labs/mcp-go and modelcontextprotocol/go-sdk.
package compat

import (
	"reflect"
)

// ServerType represents the type of MCP server
type ServerType string

const (
	// ServerTypeUnknown represents an unknown or unsupported server type
	ServerTypeUnknown ServerType = "unknown"

	// ServerTypeMark3Labs represents mark3labs/mcp-go server
	ServerTypeMark3Labs ServerType = "mark3labs"

	// ServerTypeModelContext represents modelcontextprotocol/go-sdk server
	ServerTypeModelContext ServerType = "modelcontextprotocol"
)

// DetectServerType detects the type of MCP server by examining its type
func DetectServerType(server interface{}) ServerType {
	if server == nil {
		return ServerTypeUnknown
	}

	typeName := reflect.TypeOf(server).String()

	// Check for mark3labs/mcp-go server
	if typeName == "*server.MCPServer" {
		return ServerTypeMark3Labs
	}

	// Check for modelcontextprotocol/go-sdk server
	// TODO: Add detection logic for modelcontextprotocol server

	return ServerTypeUnknown
}
