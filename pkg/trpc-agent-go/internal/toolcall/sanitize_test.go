//
// Tencent is pleased to support the open source community by making trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//
//

package toolcall

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"trpc.group/trpc-go/trpc-agent-go/internal/util/message"
	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

type stubTool struct {
	decl *tool.Declaration
}

func (s stubTool) Declaration() *tool.Declaration { return s.decl }

func TestSanitizeMessagesWithTools_DowngradesInvalidToolCallAndResult(t *testing.T) {
	in := []model.Message{
		model.NewUserMessage("hi"),
		{
			Role: model.RoleAssistant,
			ToolCalls: []model.ToolCall{
				{
					ID: "call_1",
					Function: model.FunctionDefinitionParam{
						Name:      "test_tool",
						Arguments: []byte("{a:1}"),
					},
				},
			},
		},
		{
			Role:     model.RoleTool,
			ToolID:   "call_1",
			ToolName: "test_tool",
			Content:  "tool error",
		},
	}
	out := SanitizeMessagesWithTools(in, nil)
	if assert.Len(t, out, 3) {
		assert.Equal(t, model.RoleUser, out[0].Role)
		assert.Equal(t, model.RoleUser, out[1].Role)
		assert.Contains(t, out[1].Content, invalidToolCallTag)
		assert.Equal(t, model.RoleUser, out[2].Role)
		assert.Contains(t, out[2].Content, invalidToolResultTag)
	}
	for _, msg := range out {
		assert.NotEqual(t, model.RoleTool, msg.Role)
		assert.Empty(t, msg.ToolCalls)
	}
}

func TestSanitizeMessagesWithTools_PreservesNilMessagesSlice(t *testing.T) {
	var in []model.Message
	out := SanitizeMessagesWithTools(in, nil)
	assert.Nil(t, out)
}

func TestSanitizeMessagesWithTools_PreservesEmptyMessagesSlice(t *testing.T) {
	in := make([]model.Message, 0)
	out := SanitizeMessagesWithTools(in, nil)
	assert.NotNil(t, out)
	assert.Len(t, out, 0)
}

func TestSanitizeMessagesWithTools_PreservesValidToolRound(t *testing.T) {
	in := []model.Message{
		model.NewUserMessage("hi"),
		{
			Role: model.RoleAssistant,
			ToolCalls: []model.ToolCall{
				{
					ID: "call_1",
					Function: model.FunctionDefinitionParam{
						Name:      "test_tool",
						Arguments: []byte(`{"a":1}`),
					},
				},
			},
		},
		{
			Role:    model.RoleTool,
			ToolID:  "call_1",
			Content: `{"ok":true}`,
		},
	}
	out := SanitizeMessagesWithTools(in, nil)
	if assert.Len(t, out, 3) {
		assert.Equal(t, model.RoleAssistant, out[1].Role)
		if assert.Len(t, out[1].ToolCalls, 1) {
			assert.Equal(t, []byte(`{"a":1}`), out[1].ToolCalls[0].Function.Arguments)
		}
		assert.Equal(t, model.RoleTool, out[2].Role)
		assert.Equal(t, "call_1", out[2].ToolID)
	}
}

func TestSanitizeMessagesWithTools_DowngradesDuplicateToolResult(t *testing.T) {
	in := []model.Message{
		{
			Role: model.RoleAssistant,
			ToolCalls: []model.ToolCall{
				{
					ID: "call_1",
					Function: model.FunctionDefinitionParam{
						Name:      "test_tool",
						Arguments: []byte(`{"a":1}`),
					},
				},
			},
		},
		{
			Role:    model.RoleTool,
			ToolID:  "call_1",
			Content: "first result",
		},
		{
			Role:    model.RoleTool,
			ToolID:  "call_1",
			Content: "duplicate result",
		},
	}

	out := SanitizeMessagesWithTools(in, nil)
	if assert.Len(t, out, 3) {
		assert.Equal(t, model.RoleAssistant, out[0].Role)
		assert.Equal(t, model.RoleTool, out[1].Role)
		assert.Equal(t, "first result", out[1].Content)
		assert.Equal(t, model.RoleUser, out[2].Role)
		assert.Contains(t, out[2].Content, orphanToolResultTag)
		assert.Contains(t, out[2].Content, "duplicate result")
	}
}

