package data

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/agent"
	"aranea-agents/internal/data/ent/organization"
	systemprompts "aranea-agents/internal/scenario/system"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

// SeedCompanyLeadAgents backfills 总经理办公室 / 总经理岗 / company_lead Agent
// for every company node at startup (not only when Tree/List is opened).
func SeedCompanyLeadAgents(ctx context.Context, client *ent.Client, d Dialect, lg loggateway.Logger) error {
	if client == nil {
		return nil
	}
	companies, err := client.Organization.Query().
		Where(organization.DeletedAtEQ(""), organization.LevelEQ("company")).
		All(ctx)
	if err != nil {
		lg.Error("seed company leads: query companies failed",
			loggateway.StepID("data.seed.company_lead_agents"),
			loggateway.Err(err))
		return entErrToBizErr(err, "SEED")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	created := 0
	for _, company := range companies {
		if err := seedOneCompanyLead(ctx, client, d, lg, company, now); err != nil {
			lg.Error("seed company lead failed",
				loggateway.StepID("data.seed.company_lead_agents"),
				loggateway.Str("company_key", company.OrgKey),
				loggateway.Err(err))
			continue
		}
		created++
	}
	lg.Info("company lead seed done",
		loggateway.StepID("data.seed.company_lead_agents"),
		loggateway.Int("companies", len(companies)),
		loggateway.Int("processed", created),
	)
	return nil
}

func seedOneCompanyLead(ctx context.Context, client *ent.Client, d Dialect, lg loggateway.Logger, company *ent.Organization, now string) error {
	office, err := ensureSeedOrgNode(ctx, client, lg, orgSeedSpec{
		key:         biz.CompanyOfficeDeptKey(company.OrgKey),
		name:        biz.CompanyOfficeDeptName,
		level:       "department",
		parentID:    company.ID,
		workspaceID: company.WorkspaceID,
		now:         now,
	})
	if err != nil {
		return err
	}
	pos, err := ensureSeedOrgNode(ctx, client, lg, orgSeedSpec{
		key:         biz.CompanyLeadPositionKey(company.OrgKey),
		name:        biz.CompanyLeadPositionName,
		level:       "position",
		parentID:    office.ID,
		workspaceID: company.WorkspaceID,
		now:         now,
	})
	if err != nil {
		return err
	}

	agentKey := biz.CompanyLeadAgentKeyPrefix + company.OrgKey + "__"
	agentID := "agent___company_lead_" + company.OrgKey + "__"
	displayName := "总经理-" + company.Name
	description := "公司总经理，负责「" + company.Name + "」的流程剧本授权、对外 Brief 与跨部门仲裁。"
	const q = `INSERT INTO agents (
			id, agent_key, display_name, provider, model, status,
			is_default, is_favorite, icon, agent_description,
			position_id, system_prompt_mode, context_window,
			budget_monthly_cents, config_json, roles_json, created_by,
			created_at, updated_at, deleted_at, readonly, kind, source,
			position_key, agent_variant
		) VALUES (
			?, ?, ?, 'openrouter', 'gpt-4.1-mini',
			'active', FALSE, FALSE, '', ?,
			?, 'complete', 0, 0, '{"tools":{"profile":"read_only"},"memory_enabled":true}', '[]', 'system',
			?, ?, '', TRUE, 'system_builtin', 'system',
			?, 'company_lead'
		) ON CONFLICT(agent_key) DO UPDATE SET
			status = excluded.status,
			agent_description = excluded.agent_description,
			position_id = excluded.position_id,
			position_key = excluded.position_key,
			system_prompt_mode = excluded.system_prompt_mode,
			config_json = excluded.config_json,
			deleted_at = excluded.deleted_at,
			readonly = excluded.readonly,
			kind = excluded.kind,
			source = excluded.source,
			agent_variant = excluded.agent_variant,
			updated_at = excluded.updated_at`
	if _, err := client.ExecContext(ctx, d.RenumberPlaceholders(q),
		agentID, agentKey, displayName, description, pos.ID, now, now, pos.OrgKey); err != nil {
		return entErrToBizErr(err, "SEED")
	}
	resolvedID := agentID
	if existing, err := client.Agent.Query().
		Where(agent.AgentKeyEQ(agentKey), agent.DeletedAtEQ("")).
		Only(ctx); err == nil && strings.TrimSpace(existing.ID) != "" {
		resolvedID = existing.ID
	}

	n := entToBizOrganization(company)
	n.CompanyLeadAgentID = resolvedID
	biz.ApplyCompanyLeadToMetadata(&n)
	if err := client.Organization.UpdateOneID(company.ID).
		SetMetadataJSON(n.MetadataJSON).
		SetUpdatedAt(now).
		Exec(ctx); err != nil {
		lg.Warn("seed company lead: link metadata failed",
			loggateway.StepID("data.seed.company_lead_agents"),
			loggateway.Str("company_id", company.ID),
			loggateway.Err(err))
	}
	return nil
}

type orgSeedSpec struct {
	key, name, level, parentID, workspaceID, now string
}

func ensureSeedOrgNode(ctx context.Context, client *ent.Client, lg loggateway.Logger, spec orgSeedSpec) (*ent.Organization, error) {
	row, err := client.Organization.Query().
		Where(organization.OrgKeyEQ(spec.key)).
		Only(ctx)
	if err == nil {
		if row.DeletedAt != "" {
			if err := client.Organization.UpdateOneID(row.ID).
				SetDeletedAt("").
				SetStatus("active").
				SetEnabled(true).
				SetIsSystem(true).
				SetParentID(spec.parentID).
				SetLevel(spec.level).
				SetUpdatedAt(spec.now).
				Exec(ctx); err != nil {
				return nil, entErrToBizErr(err, "SEED")
			}
			row.DeletedAt = ""
			row.Status = "active"
		}
		return row, nil
	}
	if !ent.IsNotFound(err) {
		return nil, entErrToBizErr(err, "SEED")
	}
	saved, err := client.Organization.Create().
		SetID("org_" + uuid.NewString()).
		SetOrgKey(spec.key).
		SetName(spec.name).
		SetDescription("").
		SetStatus("active").
		SetEnabled(true).
		SetSortOrder(0).
		SetParentID(spec.parentID).
		SetLevel(spec.level).
		SetScenarioKey("").
		SetWorkspaceID(spec.workspaceID).
		SetOwnerUserID("").
		SetIsSystem(true).
		SetConfigJSON("").
		SetMetadataJSON("").
		SetDeptLeadAgentID("").
		SetDeptLeadConfigJSON("{}").
		SetCreatedAt(spec.now).
		SetUpdatedAt(spec.now).
		SetDeletedAt("").
		Save(ctx)
	if err != nil {
		lg.Error("seed org node create failed",
			loggateway.StepID("data.seed.company_lead_agents"),
			loggateway.Str("org_key", spec.key),
			loggateway.Err(err))
		return nil, entErrToBizErr(err, "SEED")
	}
	return saved, nil
}

// SeedCompanyLeadPromptFiles seeds company_lead.md for each company lead agent.
func SeedCompanyLeadPromptFiles(ctx context.Context, client *ent.Client, d Dialect, scenarioDir string, lg loggateway.Logger) error {
	if client == nil {
		return nil
	}
	promptPath := filepath.Join(scenarioDir, "system", "prompts", "company_lead.md")
	data, err := os.ReadFile(promptPath)
	if err != nil {
		body, embErr := systemprompts.ReadMarkdown("company_lead.md")
		if embErr != nil {
			lg.Warn("company_lead.md not found (disk+embed), skipping",
				loggateway.StepID("data.seed.company_lead_prompt_files"),
				loggateway.Str("path", promptPath),
				loggateway.Err(err))
			return nil
		}
		lg.Warn("company_lead.md missing on disk, using embedded prompt",
			loggateway.StepID("data.seed.company_lead_prompt_files"),
			loggateway.Str("path", promptPath),
			loggateway.Err(err))
		data = []byte(body)
	}

	rows, err := client.QueryContext(ctx, `SELECT id, agent_key FROM agents WHERE agent_variant = 'company_lead' AND deleted_at = ''`)
	if err != nil {
		lg.Error("seed step failed: query company lead agents",
			loggateway.StepID("data.seed.company_lead_prompt_files"),
			loggateway.Err(err))
		return entErrToBizErr(err, "SEED")
	}
	defer rows.Close()

	// 2026-08-24 P4 修复：模板含 {{.CompanyName}}/{{.CompanyDescription}} 占位符，
	// 运行时无渲染（RenderPromptTemplate 未接入生产路径，TECH-DEBT B-5），
	// 种子必须按公司渲染后再落库，否则总经理 prompt 出现原始占位符。
	companies, err := client.Organization.Query().
		Where(organization.DeletedAtEQ(""), organization.LevelEQ("company")).
		All(ctx)
	if err != nil {
		return entErrToBizErr(err, "SEED")
	}
	type companyInfo struct{ name, description string }
	byKey := make(map[string]companyInfo, len(companies))
	for _, c := range companies {
		byKey[c.OrgKey] = companyInfo{name: c.Name, description: c.Description}
	}
	tmpl, err := template.New("company_lead_seed").Parse(string(data))
	if err != nil {
		return entErrToBizErr(err, "SEED")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	const q = `INSERT INTO agent_prompt_files (
		id, agent_id, file_name, body, sort_order, created_at, updated_at
	) VALUES (
		?, ?, ?, ?, ?, ?, ?
	) ON CONFLICT(id) DO UPDATE SET
		body = excluded.body,
		sort_order = excluded.sort_order,
		updated_at = excluded.updated_at`

	for rows.Next() {
		var agentID, agentKey string
		if err := rows.Scan(&agentID, &agentKey); err != nil {
			lg.Error("seed step failed: scan company lead agent row",
				loggateway.StepID("data.seed.company_lead_prompt_files"),
				loggateway.Err(err))
			continue
		}
		body := string(data)
		orgKey := strings.TrimSuffix(strings.TrimPrefix(agentKey, "__company_lead_"), "__")
		if co, ok := byKey[orgKey]; ok {
			var b strings.Builder
			if renderErr := tmpl.Execute(&b, map[string]string{
				"CompanyName":        co.name,
				"CompanyDescription": co.description,
			}); renderErr != nil {
				lg.Warn("company lead prompt render failed, keeping raw template",
					loggateway.StepID("data.seed.company_lead_prompt_files"),
					loggateway.Str("agent_key", agentKey),
					loggateway.Err(renderErr))
			} else {
				body = b.String()
			}
		} else {
			lg.Warn("company lead agent has no matching company node, keeping raw template",
				loggateway.StepID("data.seed.company_lead_prompt_files"),
				loggateway.Str("agent_key", agentKey))
		}
		id := "apf_company_lead_" + strings.ReplaceAll(agentKey, "/", "_")
		if _, err := client.ExecContext(ctx, d.RenumberPlaceholders(q), id, agentID, "system.md", body, 1, now, now); err != nil {
			lg.Error("seed step failed: seed company lead prompt file",
				loggateway.StepID("data.seed.company_lead_prompt_files"),
				loggateway.Str("agent_key", agentKey),
				loggateway.Err(err))
		}
	}
	return nil
}
