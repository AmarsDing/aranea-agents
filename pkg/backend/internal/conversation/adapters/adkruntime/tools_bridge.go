package adkruntime

import (
	"context"
	"strings"

	"arenea/backend/internal/domain"
	"arenea/backend/internal/capability/adkbridge"
	"arenea/backend/internal/capability/executor"
	toolmw "arenea/backend/internal/capability/middleware"
	"arenea/backend/internal/capability/registry"
	tooltelemetry "arenea/backend/internal/capability/telemetry"
	"arenea/backend/internal/capability/toolctx"
	"arenea/backend/internal/capability/tooldef"
	"google.golang.org/adk/tool"
)

type ToolCatalogSource interface {
	SearchTools(domain.ToolListQuery) (domain.ToolListResult, error)
}

func (a *ADKRuntimeAdapter) SetToolCatalogSource(source ToolCatalogSource) {
	a.toolCatalogSource = source
}

func (a *ADKRuntimeAdapter) runtimeTools(ctx context.Context, req GenerateRequest) ([]tool.Tool, error) {
	reg := registry.Builtins()
	available := reg.List()
	catalog := map[string]domain.Tool{}
	if a.toolCatalogSource != nil {
		result, err := a.toolCatalogSource.SearchTools(domain.ToolListQuery{Limit: 1000})
		if err != nil {
			return nil, err
		}
		for _, item := range result.Items {
			catalog[item.Key] = item
		}
	}

	baseCtx := toolctx.New(ctx)
	baseCtx.AgentID = req.Agent.ID
	baseCtx.AgentKey = req.Agent.AgentKey
	baseCtx.AgentName = firstNonEmpty(req.Agent.DisplayName, req.Agent.AgentKey, req.Agent.ID)
	baseCtx.AgentIcon = req.Agent.Icon
	if req.RuntimeContext != nil {
		baseCtx.SessionID = req.RuntimeContext.Session.SessionID
	}
	emit := func(event tooldef.Event) error {
		if req.OnToolEvent == nil {
			return nil
		}
		return req.OnToolEvent(toolEventFromRuntime(req, event))
	}
	exec := executor.New(
		toolmw.Tracing(tooltelemetry.DefaultProvider()),
		toolmw.Validation(),
		toolmw.Budget(toolmw.NewBudgetState(), toolmw.DefaultCallBudget, toolmw.DefaultFailureBudgetArg, emit),
	)

	out := make([]tool.Tool, 0, len(available))
	for _, rt := range available {
		if rt == nil {
			continue
		}
		if row, ok := catalog[rt.Name()]; ok && !row.Enabled {
			continue
		}
		if !runtimeToolAllowed(rt.Name(), req.ToolSettings) {
			continue
		}
		adkTool, err := adkbridge.ToADKTool(rt, adkbridge.Options{BaseContext: baseCtx, Executor: exec})
		if err != nil {
			return nil, err
		}
		out = append(out, adkTool)
	}
	return out, nil
}

func runtimeToolAllowed(name string, settings *domain.AgentRuntimeSettings) bool {
	if settings == nil {
		return true
	}
	if !settings.ToolsEnabled {
		return false
	}
	profile := strings.TrimSpace(settings.ToolsProfile)
	allow := toolmw.ToolSet(toolmw.JSONStringList(settings.ToolsAllowJSON))
	deny := toolmw.ToolSet(toolmw.JSONStringList(settings.ToolsDenyJSON))
	if deny[name] {
		return false
	}
	return profile == "" || profile == "full" || toolmw.ProfileAllows(profile, name) || allow[name]
}

func toolEventFromRuntime(req GenerateRequest, event tooldef.Event) ToolEvent {
	out := ToolEvent{
		ID:          event.ID,
		Phase:       event.Phase,
		Status:      event.Status,
		AgentID:     req.Agent.ID,
		AgentKey:    req.Agent.AgentKey,
		AgentName:   firstNonEmpty(req.Agent.DisplayName, req.Agent.AgentKey, req.Agent.ID),
		AgentIcon:   req.Agent.Icon,
		ToolName:    event.ToolName,
		ToolLabel:   event.ToolLabel,
		Arguments:   event.Arguments,
		Result:      event.Result,
		Error:       event.Error,
		OccurredAt:  event.OccurredAt.UTC().Format(timeFormatRFC3339Nano),
		DurationMS:  event.DurationMS,
		MessageHint: event.MessageHint,
	}
	return out
}

const timeFormatRFC3339Nano = "2006-01-02T15:04:05.999999999Z07:00"
