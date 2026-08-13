package service

import (
	v1 "aranea-agents/api/kratos/agent/v1"
	"aranea-agents/internal/biz"
)

func fromProtoFile(pb *v1.AgentPromptFile) biz.AgentPromptFile {
	if pb == nil {
		return biz.AgentPromptFile{}
	}
	return biz.AgentPromptFile{
		ID:        pb.GetId(),
		AgentID:   pb.GetAgentId(),
		Name:      pb.GetName(),
		Body:      pb.GetBody(),
		SortOrder: int(pb.GetSortOrder()),
		CreatedAt: pb.GetCreatedAt(),
		UpdatedAt: pb.GetUpdatedAt(),
	}
}

func toProtoFile(b biz.AgentPromptFile) *v1.AgentPromptFile {
	return &v1.AgentPromptFile{
		Id:        b.ID,
		AgentId:   b.AgentID,
		Name:      b.Name,
		Body:      b.Body,
		SortOrder: int32(b.SortOrder),
		CreatedAt: b.CreatedAt,
		UpdatedAt: b.UpdatedAt,
	}
}

func fromProtoA2AProxy(pb *v1.A2AProxyConfig) *biz.A2AProxyConfig {
	if pb == nil {
		return nil
	}
	cfg := &biz.A2AProxyConfig{
		RemoteURL:       pb.GetRemoteUrl(),
		AgentCardURL:    pb.GetAgentCardUrl(),
		EnableStreaming: pb.GetEnableStreaming(),
		AuthType:        pb.GetAuthType(),
		AuthConfigJSON:  pb.GetAuthConfigJson(),
		TimeoutSeconds:  int(pb.GetTimeoutSeconds()),
	}
	if cfg.RemoteURL == "" && cfg.AgentCardURL == "" {
		return nil
	}
	return cfg
}

func toProtoA2AProxy(cfg *biz.A2AProxyConfig) *v1.A2AProxyConfig {
	if cfg == nil {
		return nil
	}
	return &v1.A2AProxyConfig{
		RemoteUrl:       cfg.RemoteURL,
		AgentCardUrl:    cfg.AgentCardURL,
		EnableStreaming: cfg.EnableStreaming,
		AuthType:        cfg.AuthType,
		AuthConfigJson:  cfg.AuthConfigJSON,
		TimeoutSeconds:  int32(cfg.TimeoutSeconds),
	}
}

func fromProtoAgent(pb *v1.Agent) biz.Agent {
	if pb == nil {
		return biz.Agent{}
	}
	a := biz.Agent{
		ID:                 pb.GetId(),
		AgentKey:           pb.GetAgentKey(),
		DisplayName:        pb.GetDisplayName(),
		Provider:           pb.GetProvider(),
		Model:              pb.GetModel(),
		Status:             pb.GetStatus(),
		IsDefault:          biz.BoolPtr(pb.GetIsDefault()),
		IsFavorite:         biz.BoolPtr(pb.GetIsFavorite()),
		Icon:               pb.GetIcon(),
		AgentDescription:   pb.GetAgentDescription(),
		PositionID:         pb.GetPositionId(),
		PositionKey:        pb.GetPositionKey(),
		AgentVariant:       pb.GetAgentVariant(),
		VariantDescription: pb.GetVariantDescription(),
		SystemPromptMode:   pb.GetSystemPromptMode(),
		ContextWindow:      int(pb.GetContextWindow()),
		BudgetMonthlyCents: int(pb.GetBudgetMonthlyCents()),
		ConfigJSON:         pb.GetConfigJson(),
		CreatedAt:          pb.GetCreatedAt(),
		UpdatedAt:          pb.GetUpdatedAt(),
		DeletedAt:          pb.GetDeletedAt(),
		Kind:               pb.GetKind(),
		Source:             pb.GetSource(),
		AgentKind:          pb.GetAgentKind(),
		A2AProxy:           fromProtoA2AProxy(pb.GetA2AProxyConfig()),
		Readonly:           pb.GetReadonly(),
	}
	biz.HydrateAgentKind(&a)
	if s := fromProtoRuntime(pb.GetSettings()); s != nil {
		a.Settings = s
	}
	for _, f := range pb.GetFiles() {
		a.Files = append(a.Files, fromProtoFile(f))
	}
	return a
}