func TestSanitizeMessagesWithTools_NormalizesEmptyArgumentsToEmptyObject(t *testing.T) {
	in := []model.Message{
		{
			Role: model.RoleAssistant,
			ToolCalls: []model.ToolCall{
				{
					ID: "call_1",
					Function: model.FunctionDefinitionParam{
						Name:      "no_args_tool",
						Arguments: []byte(""),
					},
				},
			},
		},
		{
			Role:   model.RoleTool,
			ToolID: "call_1",
		},
	}
	out := SanitizeMessagesWithTools(in, nil)
	if assert.Len(t, out, 2) && assert.Len(t, out[0].ToolCalls, 1) {
		assert.Equal(t, []byte("{}"), out[0].ToolCalls[0].Function.Arguments)
	}
}

func TestSanitizeMessagesWithTools_SplitsMixedValidityToolRound(t *testing.T) {
	in := []model.Message{
		{
			Role: model.RoleAssistant,
			ToolCalls: []model.ToolCall{
				{
					ID: "call_ok",
					Function: model.FunctionDefinitionParam{
						Name:      "ok_tool",
						Arguments: []byte(`{"a":1}`),
					},
				},
				{
					ID: "call_bad",
					Function: model.FunctionDefinitionParam{
						Name:      "bad_tool",
						Arguments: []byte("not-json"),
					},
				},
			},
		},
		{
			Role:   model.RoleTool,
			ToolID: "call_ok",
		},
		{
			Role:    model.RoleTool,
			ToolID:  "call_bad",
			Content: "bad tool error",
		},
	}
	out := SanitizeMessagesWithTools(in, nil)
	if assert.Len(t, out, 4) {
		assert.Equal(t, model.RoleAssistant, out[0].Role)
		if assert.Len(t, out[0].ToolCalls, 1) {
			assert.Equal(t, "call_ok", out[0].ToolCalls[0].ID)
		}
		assert.Equal(t, model.RoleTool, out[1].Role)
		assert.Equal(t, "call_ok", out[1].ToolID)
		assert.Equal(t, model.RoleUser, out[2].Role)
		assert.Contains(t, out[2].Content, invalidToolCallTag)
		assert.Equal(t, model.RoleUser, out[3].Role)
		assert.Contains(t, out[3].Content, invalidToolResultTag)
	}
}

func TestSanitizeMessagesWithTools_DowngradesOrphanToolResult(t *testing.T) {
	in := []model.Message{
		{
			Role:    model.RoleTool,
			ToolID:  "call_orphan",
			Content: "orphan",
		},
	}
	out := SanitizeMessagesWithTools(in, nil)
	if assert.Len(t, out, 1) {
		assert.Equal(t, model.RoleUser, out[0].Role)
		assert.Contains(t, out[0].Content, orphanToolResultTag)
	}
}

func TestSanitizeMessagesWithTools_DowngradesOrphanToolCall(t *testing.T) {
	in := []model.Message{
		{
			Role: model.RoleAssistant,
			ToolCalls: []model.ToolCall{
				{
					ID: "call_1",
					Function: model.FunctionDefinitionParam{
						Name:      "test_tool",
						Arguments: []byte(`"string"`),
					},
				},
			},
		},
	}
	out := SanitizeMessagesWithTools(in, nil)
	if assert.Len(t, out, 1) {
		assert.Equal(t, model.RoleUser, out[0].Role)
		assert.Contains(t, out[0].Content, orphanToolCallTag)
		assert.Contains(t, out[0].Content, "call_1")
	}
}

func TestSanitizeMessagesWithTools_DropsReasoningOnlyAssistantAfterOrphanToolCall(t *testing.T) {
	in := []model.Message{
		{
			Role:             model.RoleAssistant,
			ReasoningContent: "I should call the tool.",
			ToolCalls: []model.ToolCall{
				{
					ID: "call_1",
					Function: model.FunctionDefinitionParam{
						Name:      "test_tool",
						Arguments: []byte(`{"a":1}`),
					},
				},
			},
		},
	}

	out := SanitizeMessagesWithTools(in, nil)
	if assert.Len(t, out, 1) {
		assert.Equal(t, model.RoleUser, out[0].Role)
		assert.Contains(t, out[0].Content, orphanToolCallTag)
		assert.Contains(t, out[0].Content, "call_1")
	}
}

