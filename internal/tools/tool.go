package tools

import (
	"context"
	"encoding/json"
	"strings"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type Tool = trpctool.Tool

type CallableTool = trpctool.CallableTool

type StreamableTool = trpctool.StreamableTool

type ToolSet = trpctool.ToolSet

type Declaration = trpctool.Declaration

type Schema = trpctool.Schema

type ToolUseExample struct {
	UserQuery   string          `json:"user_query"`
	ToolName    string          `json:"tool_name"`
	Arguments   json.RawMessage `json:"arguments,omitempty"`
	Explanation string          `json:"explanation,omitempty"`
}

type ToolRegistration struct {
	Name                 string
	Description          string
	Factory              func(ctx context.Context) (Tool, error)
	ToolSetFactory       func(ctx context.Context) (ToolSet, error)
	EnabledByDefault     bool
	Category             string
	Tags                 []string
	RiskLevel            string
	RequiresConfirmation bool
	SupportsStreaming    bool
	SupportsConcurrency  bool
	Deferred             bool
	Examples             []ToolUseExample
	Group                string
}

func NewToolRegistration(name, description string, factory func(ctx context.Context) (Tool, error)) *ToolRegistration {
	return &ToolRegistration{
		Name:             name,
		Description:      description,
		Factory:          factory,
		EnabledByDefault: false,
	}
}

func NewToolSetRegistration(name, description string, factory func(ctx context.Context) (ToolSet, error)) *ToolRegistration {
	return &ToolRegistration{
		Name:             name,
		Description:      description,
		ToolSetFactory:   factory,
		EnabledByDefault: false,
	}
}

func ConfigString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func RegistryByTag(tag string) []*ToolRegistration {
	tag = strings.TrimSpace(strings.ToLower(tag))
	if tag == "" {
		return nil
	}
	var out []*ToolRegistration
	for _, reg := range Registry() {
		for _, t := range reg.Tags {
			if strings.EqualFold(t, tag) {
				out = append(out, reg)
				break
			}
		}
	}
	return out
}

func RegistryByCategory(category string) []*ToolRegistration {
	category = strings.TrimSpace(strings.ToLower(category))
	if category == "" {
		return nil
	}
	var out []*ToolRegistration
	for _, reg := range Registry() {
		if strings.EqualFold(reg.Category, category) {
			out = append(out, reg)
		}
	}
	return out
}
