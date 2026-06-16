package testutil

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// CallTool invokes an MCP tool handler with the given arguments.
// handler is the tool handler function from package main tests.
type ToolHandler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)

// NewToolRequest builds a CallToolRequest for testing.
func NewToolRequest(name string, args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      name,
			Arguments: args,
		},
	}
}

// ResultText extracts the first text content from a tool result.
func ResultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if result == nil {
		t.Fatal("nil tool result")
	}
	if len(result.Content) == 0 {
		return ""
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", result.Content[0])
	}
	return tc.Text
}

// AssertNoToolError fails the test if the result is an error result.
func AssertNoToolError(t *testing.T, result *mcp.CallToolResult) {
	t.Helper()
	if result != nil && result.IsError {
		t.Fatalf("unexpected tool error: %s", ResultText(t, result))
	}
}