func TestSanitizeMessagesWithTools_SplitsMatchedAndOrphanToolCalls(t *testing.T) {
	in := []model.Message{
		{
			Role: model.RoleAssistant,
			ToolCalls: []model.ToolCall{
				{
					ID: "call_keep",
					Function: model.FunctionDefinitionParam{
						Name:      "test_tool",
						Arguments: []byte(`{"a":1}`),
					},
				},
				{
					ID: "call_orphan",
					Function: model.FunctionDefinitionParam{
						Name:      "test_tool",
						Arguments: []byte(`{"b":2}`),
					},
				},
			},
		},
		{
			Role:     model.RoleTool,
			ToolID:   "call_keep",
			ToolName: "test_tool",
			Content:  "ok",
		},
	}
	out := SanitizeMessagesWithTools(in, nil)
	if assert.Len(t, out, 3) {
		assert.Equal(t, model.RoleAssistant, out[0].Role)
		if assert.Len(t, out[0].ToolCalls, 1) {
			assert.Equal(t, "call_keep", out[0].ToolCalls[0].ID)
		}
		assert.Equal(t, model.RoleTool, out[1].Role)
		assert.Equal(t, "call_keep", out[1].ToolID)
		assert.Equal(t, model.RoleUser, out[2].Role)
		assert.Contains(t, out[2].Content, orphanToolCallTag)
		assert.Contains(t, out[2].Content, "call_orphan")
	}
}

func TestSanitizeMessagesWithTools_PreservesNonObjectJSONArgumentsWhenToolsUnknown(t *testing.T) {
	in := []model.Message{
		{
			Role: model.RoleAssistant,
			ToolCalls: []model.ToolCall{
				{
					ID: "call_1",
					Function: model.FunctionDefinitionParam{
						Name:      "test_tool",
						Arguments: []byte(`"string"`),
					},
				},
			},
		},
		{
			Role:     model.RoleTool,
			ToolID:   "call_1",
			ToolName: "test_tool",
			Content:  "ok",
		},
	}
	out := SanitizeMessagesWithTools(in, nil)
	if assert.Len(t, out, 2) {
		assert.Equal(t, model.RoleAssistant, out[0].Role)
		if assert.Len(t, out[0].ToolCalls, 1) {
			assert.Equal(t, []byte(`"string"`), out[0].ToolCalls[0].Function.Arguments)
		}
		assert.Equal(t, model.RoleTool, out[1].Role)
		assert.Equal(t, "call_1", out[1].ToolID)
	}
}

func TestSanitizeMessagesWithTools_DowngradesSchemaTypeMismatch(t *testing.T) {
	type input struct {
		A int `json:"a"`
	}
	fn := function.NewFunctionTool(
		func(context.Context, input) (string, error) { return "", nil },
		function.WithName("test_tool"),
		function.WithDescription("test tool"),
	)
	tools := map[string]tool.Tool{
		"test_tool": fn,
	}
	in := []model.Message{
		{
			Role: model.RoleAssistant,
			ToolCalls: []model.ToolCall{
				{
					ID: "call_1",
					Function: model.FunctionDefinitionParam{
						Name:      "test_tool",
						Arguments: []byte(`{"a":"not-an-int"}`),
					},
				},
			},
		},
	}
	out := SanitizeMessagesWithTools(in, tools)
	if assert.Len(t, out, 1) {
		assert.Equal(t, model.RoleUser, out[0].Role)
		assert.Contains(t, out[0].Content, invalidToolCallTag)
		assert.Contains(t, out[0].Content, "expected integer")
		assert.Contains(t, out[0].Content, "$.a")
	}
}

func TestSanitizeMessagesWithTools_DowngradesNonObjectJSONArgumentsWhenSchemaExpectsObject(t *testing.T) {
	type input struct {
		A int `json:"a"`
	}
	fn := function.NewFunctionTool(
		func(context.Context, input) (string, error) { return "", nil },
		function.WithName("test_tool"),
		function.WithDescription("test tool"),
	)
	tools := map[string]tool.Tool{
		"test_tool": fn,
	}
	in := []model.Message{
		{
			Role: model.RoleAssistant,
			ToolCalls: []model.ToolCall{
				{
					ID: "call_1",
					Function: model.FunctionDefinitionParam{
						Name:      "test_tool",
						Arguments: []byte(`"string"`),
					},
				},
			},
		},
	}
	out := SanitizeMessagesWithTools(in, tools)
	if assert.Len(t, out, 1) {
		assert.Equal(t, model.RoleUser, out[0].Role)
		assert.Contains(t, out[0].Content, invalidToolCallTag)
		assert.Contains(t, out[0].Content, "expected object")
	}
}

