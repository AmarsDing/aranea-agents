//
// Tencent is pleased to support the open source community by making trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//
//

package tool

import (
	"context"
	"testing"
	"time"
)

func TestSanitizeToolName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// Already valid names pass through unchanged.
		{"valid_name", "valid_name"},
		{"valid-name", "valid-name"},
		{"ValidName123", "ValidName123"},
		{"subagents_spawn", "subagents_spawn"},
		// MCP-style dot-separated names.
		{"playwright.browser_navigate", "playwright_browser_navigate"},
		{"server.tool.name", "server_tool_name"},
		// OpenAPI-style names with slashes and braces.
		{"get_/api/v1/users/{id}", "get__api_v1_users_id"},
		// Double colon or other separators.
		{"github:issue_create", "github_issue_create"},
		// Double underscore from MCP convention.
		{"mcp__server__tool", "mcp__server__tool"},
		// Leading digit gets prefixed.
		{"123_tool", "123_tool"}, // already valid, passes through
		// Empty or all-invalid returns fallback.
		{"", "unnamed_tool"},
		{"!!!", "unnamed_tool"},
		// Consecutive special chars collapse to single underscore.
		{"a..b", "a_b"},
		// Leading/trailing underscores are already valid, pass through.
		{"_leading", "_leading"},
		{"trailing_", "trailing_"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := SanitizeToolName(tt.input)
			if got != tt.expected {
				t.Errorf("SanitizeToolName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
			// Verify result always matches the required pattern.
			if !toolNamePattern.MatchString(got) {
				t.Errorf("SanitizeToolName(%q) = %q does not match required pattern", tt.input, got)
			}
		})
	}
}

func TestStreamableTool_Interface(t *testing.T) {
	// Compile-time check
	var _ StreamableTool = (*testStreamableTool)(nil)
}

type testStreamableTool struct{}

func (d *testStreamableTool) StreamableCall(ctx context.Context, jsonArgs []byte) (*StreamReader, error) {
	s := NewStream(1)
	go func() {
		defer s.Writer.Close()
		s.Writer.Send(StreamChunk{Content: "test", Metadata: Metadata{CreatedAt: time.Now()}}, nil)
		s.Writer.Send(StreamChunk{Content: "more data"}, nil)
		s.Writer.Send(StreamChunk{Content: "final chunk"}, nil)

	}()
	return s.Reader, nil
}
func (d *testStreamableTool) Declaration() *Declaration {
	return &Declaration{
		Name:        "TestStreamableTool",
		Description: "A test tool for streaming data.",
		InputSchema: &Schema{
			Type:        "object",
			Properties:  map[string]*Schema{"input": {Type: "string"}},
			Required:    []string{"input"},
			Description: "Input for the test streamable tool.",
		},
	}
}
