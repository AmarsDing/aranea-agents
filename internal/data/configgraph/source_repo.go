// Package configgraph — source_repo.go implements biz/configgraph.SourceRepo
// with raw SQL over the 12 config-asset source tables (plus the two agent-side
// policy tables). Rows are plain DTOs; extractors stay pure functions.
//
// Conventions (per source.go contract):
//   - soft-delete columns are TEXT with ” = active across all source tables;
//     rows are returned including tombstones so the graph marks status=deleted
//     (extractors skip out-edges for deleted rows);
//   - tool_agent_overrides is the exception: deleted rows are filtered here
//     (a soft-deleted override is no reference at all);
//   - graph_definitions / agent_prompt_files / knowledge_collections have no
//     deleted_at column — all rows are live.
package configgraph

import (
	"context"
	"database/sql"

	"aranea-agents/internal/data"
	"aranea-agents/pkg/loggateway"

	bizcg "aranea-agents/internal/biz/configgraph"
)

type sourceRepo struct {
	rw *data.ReadWriteDB
	d  data.Dialect
	lg loggateway.Logger
}

var _ bizcg.SourceRepo = (*sourceRepo)(nil)

// NewSourceRepo constructs the source repo from the shared Data handle.
func NewSourceRepo(d *data.Data, lg loggateway.Logger) bizcg.SourceRepo {
	if d == nil || d.RWDB() == nil {
		return nil
	}
	return NewSourceRepoFromRWDB(d.RWDB(), d.Dialect(), lg)
}