func TestSanitizeMessagesWithTools_PreservesStringArgumentsWhenSchemaAllows(t *testing.T) {
	fn := function.NewFunctionTool(
		func(context.Context, string) (string, error) { return "", nil },
		function.WithName("echo"),
		function.WithDescription("echo tool"),
	)
	tools := map[string]tool.Tool{
		"echo": fn,
	}
	in := []model.Message{
		{
			Role: model.RoleAssistant,
			ToolCalls: []model.ToolCall{
				{
					ID: "call_1",
					Function: model.FunctionDefinitionParam{
						Name:      "echo",
						Arguments: []byte(`"hi"`),
					},
				},
			},
		},
		{
			Role:     model.RoleTool,
			ToolID:   "call_1",
			ToolName: "echo",
			Content:  "ok",
		},
	}
	out := SanitizeMessagesWithTools(in, tools)
	if assert.Len(t, out, 2) {
		assert.Equal(t, model.RoleAssistant, out[0].Role)
		if assert.Len(t, out[0].ToolCalls, 1) {
			assert.Equal(t, []byte(`"hi"`), out[0].ToolCalls[0].Function.Arguments)
		}
		assert.Equal(t, model.RoleTool, out[1].Role)
		assert.Equal(t, "call_1", out[1].ToolID)
	}
}

func TestSanitizeMessagesWithTools_PreservesArrayArgumentsWhenSchemaAllows(t *testing.T) {
	fn := function.NewFunctionTool(
		func(context.Context, []string) ([]string, error) { return nil, nil },
		function.WithName("echo_list"),
		function.WithDescription("echo list tool"),
	)
	tools := map[string]tool.Tool{
		"echo_list": fn,
	}
	in := []model.Message{
		{
			Role: model.RoleAssistant,
			ToolCalls: []model.ToolCall{
				{
					ID: "call_1",
					Function: model.FunctionDefinitionParam{
						Name:      "echo_list",
						Arguments: []byte(`["a","b"]`),
					},
				},
			},
		},
		{
			Role:     model.RoleTool,
			ToolID:   "call_1",
			ToolName: "echo_list",
			Content:  "ok",
		},
	}
	out := SanitizeMessagesWithTools(in, tools)
	if assert.Len(t, out, 2) {
		assert.Equal(t, model.RoleAssistant, out[0].Role)
		if assert.Len(t, out[0].ToolCalls, 1) {
			assert.Equal(t, []byte(`["a","b"]`), out[0].ToolCalls[0].Function.Arguments)
		}
		assert.Equal(t, model.RoleTool, out[1].Role)
		assert.Equal(t, "call_1", out[1].ToolID)
	}
}

func TestSanitizeMessagesWithTools_PreservesNullArgumentsWhenToolsUnknown(t *testing.T) {
	in := []model.Message{
		{
			Role: model.RoleAssistant,
			ToolCalls: []model.ToolCall{
				{
					ID: "call_1",
					Function: model.FunctionDefinitionParam{
						Name:      "test_tool",
						Arguments: []byte(`null`),
					},
				},
			},
		},
		{
			Role:     model.RoleTool,
			ToolID:   "call_1",
			ToolName: "test_tool",
			Content:  "ok",
		},
	}
	out := SanitizeMessagesWithTools(in, nil)
	if assert.Len(t, out, 2) {
		assert.Equal(t, model.RoleAssistant, out[0].Role)
		if assert.Len(t, out[0].ToolCalls, 1) {
			assert.Equal(t, []byte(`null`), out[0].ToolCalls[0].Function.Arguments)
		}
		assert.Equal(t, model.RoleTool, out[1].Role)
		assert.Equal(t, "call_1", out[1].ToolID)
	}
}

func TestSanitizeMessagesWithTools_DowngradesNullArgumentsWhenSchemaExpectsObject(t *testing.T) {
	type input struct {
		A int `json:"a"`
	}
	fn := function.NewFunctionTool(
		func(context.Context, input) (string, error) { return "", nil },
		function.WithName("test_tool"),
		function.WithDescription("test tool"),
	)
	tools := map[string]tool.Tool{
		"test_tool": fn,
	}
	in := []model.Message{
		{
			Role: model.RoleAssistant,
			ToolCalls: []model.ToolCall{
				{
					ID: "call_1",
					Function: model.FunctionDefinitionParam{
						Name:      "test_tool",
						Arguments: []byte(`null`),
					},
				},
			},
		},
	}
	out := SanitizeMessagesWithTools(in, tools)
	if assert.Len(t, out, 1) {
		assert.Equal(t, model.RoleUser, out[0].Role)
		assert.Contains(t, out[0].Content, invalidToolCallTag)
		assert.Contains(t, out[0].Content, "expected object")
		assert.Contains(t, out[0].Content, "$")
	}
}

