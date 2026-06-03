//
// Tencent is pleased to support the open source community by making trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//
//

// Package tool provides tool interfaces and implementations for the agent system.
package tool

import (
	"context"
	"regexp"
	"strings"
)

// toolNamePattern is the pattern that tool names must match for
// compatibility with LLM providers (e.g. DeepSeek, OpenAI).
// Only letters, digits, underscores, and hyphens are allowed.
var toolNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// Tool defines the interface for tools that can be used by agents.
// It provides a common contract for all tool implementations.
type Tool interface {
	// Declaration returns the metadata describing the tool.
	Declaration() *Declaration
}

// CallableTool defines the interface for tools that support calling operations.
type CallableTool interface {
	// Call calls the tool with the provided context and arguments.
	// Returns the result of execution or an error if the operation fails.
	Call(ctx context.Context, jsonArgs []byte) (any, error)

	Tool
}

// StreamableTool defines the interface for tools that support streaming operations.
// This interface extends the basic CallableTool interface to provide streaming capabilities,
// allowing tools to return data progressively rather than all at once.
type StreamableTool interface {
	// StreamableCall initiates a call to the tool that supports streaming.
	// It takes a context for cancellation and timeout control, and JSON-encoded
	// arguments for the tool. Returns a StreamReader for consuming the streaming
	// results or an error if the call fails to initialize.
	StreamableCall(ctx context.Context, jsonArgs []byte) (*StreamReader, error)
	Tool
}

// Declaration describes the metadata of a tool, such as its name, description, and expected arguments.
type Declaration struct {
	// Name is the unique identifier of the tool
	Name string `json:"name"`

	// Description explains the tool's purpose and functionality
	Description string `json:"description"`

	// InputSchema defines the expected input for the tool in JSON schema format.
	InputSchema *Schema `json:"inputSchema"`

	// OutputSchema defines the expected output for the tool in JSON schema format.
	OutputSchema *Schema `json:"outputSchema,omitempty"`
}

// Schema represents the structure of JSON Schema used for defining arguments and responses.
// It follows the JSON Schema standard, supporting various types, properties, and validation rules.
// This structure is typically used to define the expected format of arguments for tools or functions
// and to validate that incoming data conforms to the expected structure.
type Schema struct {
	//  Type Specifies the data type (e.g., "object", "array", "string", "number")
	Type        string   `json:"type,omitempty"`
	Description string   `json:"description,omitempty"`
	Required    []string `json:"required,omitempty"`
	// Properties of the arguments, each with its own schema
	Properties map[string]*Schema `json:"properties,omitempty"`
	// For array types, defines the schema of items in the array
	Items *Schema `json:"items,omitempty"`
	// AdditionalProperties: Controls whether properties not defined in Properties are allowed
	AdditionalProperties any `json:"additionalProperties,omitempty"`
	// Default value for the parameter
	Default any `json:"default,omitempty"`
	// Enum contains the list of allowed values for the parameter
	Enum []any `json:"enum,omitempty"`
	// Ref is used for JSON Schema references to avoid infinite recursion
	Ref string `json:"$ref,omitempty"`
	// Defs contains reusable schema definitions
	Defs map[string]*Schema `json:"$defs,omitempty"`
}

// SanitizeToolName converts a tool name into a form compatible with LLM
// providers that require function names to match ^[a-zA-Z0-9_-]+$.
// Characters outside the allowed set are replaced with underscores;
// consecutive underscores are collapsed; leading/trailing underscores
// are stripped. If the result would start with a digit, a "t_" prefix
// is prepended. An empty or fully-invalid input returns "unnamed_tool".
func SanitizeToolName(name string) string {
	if toolNamePattern.MatchString(name) {
		return name
	}
	var b strings.Builder
	lastUnderscore := false
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
			lastUnderscore = false
		} else {
			if !lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	result := strings.Trim(b.String(), "_-")
	if result == "" {
		return "unnamed_tool"
	}
	if result[0] >= '0' && result[0] <= '9' {
		result = "t_" + result
	}
	const maxLen = 64
	if len(result) > maxLen {
		result = strings.TrimRight(result[:maxLen], "_-")
	}
	return result
}
