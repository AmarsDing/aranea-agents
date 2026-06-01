package service

import (
	"context"
	"os"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/tools"
	"aranea-agents/internal/tools/cli_admin"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type cliAdminSkillRepo struct {
	uc *biz.SkillUsecase
}

func (r cliAdminSkillRepo) ListSkills(ctx context.Context, keyword string, limit, offset int32) ([]cli_admin.SkillItem, int32, error) {
	if r.uc == nil {
		return nil, 0, nil
	}
	page, err := r.uc.List(ctx, biz.SkillListQuery{
		Search: strings.TrimSpace(keyword),
		Limit:  int(limit),
		Offset: int(offset),
	})
	if err != nil {
		return nil, 0, err
	}
	items := make([]cli_admin.SkillItem, 0, len(page.Items))
	for _, s := range page.Items {
		items = append(items, skillItemFromBiz(s))
	}
	return items, int32(page.Total), nil
}

func (r cliAdminSkillRepo) GetSkill(ctx context.Context, id string) (*cli_admin.SkillItem, error) {
	if r.uc == nil {
		return nil, nil
	}
	s, err := r.uc.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	item := skillItemFromBiz(s)
	return &item, nil
}

type cliAdminAgentRepo struct {
	uc *biz.AgentUsecase
}

func (r cliAdminAgentRepo) ListAgents(ctx context.Context, keyword string, limit, offset int32) ([]cli_admin.AgentItem, int32, error) {
	if r.uc == nil {
		return nil, 0, nil
	}
	page, err := r.uc.List(ctx, biz.AgentListQuery{
		Keyword: strings.TrimSpace(keyword),
		Limit:   int(limit),
		Offset:  int(offset),
	})
	if err != nil {
		return nil, 0, err
	}
	items := make([]cli_admin.AgentItem, 0, len(page.Items))
	for _, a := range page.Items {
		items = append(items, agentItemFromBiz(a))
	}
	return items, int32(page.Total), nil
}

func (r cliAdminAgentRepo) GetAgent(ctx context.Context, id string) (*cli_admin.AgentItem, error) {
	if r.uc == nil {
		return nil, nil
	}
	a, err := r.uc.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	item := agentItemFromBiz(a)
	return &item, nil
}

func (o *ChatOrchestrator) cliAdminTools(ctx context.Context, ag biz.Agent) []trpctool.Tool {
	if o == nil || !cli_admin.IsCLIAdminAllowed(strings.TrimSpace(ag.AgentKey)) {
		return nil
	}
	return cli_admin.RegisterAll(cli_admin.Deps{
		SkillRepo:  cliAdminSkillRepo{uc: o.td.Catalog.SkillUC},
		AgentRepo:  cliAdminAgentRepo{uc: o.td.Catalog.AgentsUC},
		APIBaseURL: cliAdminAPIBaseURL(ctx, o.td.Catalog.Settings),
		APIToken:   cliAdminAPIToken(),
	})
}

func (o *ChatOrchestrator) spiritCustomTools(ag biz.Agent) []trpctool.Tool {
	if o == nil || o.spiritAssembler == nil {
		return nil
	}
	if strings.TrimSpace(ag.AgentKey) != biz.SpiritAgentKey {
		return nil
	}
	var out []trpctool.Tool
	out = append(out, tools.NewAssembleTeamTool(o.spiritAssembler, o.spiritAssembler, o.lg))
	out = append(out, tools.NewListButlersTool())
	out = append(out, tools.NewQueryButlerStatusTool())
	out = append(out, tools.NewCheckTeamProgressTool(o.spiritAssembler))
	out = append(out, tools.NewCancelTeamTool(o.spiritAssembler))
	if o.spiritSynthesis != nil {
		out = append(out, tools.NewSynthesizeResultsTool(o.spiritSynthesis))
	}
	return out
}

func skillItemFromBiz(s biz.Skill) cli_admin.SkillItem {
	version := ""
	if s.CurrentVersion != nil {
		version = s.CurrentVersion.Version
	}
	return cli_admin.SkillItem{
		ID:          s.ID,
		SkillKey:    s.Slug,
		DisplayName: s.Name,
		Status:      s.Status,
		Version:     version,
	}
}

func agentItemFromBiz(a biz.Agent) cli_admin.AgentItem {
	return cli_admin.AgentItem{
		ID:          a.ID,
		AgentKey:    a.AgentKey,
		DisplayName: a.DisplayName,
		Provider:    a.Provider,
		Model:       a.Model,
		Status:      a.Status,
	}
}

func cliAdminAPIBaseURL(ctx context.Context, sys biz.SystemSettingRepo) string {
	for _, key := range []string{"ARANEA_CLI_ADMIN_BASE_URL", "ARANEA_BASE_URL"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return strings.TrimRight(v, "/")
		}
	}
	if sys != nil {
		if s, err := sys.Get(ctx); err == nil && strings.TrimSpace(s.A2APublicBaseURL) != "" {
			return strings.TrimRight(strings.TrimSpace(s.A2APublicBaseURL), "/")
		}
	}
	return "http://127.0.0.1:8080"
}

func cliAdminAPIToken() string {
	for _, key := range []string{"ARANEA_CLI_ADMIN_TOKEN", "ARANEA_TOKEN"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return ""
}