func TestSanitizeMessagesWithTools_PreservesNullArgumentsWhenSchemaAllowsNull(t *testing.T) {
	fn := function.NewFunctionTool(
		func(context.Context, any) (string, error) { return "", nil },
		function.WithName("nil_tool"),
		function.WithDescription("nil tool"),
		function.WithInputSchema(&tool.Schema{Type: "null"}),
	)
	tools := map[string]tool.Tool{
		"nil_tool": fn,
	}
	in := []model.Message{
		{
			Role: model.RoleAssistant,
			ToolCalls: []model.ToolCall{
				{
					ID: "call_1",
					Function: model.FunctionDefinitionParam{
						Name:      "nil_tool",
						Arguments: []byte(`null`),
					},
				},
			},
		},
		{
			Role:     model.RoleTool,
			ToolID:   "call_1",
			ToolName: "nil_tool",
			Content:  "ok",
		},
	}
	out := SanitizeMessagesWithTools(in, tools)
	if assert.Len(t, out, 2) {
		assert.Equal(t, model.RoleAssistant, out[0].Role)
		if assert.Len(t, out[0].ToolCalls, 1) {
			assert.Equal(t, []byte(`null`), out[0].ToolCalls[0].Function.Arguments)
		}
		assert.Equal(t, model.RoleTool, out[1].Role)
		assert.Equal(t, "call_1", out[1].ToolID)
	}
}

func TestResolveSchemaRef(t *testing.T) {
	defs := map[string]*tool.Schema{
		"Input": {Type: "object"},
	}
	assert.NotNil(t, resolveSchemaRef("#/$defs/Input", defs))
	assert.Nil(t, resolveSchemaRef("#/$defs/Missing", defs))
	assert.Nil(t, resolveSchemaRef("#/$defs/", defs))
	assert.Nil(t, resolveSchemaRef("https://example.com/schema.json", defs))
	assert.Nil(t, resolveSchemaRef("#/$defs/Input", nil))
}

func TestInferSchemaType(t *testing.T) {
	assert.Equal(t, "boolean", inferSchemaType(&tool.Schema{Type: "boolean"}))
	assert.Equal(t, "object", inferSchemaType(&tool.Schema{Properties: map[string]*tool.Schema{"a": {Type: "string"}}}))
	assert.Equal(t, "array", inferSchemaType(&tool.Schema{Items: &tool.Schema{Type: "string"}}))
	assert.Equal(t, "", inferSchemaType(&tool.Schema{}))
	assert.Equal(t, "", inferSchemaType(nil))
}

func TestValidateArgumentsAgainstSchema_NullArgsRefToObject(t *testing.T) {
	schema := &tool.Schema{
		Ref: "#/$defs/Input",
		Defs: map[string]*tool.Schema{
			"Input": {Properties: map[string]*tool.Schema{"a": {Type: "integer"}}},
		},
	}
	ok, reason := validateArgumentsAgainstSchema(nil, schema)
	assert.False(t, ok)
	assert.Contains(t, reason, "expected object")
}

func TestValidateArgumentsAgainstSchema_NullArgsUnknownRef(t *testing.T) {
	schema := &tool.Schema{
		Ref:  "#/$defs/Missing",
		Defs: map[string]*tool.Schema{"Other": {Type: "object"}},
	}
	ok, reason := validateArgumentsAgainstSchema(nil, schema)
	assert.True(t, ok)
	assert.Empty(t, reason)
}

func TestValidateArgumentsAgainstSchema_NullArgsSchemaTypes(t *testing.T) {
	tests := []struct {
		schema *tool.Schema
		substr string
	}{
		{schema: &tool.Schema{Type: "array"}, substr: "expected array"},
		{schema: &tool.Schema{Type: "string"}, substr: "expected string"},
		{schema: &tool.Schema{Type: "boolean"}, substr: "expected boolean"},
		{schema: &tool.Schema{Type: "integer"}, substr: "expected integer"},
		{schema: &tool.Schema{Type: "number"}, substr: "expected number"},
	}
	for _, tt := range tests {
		ok, reason := validateArgumentsAgainstSchema(nil, tt.schema)
		assert.False(t, ok)
		assert.Contains(t, reason, tt.substr)
	}
}

func TestValidateToolCallArguments_SkipsWhenDeclarationMissing(t *testing.T) {
	tests := []struct {
		name  string
		tools map[string]tool.Tool
	}{
		{
			name: "nil declaration",
			tools: map[string]tool.Tool{
				"t": stubTool{decl: nil},
			},
		},
		{
			name: "nil input schema",
			tools: map[string]tool.Tool{
				"t": stubTool{decl: &tool.Declaration{Name: "t", InputSchema: nil}},
			},
		},
		{
			name:  "tool missing",
			tools: map[string]tool.Tool{},
		},
	}
	for _, tt := range tests {
		ok, reason := validateToolCallArguments("t", map[string]any{"a": 1}, tt.tools)
		assert.True(t, ok)
		assert.Empty(t, reason)
	}
}