// NewSourceRepoFromRWDB constructs the source repo from explicit handles.
func NewSourceRepoFromRWDB(rw *data.ReadWriteDB, dialect data.Dialect, lg loggateway.Logger) bizcg.SourceRepo {
	if rw == nil {
		return nil
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &sourceRepo{rw: rw, d: dialect, lg: lg.With(loggateway.Domain("config_graph"))}
}

// query runs q (no bind params — all lists are full scans) and invokes scan
// per row. Errors translate through toBizErr (repo.go, same package).
func (s *sourceRepo) query(ctx context.Context, q string, scan func(*sql.Rows) error) error {
	rows, err := s.rw.ReadDB(ctx).QueryContext(ctx, s.d.RenumberPlaceholders(q))
	if err != nil {
		return toBizErr(err)
	}
	defer rows.Close()
	for rows.Next() {
		if err := scan(rows); err != nil {
			return toBizErr(err)
		}
	}
	return toBizErr(rows.Err())
}

func (s *sourceRepo) ListAgents(ctx context.Context) ([]bizcg.AgentRow, error) {
	if s == nil || s.rw == nil {
		return nil, nil
	}
	out := make([]bizcg.AgentRow, 0, 256)
	err := s.query(ctx,
		`SELECT id, agent_key, display_name, status, kind, agent_variant, position_id, position_key, workspace_id, deleted_at
		   FROM agents ORDER BY id`,
		func(rows *sql.Rows) error {
			var r bizcg.AgentRow
			if err := rows.Scan(&r.ID, &r.AgentKey, &r.DisplayName, &r.Status, &r.Kind, &r.AgentVariant,
				&r.PositionID, &r.PositionKey, &r.WorkspaceID, &r.DeletedAt); err != nil {
				return err
			}
			out = append(out, r)
			return nil
		})
	return out, err
}

func (s *sourceRepo) ListTeams(ctx context.Context) ([]bizcg.TeamRow, error) {
	if s == nil || s.rw == nil {
		return nil, nil
	}
	out := make([]bizcg.TeamRow, 0, 128)
	err := s.query(ctx,
		`SELECT id, team_key, display_name, status, is_default, kind, topology, definition_json,
		        department_id, dept_lead_agent_id, cross_dept_member_ids, linked_graph_id, workspace_id, deleted_at
		   FROM teams ORDER BY id`,
		func(rows *sql.Rows) error {
			var r bizcg.TeamRow
			if err := rows.Scan(&r.ID, &r.TeamKey, &r.DisplayName, &r.Status, &r.IsDefault, &r.Kind, &r.Topology,
				&r.DefinitionJSON, &r.DepartmentID, &r.DeptLeadAgentID, &r.CrossDeptMemberIDs, &r.LinkedGraphID,
				&r.WorkspaceID, &r.DeletedAt); err != nil {
				return err
			}
			out = append(out, r)
			return nil
		})
	return out, err
}

func (s *sourceRepo) ListSkills(ctx context.Context) ([]bizcg.SkillRow, error) {
	if s == nil || s.rw == nil {
		return nil, nil
	}
	out := make([]bizcg.SkillRow, 0, 256)
	err := s.query(ctx,
		`SELECT id, skill_key, name, status, enabled, parent_id, agent_id, workspace_id, deleted_at
		   FROM skill ORDER BY id`,
		func(rows *sql.Rows) error {
			var r bizcg.SkillRow
			if err := rows.Scan(&r.ID, &r.SkillKey, &r.Name, &r.Status, &r.Enabled,
				&r.ParentID, &r.AgentID, &r.WorkspaceID, &r.DeletedAt); err != nil {
				return err
			}
			out = append(out, r)
			return nil
		})
	return out, err
}

func (s *sourceRepo) ListTools(ctx context.Context) ([]bizcg.ToolRow, error) {
	if s == nil || s.rw == nil {
		return nil, nil
	}
	out := make([]bizcg.ToolRow, 0, 256)
	err := s.query(ctx,
		`SELECT id, tool_key, display_name, category, risk_level, enabled, requires_confirmation, workspace_id, deleted_at
		   FROM tools ORDER BY id`,
		func(rows *sql.Rows) error {
			var r bizcg.ToolRow
			if err := rows.Scan(&r.ID, &r.ToolKey, &r.DisplayName, &r.Category, &r.RiskLevel, &r.Enabled,
				&r.RequiresConfirmation, &r.WorkspaceID, &r.DeletedAt); err != nil {
				return err
			}
			out = append(out, r)
			return nil
		})
	return out, err
}

func (s *sourceRepo) ListPromptFiles(ctx context.Context) ([]bizcg.PromptFileRow, error) {
	if s == nil || s.rw == nil {
		return nil, nil
	}
	out := make([]bizcg.PromptFileRow, 0, 256)
	err := s.query(ctx,
		`SELECT p.id, p.agent_id, COALESCE(a.agent_key, ''), p.file_name, p.body, p.sort_order
		   FROM agent_prompt_files p
		   LEFT JOIN agents a ON a.id = p.agent_id
		   ORDER BY p.agent_id, p.sort_order`,
		func(rows *sql.Rows) error {
			var r bizcg.PromptFileRow
			if err := rows.Scan(&r.ID, &r.AgentID, &r.AgentKey, &r.FileName, &r.Body, &r.SortOrder); err != nil {
				return err
			}
			out = append(out, r)
			return nil
		})
	return out, err
}

func (s *sourceRepo) ListCronTasks(ctx context.Context) ([]bizcg.CronTaskRow, error) {
	if s == nil || s.rw == nil {
		return nil, nil
	}
	out := make([]bizcg.CronTaskRow, 0, 64)
	err := s.query(ctx,
		`SELECT id, task_key, name, status, enabled, agent_id, config_json, workspace_id, deleted_at
		   FROM cron_task ORDER BY id`,
		func(rows *sql.Rows) error {
			var r bizcg.CronTaskRow
			if err := rows.Scan(&r.ID, &r.TaskKey, &r.Name, &r.Status, &r.Enabled,
				&r.AgentID, &r.ConfigJSON, &r.WorkspaceID, &r.DeletedAt); err != nil {
				return err
			}
			out = append(out, r)
			return nil
		})
	return out, err
}

func (s *sourceRepo) ListChannels(ctx context.Context) ([]bizcg.ChannelRow, error) {
	if s == nil || s.rw == nil {
		return nil, nil
	}
	out := make([]bizcg.ChannelRow, 0, 64)
	err := s.query(ctx,
		`SELECT id, channel_key, name, status, enabled, config_json, workspace_id, deleted_at
		   FROM channel ORDER BY id`,
		func(rows *sql.Rows) error {
			var r bizcg.ChannelRow
			if err := rows.Scan(&r.ID, &r.ChannelKey, &r.Name, &r.Status, &r.Enabled,
				&r.ConfigJSON, &r.WorkspaceID, &r.DeletedAt); err != nil {
				return err
			}
			out = append(out, r)
			return nil
		})
	return out, err
}

func (s *sourceRepo) ListOrganizations(ctx context.Context) ([]bizcg.OrganizationRow, error) {
	if s == nil || s.rw == nil {
		return nil, nil
	}
	out := make([]bizcg.OrganizationRow, 0, 128)
	err := s.query(ctx,
		`SELECT id, org_key, name, status, parent_id, level, dept_lead_agent_id, workspace_id, deleted_at
		   FROM organizations ORDER BY id`,
		func(rows *sql.Rows) error {
			var r bizcg.OrganizationRow
			if err := rows.Scan(&r.ID, &r.OrgKey, &r.Name, &r.Status, &r.ParentID, &r.Level,
				&r.DeptLeadAgentID, &r.WorkspaceID, &r.DeletedAt); err != nil {
				return err
			}
			out = append(out, r)
			return nil
		})
	return out, err
}

func (s *sourceRepo) ListGraphs(ctx context.Context) ([]bizcg.GraphRow, error) {
	if s == nil || s.rw == nil {
		return nil, nil
	}
	out := make([]bizcg.GraphRow, 0, 64)
	// graph_definitions has no deleted_at; node definitions live in `nodes`.
	err := s.query(ctx,
		`SELECT id, name, nodes, team_id, is_template, workspace_id
		   FROM graph_definitions ORDER BY id`,
		func(rows *sql.Rows) error {
			var r bizcg.GraphRow
			if err := rows.Scan(&r.ID, &r.Name, &r.NodesJSON, &r.TeamID, &r.IsTemplate, &r.WorkspaceID); err != nil {
				return err
			}
			out = append(out, r)
			return nil
		})
	return out, err
}

func (s *sourceRepo) ListKnowledgeCollections(ctx context.Context) ([]bizcg.KnowledgeCollectionRow, error) {
	if s == nil || s.rw == nil {
		return nil, nil
	}
	out := make([]bizcg.KnowledgeCollectionRow, 0, 64)
	// knowledge_collections has no deleted_at; workspace column is `workspace`.
	err := s.query(ctx,
		`SELECT id, name, status, workspace
		   FROM knowledge_collections ORDER BY id`,
		func(rows *sql.Rows) error {
			var r bizcg.KnowledgeCollectionRow
			if err := rows.Scan(&r.ID, &r.Name, &r.Status, &r.Workspace); err != nil {
				return err
			}
			out = append(out, r)
			return nil
		})
	return out, err
}

func (s *sourceRepo) ListMCPServers(ctx context.Context) ([]bizcg.MCPServerRow, error) {
	if s == nil || s.rw == nil {
		return nil, nil
	}
	out := make([]bizcg.MCPServerRow, 0, 64)
	err := s.query(ctx,
		`SELECT id, server_key, name, status, enabled, workspace_id, deleted_at
		   FROM mcp_server ORDER BY id`,
		func(rows *sql.Rows) error {
			var r bizcg.MCPServerRow
			if err := rows.Scan(&r.ID, &r.ServerKey, &r.Name, &r.Status, &r.Enabled,
				&r.WorkspaceID, &r.DeletedAt); err != nil {
				return err
			}
			out = append(out, r)
			return nil
		})
	return out, err
}

func (s *sourceRepo) ListHooks(ctx context.Context) ([]bizcg.HookRow, error) {
	if s == nil || s.rw == nil {
		return nil, nil
	}
	out := make([]bizcg.HookRow, 0, 64)
	err := s.query(ctx,
		`SELECT id, hook_key, name, status, enabled, config_json, deleted_at
		   FROM hooks ORDER BY id`,
		func(rows *sql.Rows) error {
			var r bizcg.HookRow
			if err := rows.Scan(&r.ID, &r.HookKey, &r.Name, &r.Status, &r.Enabled,
				&r.ConfigJSON, &r.DeletedAt); err != nil {
				return err
			}
			out = append(out, r)
			return nil
		})
	return out, err
}

func (s *sourceRepo) ListToolOverrides(ctx context.Context) ([]bizcg.ToolOverrideRow, error) {
	if s == nil || s.rw == nil {
		return nil, nil
	}
	out := make([]bizcg.ToolOverrideRow, 0, 128)
	// Soft-deleted overrides are no reference at all — filtered here per contract.
	err := s.query(ctx,
		`SELECT id, tool_id, tool_key, agent_id, mode, enabled, deleted_at
		   FROM tool_agent_overrides WHERE deleted_at = '' ORDER BY id`,
		func(rows *sql.Rows) error {
			var r bizcg.ToolOverrideRow
			if err := rows.Scan(&r.ID, &r.ToolID, &r.ToolKey, &r.AgentID, &r.Mode,
				&r.Enabled, &r.DeletedAt); err != nil {
				return err
			}
			out = append(out, r)
			return nil
		})
	return out, err
}

func (s *sourceRepo) ListAgentToolPolicies(ctx context.Context) ([]bizcg.AgentToolPolicyRow, error) {
	if s == nil || s.rw == nil {
		return nil, nil
	}
	out := make([]bizcg.AgentToolPolicyRow, 0, 256)
	// agent_runtime_settings PK is stored under column agent_id (StorageKey).
	err := s.query(ctx,
		`SELECT agent_id, tools_allow_json, tools_deny_json, skill_runtime_json
		   FROM agent_runtime_settings ORDER BY agent_id`,
		func(rows *sql.Rows) error {
			var r bizcg.AgentToolPolicyRow
			if err := rows.Scan(&r.AgentID, &r.ToolsAllowJSON, &r.ToolsDenyJSON, &r.SkillRuntimeJSON); err != nil {
				return err
			}
			out = append(out, r)
			return nil
		})
	return out, err
}
