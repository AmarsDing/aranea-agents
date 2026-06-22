package data

import (
	"context"
	"encoding/json"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

type runtimeProfileRepo struct{ data *Data }

var _ biz.RuntimeProfileReadWriter = (*runtimeProfileRepo)(nil)

func NewRuntimeProfileRepo(data *Data) biz.RuntimeProfileReadWriter {
	return &runtimeProfileRepo{data: data}
}

const rpDomain = apierror.DomainRuntimeProfile

const runtimeProfileColumns = `id, agent_id, name, description, version, is_active, priority,
       prompt_config, tool_policy, skill_policy, knowledge_policy,
       workspace_policy, credential_policy, isolation_policy, extra_model_config,
       created_at, updated_at`

func (r *runtimeProfileRepo) List(ctx context.Context, agentID string, activeOnly bool) ([]biz.RuntimeProfile, error) {
	q := `SELECT ` + runtimeProfileColumns + ` FROM runtime_profiles`
	args := []any{}
	if agentID != "" {
		q += ` WHERE agent_id = ?`
		args = append(args, agentID)
		if activeOnly {
			q += ` AND is_active = 1`
		}
	} else if activeOnly {
		q += ` WHERE is_active = 1`
	}
	q += ` ORDER BY priority DESC, updated_at DESC`
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return nil, entErrToBizErr(err, rpDomain)
	}
	defer rows.Close()
	var result []biz.RuntimeProfile
	for rows.Next() {
		p, sErr := scanRuntimeProfile(rows)
		if sErr != nil {
			return nil, entErrToBizErr(sErr, rpDomain)
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func (r *runtimeProfileRepo) GetByID(ctx context.Context, id string) (biz.RuntimeProfile, error) {
	q := r.data.Dialect().RenumberPlaceholders(`SELECT ` + runtimeProfileColumns + ` FROM runtime_profiles WHERE id = ?`)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, id)
	if err != nil {
		return biz.RuntimeProfile{}, entErrToBizErr(err, rpDomain)
	}
	defer rows.Close()
	if !rows.Next() {
		return biz.RuntimeProfile{}, entErrToBizErr(apierror.NotFound(rpDomain, "runtime profile not found"), rpDomain)
	}
	p, err := scanRuntimeProfile(rows)
	if err != nil {
		return biz.RuntimeProfile{}, entErrToBizErr(err, rpDomain)
	}
	return p, rows.Err()
}

func (r *runtimeProfileRepo) GetActive(ctx context.Context, agentID string) (*biz.RuntimeProfile, error) {
	q := r.data.Dialect().RenumberPlaceholders(`SELECT ` + runtimeProfileColumns + ` FROM runtime_profiles WHERE agent_id = ? AND is_active = 1 ORDER BY priority DESC LIMIT 1`)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, agentID)
	if err != nil {
		return nil, entErrToBizErr(err, rpDomain)
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, nil // no active profile — graceful degradation
	}
	p, err := scanRuntimeProfile(rows)
	if err != nil {
		return nil, entErrToBizErr(err, rpDomain)
	}
	return &p, rows.Err()
}

func (r *runtimeProfileRepo) Create(ctx context.Context, p biz.RuntimeProfile) (biz.RuntimeProfile, error) {
	now := time.Now().UTC()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	policyJSON, err := p.PolicyJSON()
	if err != nil {
		return biz.RuntimeProfile{}, entErrToBizErr(err, rpDomain)
	}
	q := r.data.Dialect().RenumberPlaceholders(`INSERT INTO runtime_profiles
       (id, agent_id, name, description, version, is_active, priority,
        prompt_config, tool_policy, skill_policy, knowledge_policy,
        workspace_policy, credential_policy, isolation_policy, extra_model_config,
        created_at, updated_at)
       VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	activeVal := 0
	if p.IsActive {
		activeVal = 1
	}
	_, err = r.data.RWDB().WriteDB(ctx).ExecContext(ctx, q,
		p.ID, p.AgentID, p.Name, p.Description, p.Version, activeVal, p.Priority,
		policyJSON["prompt_config"], policyJSON["tool_policy"], policyJSON["skill_policy"],
		policyJSON["knowledge_policy"], policyJSON["workspace_policy"], policyJSON["credential_policy"],
		policyJSON["isolation_policy"], policyJSON["extra_model_config"],
		p.CreatedAt.Format(time.RFC3339), p.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return biz.RuntimeProfile{}, entErrToBizErr(err, rpDomain)
	}
	return p, nil
}

func (r *runtimeProfileRepo) Update(ctx context.Context, p biz.RuntimeProfile) (biz.RuntimeProfile, error) {
	p.UpdatedAt = time.Now().UTC()
	policyJSON, err := p.PolicyJSON()
	if err != nil {
		return biz.RuntimeProfile{}, entErrToBizErr(err, rpDomain)
	}
	activeVal := 0
	if p.IsActive {
		activeVal = 1
	}
	q := r.data.Dialect().RenumberPlaceholders(`UPDATE runtime_profiles SET
       agent_id = ?, name = ?, description = ?, version = ?, is_active = ?, priority = ?,
       prompt_config = ?, tool_policy = ?, skill_policy = ?, knowledge_policy = ?,
       workspace_policy = ?, credential_policy = ?, isolation_policy = ?, extra_model_config = ?,
       updated_at = ? WHERE id = ?`)
	_, err = r.data.RWDB().WriteDB(ctx).ExecContext(ctx, q,
		p.AgentID, p.Name, p.Description, p.Version, activeVal, p.Priority,
		policyJSON["prompt_config"], policyJSON["tool_policy"], policyJSON["skill_policy"],
		policyJSON["knowledge_policy"], policyJSON["workspace_policy"], policyJSON["credential_policy"],
		policyJSON["isolation_policy"], policyJSON["extra_model_config"],
		p.UpdatedAt.Format(time.RFC3339), p.ID,
	)
	if err != nil {
		return biz.RuntimeProfile{}, entErrToBizErr(err, rpDomain)
	}
	return p, nil
}

func (r *runtimeProfileRepo) Delete(ctx context.Context, id string) error {
	q := r.data.Dialect().RenumberPlaceholders(`DELETE FROM runtime_profiles WHERE id = ?`)
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, q, id)
	return entErrToBizErr(err, rpDomain)
}

func (r *runtimeProfileRepo) SetActive(ctx context.Context, id string, active bool) (biz.RuntimeProfile, error) {
	activeVal := 0
	if active {
		activeVal = 1
	}
	now := time.Now().UTC().Format(time.RFC3339)
	q := r.data.Dialect().RenumberPlaceholders(`UPDATE runtime_profiles SET is_active = ?, updated_at = ? WHERE id = ?`)
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, q, activeVal, now, id)
	if err != nil {
		return biz.RuntimeProfile{}, entErrToBizErr(err, rpDomain)
	}
	return r.GetByID(ctx, id)
}

// rpScanner abstracts *sql.Rows for shared scan logic.
type rpScanner interface {
	Scan(dest ...any) error
}

func scanRuntimeProfile(s rpScanner) (biz.RuntimeProfile, error) {
	var p biz.RuntimeProfile
	var isActive int
	var promptCfg, toolPol, skillPol, knowPol, wsPol, credPol, isoPol, extraModel string
	var createdAt, updatedAt string
	if err := s.Scan(
		&p.ID, &p.AgentID, &p.Name, &p.Description, &p.Version, &isActive, &p.Priority,
		&promptCfg, &toolPol, &skillPol, &knowPol, &wsPol, &credPol, &isoPol, &extraModel,
		&createdAt, &updatedAt,
	); err != nil {
		return biz.RuntimeProfile{}, err
	}
	p.IsActive = isActive == 1
	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		p.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, updatedAt); err == nil {
		p.UpdatedAt = t
	}
	_ = json.Unmarshal([]byte(promptCfg), &p.PromptConfig)
	_ = json.Unmarshal([]byte(toolPol), &p.ToolPolicy)
	_ = json.Unmarshal([]byte(skillPol), &p.SkillPolicy)
	_ = json.Unmarshal([]byte(knowPol), &p.KnowledgePolicy)
	_ = json.Unmarshal([]byte(wsPol), &p.WorkspacePolicy)
	_ = json.Unmarshal([]byte(credPol), &p.CredentialPolicy)
	_ = json.Unmarshal([]byte(isoPol), &p.IsolationPolicy)
	if extraModel != "" && extraModel != "{}" {
		_ = json.Unmarshal([]byte(extraModel), &p.ExtraModelConfig)
	}
	return p, nil
}
