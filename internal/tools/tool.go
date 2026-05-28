package tools

import (
	"context"
	"strings"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type Tool = trpctool.Tool

type CallableTool = trpctool.CallableTool

type StreamableTool = trpctool.StreamableTool

type ToolSet = trpctool.ToolSet

type Declaration = trpctool.Declaration

type Schema = trpctool.Schema

type ToolRegistration struct {
	Name                 string
	Description          string
	Factory              func(ctx context.Context) (Tool, error)
	ToolSetFactory       func(ctx context.Context) (ToolSet, error)
	EnabledByDefault     bool
	Category             string
	RiskLevel            string
	RequiresConfirmation bool
	SupportsStreaming    bool
	SupportsConcurrency  bool
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