func toProtoAgent(b biz.Agent) *v1.Agent {
	biz.HydrateAgentKind(&b)
	out := &v1.Agent{
		Id:                    b.ID,
		AgentKey:              b.AgentKey,
		DisplayName:           b.DisplayName,
		Provider:              b.Provider,
		Model:                 b.Model,
		Status:                b.Status,
		IsDefault:             biz.BoolVal(b.IsDefault),
		IsFavorite:            biz.BoolVal(b.IsFavorite),
		Icon:                  b.Icon,
		AgentDescription:      b.AgentDescription,
		PositionId:            b.PositionID,
		PositionKey:           b.PositionKey,
		AgentVariant:          b.AgentVariant,
		VariantDescription:    b.VariantDescription,
		SystemPromptMode:      b.SystemPromptMode,
		ContextWindow:         int32(b.ContextWindow),
		BudgetMonthlyCents:    int32(b.BudgetMonthlyCents),
		ConfigJson:            b.ConfigJSON,
		CreatedAt:             b.CreatedAt,
		UpdatedAt:             b.UpdatedAt,
		DeletedAt:             b.DeletedAt,
		Settings:              toProtoRuntime(b.Settings),
		AgentKind:             b.AgentKind,
		A2AProxyConfig:        toProtoA2AProxy(b.A2AProxy),
		A2AEndpointEnabled:    b.A2AEndpointEnabled,
		LastRunStatus:         b.LastRunStatus,
		LastRunAt:             b.LastRunAt,
		PendingEvolutionCount: int32(b.PendingEvolutionCount),
		CreatedBy:             b.CreatedBy,
		Readonly:              b.Readonly,
		Source:                b.Source,
		Kind:                  b.Kind,
	}
	for i := range b.Files {
		out.Files = append(out.Files, toProtoFile(b.Files[i]))
	}
	return out
}

func fromProtoCreate(req *v1.CreateAgentRequest) biz.Agent {
	if req == nil {
		return biz.Agent{}
	}
	a := biz.Agent{
		AgentKey:           req.GetAgentKey(),
		DisplayName:        req.GetDisplayName(),
		Provider:           req.GetProvider(),
		Model:              req.GetModel(),
		Icon:               req.GetIcon(),
		AgentDescription:   req.GetAgentDescription(),
		PositionID:         req.GetPositionId(),
		PositionKey:        req.GetPositionKey(),
		AgentVariant:       req.GetAgentVariant(),
		VariantDescription: req.GetVariantDescription(),
		SystemPromptMode:   req.GetSystemPromptMode(),
		ContextWindow:      int(req.GetContextWindow()),
		BudgetMonthlyCents: int(req.GetBudgetMonthlyCents()),
		ConfigJSON:         req.GetConfigJson(),
		Kind:               "user", // ownership: user-created agents are always "user"
		Source:             "user",
		AgentKind:          req.GetAgentKind(), // technical type: llm | a2a_proxy
		A2AProxy:           fromProtoA2AProxy(req.GetA2AProxyConfig()),
	}
	biz.HydrateAgentKind(&a)
	if s := fromProtoRuntime(req.GetSettings()); s != nil {
		a.Settings = s
	}
	for _, f := range req.GetFiles() {
		a.Files = append(a.Files, fromProtoFile(f))
	}
	return a
}

func bizEffectiveToolsToProto(in biz.AgentEffectiveTools) *v1.AgentEffectiveToolsView {
	items := make([]*v1.EffectiveAgentTool, 0, len(in.Items))
	for _, row := range in.Items {
		items = append(items, &v1.EffectiveAgentTool{
			ToolKey:        row.ToolKey,
			DisplayName:    row.DisplayName,
			Category:       row.Category,
			Source:         row.Source,
			Enabled:        row.Enabled,
			EffectiveState: row.EffectiveState,
			Reason:         row.Reason,
		})
	}
	return &v1.AgentEffectiveToolsView{
		ToolsEnabled: in.ToolsEnabled,
		Profile:      in.Profile,
		Allow:        append([]string(nil), in.Allow...),
		Deny:         append([]string(nil), in.Deny...),
		Items:        items,
	}
}
