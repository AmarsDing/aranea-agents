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
	"fmt"
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

// ResultBudget controls the maximum size of tool execution results.
// When a tool result exceeds MaxBytes, it is automatically truncated.
type ResultBudget struct {
	// MaxBytes is the maximum number of bytes for the serialized tool result.
	// 0 means no limit (default).
	MaxBytes int

	// TruncationMode controls how results are truncated:
	// "tail" - keep the beginning, truncate the end (default)
	// "head" - keep the end, truncate the beginning
	TruncationMode string
}

// DefaultResultBudget is the default budget for tool results (10KB).
var DefaultResultBudget = &ResultBudget{
	MaxBytes:       10 * 1024, // 10KB
	TruncationMode: "tail",
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

	// ResultBudget controls the maximum size of tool execution results.
	// If nil, the global budget (if set) is used instead.
	// This field is not exposed to the LLM.
	ResultBudget *ResultBudget `json:"-"`
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

	// Constraint fields for enhanced parameter validation.
	// These follow JSON Schema specification and help LLMs generate
	// compliant arguments. Providers that don't support these natively
	// should migrate them to property descriptions (see appendConstraintsToDescription).

	// MinLength constrains the minimum length of a string value.
	MinLength *int `json:"minLength,omitempty"`
	// MaxLength constrains the maximum length of a string value.
	MaxLength *int `json:"maxLength,omitempty"`
	// Pattern constrains a string value to match a regular expression.
	Pattern string `json:"pattern,omitempty"`
	// Minimum constrains the minimum value of a number.
	Minimum *float64 `json:"minimum,omitempty"`
	// Maximum constrains the maximum value of a number.
	Maximum *float64 `json:"maximum,omitempty"`
	// MinItems constrains the minimum number of items in an array.
	MinItems *int `json:"minItems,omitempty"`
	// MaxItems constrains the maximum number of items in an array.
	MaxItems *int `json:"maxItems,omitempty"`
}

// ExtraFields returns a map of JSON Schema fields not covered by the explicit
// Schema struct fields (Type, Description, Required, Properties, Items,
// AdditionalProperties, Default, Enum, Ref, Defs). This is useful for LLM
// provider adapters whose SDK types only expose a subset of JSON Schema
// fields natively but accept additional fields via an "extra fields" or
// "additional properties" mechanism (e.g. Anthropic ToolInputSchemaParam.ExtraFields).
//
// The returned map may contain: "$defs", "additionalProperties", "items",
// "enum", "default", "$ref", "minLength", "maxLength", "pattern",
// "minimum", "maximum", "minItems", "maxItems".
// Returns nil if no extra fields are present.
func (s *Schema) ExtraFields() map[string]any {
	if s == nil {
		return nil
	}
	extra := make(map[string]any)
	if s.Defs != nil {
		extra["$defs"] = s.Defs
	}
	if s.AdditionalProperties != nil {
		extra["additionalProperties"] = s.AdditionalProperties
	}
	if s.Items != nil {
		extra["items"] = s.Items
	}
	if len(s.Enum) > 0 {
		extra["enum"] = s.Enum
	}
	if s.Default != nil {
		extra["default"] = s.Default
	}
	if s.Ref != "" {
		extra["$ref"] = s.Ref
	}
	if s.MinLength != nil {
		extra["minLength"] = *s.MinLength
	}
	if s.MaxLength != nil {
		extra["maxLength"] = *s.MaxLength
	}
	if s.Pattern != "" {
		extra["pattern"] = s.Pattern
	}
	if s.Minimum != nil {
		extra["minimum"] = *s.Minimum
	}
	if s.Maximum != nil {
		extra["maximum"] = *s.Maximum
	}
	if s.MinItems != nil {
		extra["minItems"] = *s.MinItems
	}
	if s.MaxItems != nil {
		extra["maxItems"] = *s.MaxItems
	}
	if len(extra) == 0 {
		return nil
	}
	return extra
}

// AppendConstraintsToDescription appends JSON Schema constraint keywords
// (minLength, maxLength, pattern, minimum, maximum, minItems, maxItems)
// to the description string as natural language hints. This is the standard
// fallback for LLM providers whose SDK types do not natively support these
// constraint fields (e.g. Ollama api.ToolProperty).
//
// If no constraints are present, the description is returned unchanged.
func AppendConstraintsToDescription(desc string, schema *Schema) string {
	if schema == nil {
		return desc
	}
	var hints []string

	if schema.MinLength != nil {
		hints = append(hints, fmt.Sprintf("minLength: %d", *schema.MinLength))
	}
	if schema.MaxLength != nil {
		hints = append(hints, fmt.Sprintf("maxLength: %d", *schema.MaxLength))
	}
	if schema.Pattern != "" {
		hints = append(hints, fmt.Sprintf("pattern: %s", schema.Pattern))
	}
	if schema.Minimum != nil {
		hints = append(hints, fmt.Sprintf("minimum: %v", *schema.Minimum))
	}
	if schema.Maximum != nil {
		hints = append(hints, fmt.Sprintf("maximum: %v", *schema.Maximum))
	}
	if schema.MinItems != nil {
		hints = append(hints, fmt.Sprintf("minItems: %d", *schema.MinItems))
	}
	if schema.MaxItems != nil {
		hints = append(hints, fmt.Sprintf("maxItems: %d", *schema.MaxItems))
	}

	if len(hints) == 0 {
		return desc
	}
	if desc != "" {
		desc += " "
	}
	desc += "(" + strings.Join(hints, ", ") + ")"
	return desc
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