func TestValidateValueAgainstSchema_ScalarTypes(t *testing.T) {
	ok, reason := validateValueAgainstSchema(true, &tool.Schema{Type: "boolean"}, nil, "$")
	assert.True(t, ok)
	assert.Empty(t, reason)

	ok, reason = validateValueAgainstSchema("x", &tool.Schema{Type: "boolean"}, nil, "$")
	assert.False(t, ok)
	assert.Contains(t, reason, "expected boolean")

	ok, reason = validateValueAgainstSchema(json.Number("1.25"), &tool.Schema{Type: "number"}, nil, "$")
	assert.True(t, ok)
	assert.Empty(t, reason)

	ok, reason = validateValueAgainstSchema(json.Number("1e309"), &tool.Schema{Type: "number"}, nil, "$")
	assert.False(t, ok)
	assert.Contains(t, reason, "expected number")

	ok, reason = validateValueAgainstSchema(json.Number("1.25"), &tool.Schema{Type: "integer"}, nil, "$")
	assert.False(t, ok)
	assert.Contains(t, reason, "expected integer")

	ok, reason = validateValueAgainstSchema(1.0, &tool.Schema{Type: "integer"}, nil, "$")
	assert.False(t, ok)
	assert.Contains(t, reason, "expected integer")
}

func TestValidateValueAgainstSchema_ArrayItemsNil(t *testing.T) {
	ok, reason := validateValueAgainstSchema([]any{json.Number("1")}, &tool.Schema{Type: "array"}, nil, "$")
	assert.True(t, ok)
	assert.Empty(t, reason)
}

func TestValidateValueAgainstSchema_ArrayTypeMismatch(t *testing.T) {
	ok, reason := validateValueAgainstSchema(map[string]any{}, &tool.Schema{Type: "array"}, nil, "$")
	assert.False(t, ok)
	assert.Contains(t, reason, "expected array")
}

func TestSplitToolResults_GroupsByIDs(t *testing.T) {
	toolResults := []model.Message{
		{Role: model.RoleTool, ToolID: ""},
		{Role: model.RoleTool, ToolID: "valid"},
		{Role: model.RoleTool, ToolID: "invalid"},
		{Role: model.RoleTool, ToolID: "unknown"},
	}
	validIDs := map[string]struct{}{"valid": {}}
	invalidIDs := map[string]struct{}{"invalid": {}}
	split := splitToolResults(toolResults, validIDs, invalidIDs)
	assert.Len(t, split.kept, 1)
	assert.Len(t, split.invalidByID["invalid"], 1)
	assert.Len(t, split.orphan, 2)
}

func TestIsEmptyAssistantMessage(t *testing.T) {
	assert.True(t, message.IsEmptyAssistantMessage(model.Message{Role: model.RoleAssistant}))
	assert.False(t, message.IsEmptyAssistantMessage(model.Message{Role: model.RoleUser}))
	assert.False(t, message.IsEmptyAssistantMessage(model.Message{Role: model.RoleAssistant, Content: "x"}))
	assert.True(t, message.IsEmptyAssistantMessage(model.Message{Role: model.RoleAssistant, ReasoningContent: "x"}))
	assert.False(t, message.IsEmptyAssistantMessage(model.Message{Role: model.RoleAssistant, ToolCalls: []model.ToolCall{{ID: "call_1"}}}))
}

// TestSanitizeMessagesWithTools_HookAppendedUserAfterPairedRound simulates the
// scenario where a BeforeModel hook appends a user message after a fully paired
// assistant(tool_calls) + tool(tool_call_id) round. The pairing is intact, so
// sanitize should preserve the sequence as-is.
func TestSanitizeMessagesWithTools_HookAppendedUserAfterPairedRound(t *testing.T) {
	in := []model.Message{
		model.NewUserMessage("question"),
		{
			Role: model.RoleAssistant,
			ToolCalls: []model.ToolCall{
				{
					ID: "call_1",
					Function: model.FunctionDefinitionParam{
						Name:      "test_tool",
						Arguments: []byte(`{"a":1}`),
					},
				},
			},
		},
		{
			Role:    model.RoleTool,
			ToolID:  "call_1",
			Content: "result",
		},
		model.NewUserMessage("hook-appended-user"),
	}
	out := SanitizeMessagesWithTools(in, nil)
	if assert.Len(t, out, 4) {
		assert.Equal(t, model.RoleUser, out[0].Role)
		assert.Equal(t, model.RoleAssistant, out[1].Role)
		if assert.Len(t, out[1].ToolCalls, 1) {
			assert.Equal(t, "call_1", out[1].ToolCalls[0].ID)
		}
		assert.Equal(t, model.RoleTool, out[2].Role)
		assert.Equal(t, "call_1", out[2].ToolID)
		assert.Equal(t, model.RoleUser, out[3].Role)
	}
}

