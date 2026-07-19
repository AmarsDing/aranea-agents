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
	// BehaviorVersion identifies the tool's behavior contract version. When a
	// tool's behavior changes incompatibly, register a new ToolRegistration
	// with an incremented BehaviorVersion instead of replacing the old one, so
	// sessions pinned to an older version keep reproducible behavior.
	// Zero value means unversioned and resolves as version 1.
	BehaviorVersion int
}

// effectiveVersion normalizes the unversioned zero value to 1.
func (r *ToolRegistration) effectiveVersion() int {
	if r == nil || r.BehaviorVersion <= 0 {
		return 1
	}
	return r.BehaviorVersion
}

// ResolveRegistration picks the registration for name with the exact behavior
// version. version <= 0 means "unpinned" and resolves the latest registered
// version. Returns nil when no registration matches.
func ResolveRegistration(regs []*ToolRegistration, name string, version int) *ToolRegistration {
	if version <= 0 {
		latest := LatestBehaviorVersion(regs, name)
		if latest == 0 {
			return nil
		}
		version = latest
	}
	for _, r := range regs {
		if r != nil && r.Name == name && r.effectiveVersion() == version {
			return r
		}
	}
	return nil
}

// LatestBehaviorVersion returns the highest registered behavior version for
// name, or 0 when the tool is not registered.
func LatestBehaviorVersion(regs []*ToolRegistration, name string) int {
	latest := 0
	for _, r := range regs {
		if r != nil && r.Name == name {
			if v := r.effectiveVersion(); v > latest {
				latest = v
			}
		}
	}
	return latest
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
	tag = strings.TrimSpace(tag)
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
	category = strings.TrimSpace(category)
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
