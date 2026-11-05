package data

import (
	"context"
	"database/sql"
	"fmt"

	dataent "aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

// ddl_migration_hooks_coding.go —— 03 方案 P0：编码工具注册表默认记录种子。
//
// 种两条注册记录（Codex / Claude Code，真实图标走前端静态目录
// web/public/coding-tools/*.svg，来源 simple-icons CC0），默认 enabled=false：
// 二进制是否安装由管理员在 /settings/coding-tools 探测后自行启用，
// 避免未装机环境产生必败任务。幂等：agent_key 已存在则跳过。

func ddlCodingAgentsSeed(ctx context.Context, rawDB *sql.DB, entClient *dataent.Client, d Dialect, lg loggateway.Logger) error {
	return ensureCodingAgentsSeed(ctx, rawDB, d)
}

// codingAgentSeedRow 注册表种子行（command/args 镜像 biz DefaultACPLaunch 预设，
// 见 internal/biz/agentbridge/launch.go）。
type codingAgentSeedRow struct {
	agentKey    string
	displayName string
	command     string
	argsJSON    string
	iconURL     string
	autonomy    string
}

var codingAgentSeeds = []codingAgentSeedRow{
	{
		agentKey:    "codex",
		displayName: "Codex",
		command:     "npx",
		argsJSON:    `["-y","@agentclientprotocol/codex-acp"]`,
		iconURL:     "/coding-tools/openai.svg",
		autonomy:    "workspace",
	},
	{
		agentKey:    "claude_code",
		displayName: "Claude Code",
		command:     "claude-agent-acp",
		argsJSON:    `[]`,
		iconURL:     "/coding-tools/claude.svg",
		autonomy:    "workspace",
	},
}

const codingAgentCapabilitiesJSON = `["read_workspace","write_workspace","exec_command"]`

func ensureCodingAgentsSeed(ctx context.Context, rawDB *sql.DB, d Dialect) error {
	if rawDB == nil {
		return nil
	}
	now := nowRFC3339()
	for _, seed := range codingAgentSeeds {
		q := `INSERT INTO coding_agents
			(id, workspace, agent_key, display_name, command, args, env, enabled,
			 last_probe_ok, last_probe_error, created_at, updated_at,
			 icon_url, capabilities, default_autonomy)
			SELECT ?, 'default', ?, ?, ?, ?, '{}', false,
			       false, '', ?, ?, ?, ?, ?
			WHERE NOT EXISTS (
				SELECT 1 FROM coding_agents WHERE workspace='default' AND agent_key=?
			)`
		_, err := rawDB.ExecContext(ctx, d.RenumberPlaceholders(q),
			"codingagent_"+uuid.NewString(), seed.agentKey, seed.displayName, seed.command,
			seed.argsJSON, now, now, seed.iconURL, codingAgentCapabilitiesJSON, seed.autonomy,
			seed.agentKey)
		if err != nil {
			return fmt.Errorf("seed coding agent %s: %w", seed.agentKey, err)
		}
	}
	return nil
}