// TestSanitizeMessagesWithTools_OrphanToolCallFollowedByUser simulates the
// scenario where token tailoring or track restoration drops a tool result,
// leaving assistant(tool_calls) followed by a user message. Sanitize must
// downgrade the orphan tool_call to a user message so that DeepSeek does not
// see assistant(tool_calls) followed by a non-tool message.
func TestSanitizeMessagesWithTools_OrphanToolCallFollowedByUser(t *testing.T) {
	in := []model.Message{
		model.NewUserMessage("question"),
		{
			Role: model.RoleAssistant,
			ToolCalls: []model.ToolCall{
				{
					ID: "call_orphan",
					Function: model.FunctionDefinitionParam{
						Name:      "test_tool",
						Arguments: []byte(`{"a":1}`),
					},
				},
			},
		},
		model.NewUserMessage("next-turn"),
	}
	out := SanitizeMessagesWithTools(in, nil)
	// The orphan tool_call must be downgraded; the assistant (no content,
	// no tool_calls after filtering) is dropped because IsEmptyAssistantMessage
	// returns true.
	if assert.Len(t, out, 3) {
		assert.Equal(t, model.RoleUser, out[0].Role)
		assert.Equal(t, model.RoleUser, out[1].Role)
		assert.Contains(t, out[1].Content, orphanToolCallTag)
		assert.Contains(t, out[1].Content, "call_orphan")
		assert.Equal(t, model.RoleUser, out[2].Role)
	}
	// Verify no assistant with tool_calls remains.
	for _, msg := range out {
		if msg.Role == model.RoleAssistant {
			assert.Empty(t, msg.ToolCalls)
		}
	}
}

// TestSanitizeMessagesWithTools_OrphanToolCallWithContentFollowedByUser
// simulates the scenario where assistant has both content and tool_calls,
// but the tool result is missing. The assistant should be preserved (with
// tool_calls cleared) and the orphan tool_call downgraded to user.
func TestSanitizeMessagesWithTools_OrphanToolCallWithContentFollowedByUser(t *testing.T) {
	in := []model.Message{
		model.NewUserMessage("question"),
		{
			Role:    model.RoleAssistant,
			Content: "I will call the tool",
			ToolCalls: []model.ToolCall{
				{
					ID: "call_orphan",
					Function: model.FunctionDefinitionParam{
						Name:      "test_tool",
						Arguments: []byte(`{"a":1}`),
					},
				},
			},
		},
		model.NewUserMessage("next-turn"),
	}
	out := SanitizeMessagesWithTools(in, nil)
	if assert.Len(t, out, 4) {
		assert.Equal(t, model.RoleUser, out[0].Role)
		assert.Equal(t, "question", out[0].Content)
		assert.Equal(t, model.RoleAssistant, out[1].Role)
		assert.Equal(t, "I will call the tool", out[1].Content)
		assert.Empty(t, out[1].ToolCalls)
		assert.Equal(t, model.RoleUser, out[2].Role)
		assert.Contains(t, out[2].Content, orphanToolCallTag)
		assert.Contains(t, out[2].Content, "call_orphan")
		assert.Equal(t, model.RoleUser, out[3].Role)
		assert.Equal(t, "next-turn", out[3].Content)
	}
}

// TestSanitizeMessagesWithTools_MiddleOrphanToolCall simulates the scenario
// where rearrangeAsyncFuncRespHist produces a sequence with an orphan
// tool_call in the middle (tool result lost). Sanitize must downgrade it.
func TestSanitizeMessagesWithTools_MiddleOrphanToolCall(t *testing.T) {
	in := []model.Message{
		model.NewUserMessage("first"),
		{
			Role: model.RoleAssistant,
			ToolCalls: []model.ToolCall{
				{
					ID: "call_lost",
					Function: model.FunctionDefinitionParam{
						Name:      "test_tool",
						Arguments: []byte(`{"a":1}`),
					},
				},
			},
		},
		model.NewUserMessage("second"),
		{
			Role: model.RoleAssistant,
			ToolCalls: []model.ToolCall{
				{
					ID: "call_ok",
					Function: model.FunctionDefinitionParam{
						Name:      "test_tool",
						Arguments: []byte(`{"b":2}`),
					},
				},
			},
		},
		{
			Role:    model.RoleTool,
			ToolID:  "call_ok",
			Content: "ok",
		},
	}
	out := SanitizeMessagesWithTools(in, nil)
	// call_lost has no tool result → downgraded to user.
	// call_ok has tool result → preserved.
	for _, msg := range out {
		if msg.Role == model.RoleAssistant {
			for _, tc := range msg.ToolCalls {
				assert.NotEqual(t, "call_lost", tc.ID,
					"orphan tool_call call_lost must not survive sanitize")
			}
		}
	}
	// Verify call_ok pairing is intact.
	var foundPaired bool
	for i := 0; i < len(out)-1; i++ {
		if out[i].Role == model.RoleAssistant && len(out[i].ToolCalls) > 0 &&
			out[i].ToolCalls[0].ID == "call_ok" &&
			out[i+1].Role == model.RoleTool && out[i+1].ToolID == "call_ok" {
			foundPaired = true
		}
	}
	assert.True(t, foundPaired, "call_ok pairing must be preserved")
}

