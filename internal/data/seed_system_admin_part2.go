package data

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	systemprompts "aranea-agents/internal/scenario/system"
	"aranea-agents/pkg/loggateway"
	"context"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"
)

// SeedDeptLeadAgents creates department lead agents for all existing department-level
// org nodes. This is called during seed to ensure every department has a lead agent.
func SeedDeptLeadAgents(ctx context.Context, client *ent.Client, d Dialect, lg loggateway.Logger) error {
	if client == nil {
		return nil
	}

	rows, err := client.QueryContext(ctx, `SELECT id, org_key, name, description FROM organizations c
		WHERE c.level = 'department' AND c.deleted_at = ''
		AND NOT EXISTS (
			SELECT 1 FROM organizations p
			WHERE p.id = c.parent_id AND p.level = 'company' AND p.deleted_at = ''
			AND c.org_key = p.org_key || '_office'
		)`)
	if err != nil {
		lg.Warn("seed step failed: query departments", loggateway.StepID("data.seed.dept_lead_agents"), loggateway.Err(err))
		return entErrToBizErr(err, "SEED")
	}
	defer rows.Close()

	now := time.Now().UTC().Format(time.RFC3339)

	for rows.Next() {
		var id, key, name, desc string
		if err := rows.Scan(&id, &key, &name, &desc); err != nil {
			lg.Warn("seed step failed: scan department row", loggateway.StepID("data.seed.dept_lead_agents"), loggateway.Err(err))
			continue
		}
		agentKey := biz.DeptLeadAgentKeyPrefix + key + "__"
		agentID := "agent___dept_lead_" + key + "__"
		displayName := "部门主管-" + name
		description := "部门主管，负责「" + name + "」的资源协调和跨部门交付审批。"
		if desc != "" {
			description = "部门主管，负责「" + name + "」的资源协调和跨部门交付审批。" + desc
		}

		// position_id/position_key 绑定部门节点本身（对齐正常路径 biz.CreateDeptLead
		// PositionID: deptNode.ID）。2026-08-25 包A-A2'a 实锤：旧种子 position_id 硬
		// 编码 '' → resolveAgentDepartment 返空 → memberfs/deptmail 7 个专属工具必返
		// "caller is not attached to a department"，主管会话 10 轮空转烧 80K in。
		// DO UPDATE 同步 position_id 使存量空挂行随启动种子自愈。
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
	?, 'dept_lead'
) ON CONFLICT(agent_key) DO UPDATE SET
	status = excluded.status,
	agent_description = excluded.agent_description,
	position_id = excluded.position_id,
	system_prompt_mode = excluded.system_prompt_mode,
	config_json = excluded.config_json,
	deleted_at = excluded.deleted_at,
	readonly = excluded.readonly,
	kind = excluded.kind,
	source = excluded.source,
	position_key = excluded.position_key,
	agent_variant = excluded.agent_variant,
	updated_at = excluded.updated_at`
		if _, err := client.ExecContext(ctx, d.RenumberPlaceholders(q), agentID, agentKey, displayName, description, id, now, now, key); err != nil {
			lg.Warn("seed step failed: create dept lead agent",
				loggateway.StepID("data.seed.dept_lead_agents"),
				loggateway.Str("dept_key", key),
				loggateway.Err(err))
			continue
		}

		if _, err := client.ExecContext(ctx, d.RenumberPlaceholders(`UPDATE organizations SET dept_lead_agent_id = ?, updated_at = ? WHERE id = ? AND (dept_lead_agent_id = '' OR dept_lead_agent_id IS NULL)`), agentID, now, id); err != nil {
			lg.Warn("seed step failed: link dept lead to org node",
				loggateway.StepID("data.seed.dept_lead_agents"),
				loggateway.Str("dept_id", id),
				loggateway.Err(err))
		}
	}
	return nil
}

// SeedDeptLeadPromptFiles seeds the dept_lead.md prompt file for each department lead agent.
func SeedDeptLeadPromptFiles(ctx context.Context, client *ent.Client, d Dialect, scenarioDir string, lg loggateway.Logger) error {
	if client == nil {
		return nil
	}
	promptPath := filepath.Join(scenarioDir, "system", "prompts", "dept_lead.md")
	data, err := os.ReadFile(promptPath)
	if err != nil {
		body, embErr := systemprompts.ReadMarkdown("dept_lead.md")
		if embErr != nil {
			lg.Warn("dept_lead.md not found (disk+embed), skipping",
				loggateway.StepID("data.seed.dept_lead_prompt_files"),
				loggateway.Str("path", promptPath),
				loggateway.Err(err))
			return nil
		}
		lg.Warn("dept_lead.md missing on disk, using embedded prompt",
			loggateway.StepID("data.seed.dept_lead_prompt_files"),
			loggateway.Str("path", promptPath),
			loggateway.Err(err))
		data = []byte(body)
	}

	rows, err := client.QueryContext(ctx, `SELECT id, agent_key FROM agents WHERE agent_variant = 'dept_lead' AND deleted_at = ''`)
	if err != nil {
		lg.Warn("seed step failed: query dept lead agents", loggateway.StepID("data.seed.dept_lead_prompt_files"), loggateway.Err(err))
		return entErrToBizErr(err, "SEED")
	}
	defer rows.Close()

	deptRows, err := client.QueryContext(ctx, `SELECT org_key, name, description FROM organizations WHERE level = 'department' AND deleted_at = ''`)
	if err != nil {
		return entErrToBizErr(err, "SEED")
	}
	type deptInfo struct{ name, description string }
	byKey := make(map[string]deptInfo)
	for deptRows.Next() {
		var k, n, d string
		if err := deptRows.Scan(&k, &n, &d); err != nil {
			continue
		}
		byKey[k] = deptInfo{name: n, description: d}
	}
	deptRows.Close()

	tmpl, err := template.New("dept_lead_seed").Option("missingkey=error").Parse(string(data))
	if err != nil {
		return entErrToBizErr(err, "SEED")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	const q = `INSERT INTO agent_prompt_files (
		id, agent_id, file_name, body, sort_order, created_at, updated_at
	) VALUES (
		?, ?, ?, ?, ?, ?, ?
	) ON CONFLICT(id) DO UPDATE SET
		file_name = excluded.file_name,
		body = excluded.body,
		sort_order = excluded.sort_order,
		updated_at = excluded.updated_at`

	for rows.Next() {
		var agentID, agentKey string
		if err := rows.Scan(&agentID, &agentKey); err != nil {
			lg.Warn("seed step failed: scan dept lead agent row", loggateway.StepID("data.seed.dept_lead_prompt_files"), loggateway.Err(err))
			continue
		}
		deptKey := strings.TrimSuffix(strings.TrimPrefix(agentKey, biz.DeptLeadAgentKeyPrefix), "__")
		dept, ok := byKey[deptKey]
		if !ok {

			lg.Warn("dept lead agent has no matching department node, skipping prompt seed",
				loggateway.StepID("data.seed.dept_lead_prompt_files"),
				loggateway.Str("agent_key", agentKey))
			continue
		}
		var b strings.Builder
		if renderErr := tmpl.Execute(&b, map[string]string{
			"DepartmentName":        dept.name,
			"DepartmentDescription": dept.description,
		}); renderErr != nil {

			lg.Warn("dept lead prompt render failed, skipping prompt seed",
				loggateway.StepID("data.seed.dept_lead_prompt_files"),
				loggateway.Str("agent_key", agentKey),
				loggateway.Err(renderErr))
			continue
		}
		body := b.String()
		id := "apf_dept_lead_" + agentKey
		if _, err := client.ExecContext(ctx, d.RenumberPlaceholders(q), id, agentID, "system.md", body, 1, now, now); err != nil {
			lg.Warn("seed step failed: seed dept lead prompt file",
				loggateway.StepID("data.seed.dept_lead_prompt_files"),
				loggateway.Str("agent_key", agentKey),
				loggateway.Err(err))
		}
	}
	return nil
}