// TestSanitizeMessagesWithTools_Idempotent verifies that calling sanitize
// multiple times produces the same result. This is critical because the
// defensive guard in prepareChatRequest calls sanitize after it has already
// run in preprocess.
func TestSanitizeMessagesWithTools_Idempotent(t *testing.T) {
	in := []model.Message{
		model.NewUserMessage("q"),
		{
			Role: model.RoleAssistant,
			ToolCalls: []model.ToolCall{
				{
					ID: "call_1",
					Function: model.FunctionDefinitionParam{
						Name:      "test_tool",
						Arguments: []byte(`{"a":1}`),
					},
				},
				{
					ID: "call_orphan",
					Function: model.FunctionDefinitionParam{
						Name:      "test_tool",
						Arguments: []byte(`{"b":2}`),
					},
				},
			},
		},
		{
			Role:    model.RoleTool,
			ToolID:  "call_1",
			Content: "result",
		},
		model.NewUserMessage("next"),
	}
	first := SanitizeMessagesWithTools(in, nil)
	second := SanitizeMessagesWithTools(first, nil)
	assert.Equal(t, first, second,
		"sanitize must be idempotent; second run must not change output")
}

// TestSanitizeMessagesWithTools_NoAssistantToolCallsFollowedByNonTool is a
// property-based check: after sanitize, no assistant message with tool_calls
// should be immediately followed by a non-tool message. This is the exact
// invariant DeepSeek 400 enforces.
func TestSanitizeMessagesWithTools_NoAssistantToolCallsFollowedByNonTool(t *testing.T) {
	cases := [][]model.Message{
		// hook appended user after orphan tool_call
		{
			model.NewUserMessage("q"),
			{Role: model.RoleAssistant, ToolCalls: []model.ToolCall{{ID: "x",
				Function: model.FunctionDefinitionParam{Name: "t", Arguments: []byte(`{}`)}}}},
			model.NewUserMessage("hook-user"),
		},
		// middle orphan with content
		{
			model.NewUserMessage("q"),
			{Role: model.RoleAssistant, Content: "c", ToolCalls: []model.ToolCall{{ID: "x",
				Function: model.FunctionDefinitionParam{Name: "t", Arguments: []byte(`{}`)}}}},
			model.NewUserMessage("u"),
			{Role: model.RoleAssistant, ToolCalls: []model.ToolCall{{ID: "y",
				Function: model.FunctionDefinitionParam{Name: "t", Arguments: []byte(`{}`)}}}},
			{Role: model.RoleTool, ToolID: "y", Content: "r"},
		},
		// valid round + hook user
		{
			model.NewUserMessage("q"),
			{Role: model.RoleAssistant, ToolCalls: []model.ToolCall{{ID: "x",
				Function: model.FunctionDefinitionParam{Name: "t", Arguments: []byte(`{}`)}}}},
			{Role: model.RoleTool, ToolID: "x", Content: "r"},
			model.NewUserMessage("hook-user"),
		},
	}
	for i, in := range cases {
		out := SanitizeMessagesWithTools(in, nil)
		for j := 0; j < len(out)-1; j++ {
			if out[j].Role == model.RoleAssistant && len(out[j].ToolCalls) > 0 {
				assert.Equal(t, model.RoleTool, out[j+1].Role,
					"case %d: assistant(tool_calls) at index %d must be followed by tool, got %s",
					i, j, out[j+1].Role)
			}
		}
		// Last message must not be assistant with tool_calls.
		if len(out) > 0 {
			last := out[len(out)-1]
			if last.Role == model.RoleAssistant {
				assert.Empty(t, last.ToolCalls,
					"case %d: last message must not have tool_calls", i)
			}
		}
	}
}
