package sqlite

import (
	"database/sql"
	"errors"
	"strings"

	"arenea/backend/internal/domain"
	"arenea/backend/internal/kernel/contracts"
)

// EvolutionRepository 承载智能体身份、策略、进化事件/提案与技能统计的 SQLite 实现。

// EvolutionRepository 实现 Store 中与进化相关的持久化子集。
type EvolutionRepository struct {
	db *sql.DB
}

// NewEvolutionRepository 从 *sql.DB 构建进化仓储。
func NewEvolutionRepository(db *sql.DB) *EvolutionRepository {
	return &EvolutionRepository{db: db}
}

func (r *EvolutionRepository) GetAgentIdentity(agentID string) (domain.AgentIdentity, error) {
	if agentID == "" {
		return domain.AgentIdentity{}, errors.New("agent id is required")
	}
	row := r.db.QueryRow(
		`SELECT agent_id, persona, values_json, tone, domains_json, user_expectations, current_phase, metadata_json, version, created_at, updated_at
		 FROM agent_identity WHERE agent_id = ?`, agentID,
	)
	var (
		v          domain.AgentIdentity
		valuesRaw  string
		domainsRaw string
		metaRaw    string
	)
	if err := row.Scan(&v.AgentID, &v.Persona, &valuesRaw, &v.Tone, &domainsRaw, &v.UserExpectations, &v.CurrentPhase, &metaRaw, &v.Version, &v.CreatedAt, &v.UpdatedAt); err != nil {
		return domain.AgentIdentity{}, err
	}
	v.Values = decodeJSONStringSlice(valuesRaw)
	v.Domains = decodeJSONStringSlice(domainsRaw)
	v.Metadata = decodeJSONObject(metaRaw)
	return v, nil
}

func (r *EvolutionRepository) UpsertAgentIdentity(id domain.AgentIdentity) (domain.AgentIdentity, error) {
	if id.AgentID == "" {
		return domain.AgentIdentity{}, errors.New("agent id is required")
	}
	if id.CurrentPhase == "" {
		id.CurrentPhase = domain.AgentPhaseColdStart
	}
	if id.Version <= 0 {
		id.Version = 1
	}
	now := nowISO()
	if id.CreatedAt == "" {
		id.CreatedAt = now
	}
	id.UpdatedAt = now
	_, err := r.db.Exec(
		`INSERT INTO agent_identity(agent_id, persona, values_json, tone, domains_json, user_expectations, current_phase, metadata_json, version, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(agent_id) DO UPDATE SET
			persona = excluded.persona,
			values_json = excluded.values_json,
			tone = excluded.tone,
			domains_json = excluded.domains_json,
			user_expectations = excluded.user_expectations,
			current_phase = excluded.current_phase,
			metadata_json = excluded.metadata_json,
			version = excluded.version,
			updated_at = excluded.updated_at`,
		id.AgentID, id.Persona, encodeJSONStringSlice(id.Values), id.Tone, encodeJSONStringSlice(id.Domains),
		id.UserExpectations, id.CurrentPhase, encodeJSONObject(id.Metadata), id.Version, id.CreatedAt, id.UpdatedAt,
	)
	if err != nil {
		return domain.AgentIdentity{}, err
	}
	return r.GetAgentIdentity(id.AgentID)
}

func (r *EvolutionRepository) GetAgentStrategyProfile(agentID string) (domain.AgentStrategyProfile, error) {
	if agentID == "" {
		return domain.AgentStrategyProfile{}, errors.New("agent id is required")
	}
	row := r.db.QueryRow(
		`SELECT agent_id, exploration, conciseness, caution, delegation,
				tool_preference_json, tool_blacklist_json, provider_preference_json, model_preference_json,
				stats_json, metadata_json, version, created_at, updated_at
		 FROM agent_strategy_profile WHERE agent_id = ?`, agentID,
	)
	var (
		v        domain.AgentStrategyProfile
		toolPref string
		toolBL   string
		provPref string
		modelPrf string
		statsRaw string
		metaRaw  string
	)
	if err := row.Scan(&v.AgentID, &v.Exploration, &v.Conciseness, &v.Caution, &v.Delegation,
		&toolPref, &toolBL, &provPref, &modelPrf, &statsRaw, &metaRaw, &v.Version, &v.CreatedAt, &v.UpdatedAt); err != nil {
		return domain.AgentStrategyProfile{}, err
	}
	v.ToolPreference = decodeJSONFloatMap(toolPref)
	v.ToolBlacklist = decodeJSONStringSlice(toolBL)
	v.ProviderPreference = decodeJSONFloatMap(provPref)
	v.ModelPreference = decodeJSONFloatMap(modelPrf)
	v.Stats = decodeJSONObject(statsRaw)
	v.Metadata = decodeJSONObject(metaRaw)
	return v, nil
}

func (r *EvolutionRepository) UpsertAgentStrategyProfile(p domain.AgentStrategyProfile) (domain.AgentStrategyProfile, error) {
	if p.AgentID == "" {
		return domain.AgentStrategyProfile{}, errors.New("agent id is required")
	}
	if p.Exploration == 0 {
		p.Exploration = 0.5
	}
	if p.Conciseness == 0 {
		p.Conciseness = 0.5
	}
	if p.Caution == 0 {
		p.Caution = 0.5
	}
	if p.Delegation == 0 {
		p.Delegation = 0.5
	}
	if p.Version <= 0 {
		p.Version = 1
	}
	now := nowISO()
	if p.CreatedAt == "" {
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	_, err := r.db.Exec(
		`INSERT INTO agent_strategy_profile(
			agent_id, exploration, conciseness, caution, delegation,
			tool_preference_json, tool_blacklist_json, provider_preference_json, model_preference_json,
			stats_json, metadata_json, version, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(agent_id) DO UPDATE SET
			exploration = excluded.exploration,
			conciseness = excluded.conciseness,
			caution = excluded.caution,
			delegation = excluded.delegation,
			tool_preference_json = excluded.tool_preference_json,
			tool_blacklist_json = excluded.tool_blacklist_json,
			provider_preference_json = excluded.provider_preference_json,
			model_preference_json = excluded.model_preference_json,
			stats_json = excluded.stats_json,
			metadata_json = excluded.metadata_json,
			version = excluded.version,
			updated_at = excluded.updated_at`,
		p.AgentID, p.Exploration, p.Conciseness, p.Caution, p.Delegation,
		encodeJSONFloatMap(p.ToolPreference), encodeJSONStringSlice(p.ToolBlacklist), encodeJSONFloatMap(p.ProviderPreference), encodeJSONFloatMap(p.ModelPreference),
		encodeJSONObject(p.Stats), encodeJSONObject(p.Metadata), p.Version, p.CreatedAt, p.UpdatedAt,
	)
	if err != nil {
		return domain.AgentStrategyProfile{}, err
	}
	return r.GetAgentStrategyProfile(p.AgentID)
}

func (r *EvolutionRepository) InsertEvolutionEvent(e domain.EvolutionEvent) (domain.EvolutionEvent, error) {
	if e.ID == "" {
		return domain.EvolutionEvent{}, errors.New("event id is required")
	}
	if e.AgentID == "" {
		return domain.EvolutionEvent{}, errors.New("agent id is required")
	}
	if e.Kind == "" {
		return domain.EvolutionEvent{}, errors.New("event_kind is required")
	}
	if e.TriggerKind == "" {
		e.TriggerKind = domain.EvoTriggerUser
	}
	now := nowISO()
	if e.CreatedAt == "" {
		e.CreatedAt = now
	}
	if e.Applied && e.AppliedAt == "" {
		e.AppliedAt = now
	}
	if e.DiffJSON == "" {
		e.DiffJSON = "{}"
	}
	_, err := r.db.Exec(
		`INSERT INTO agent_evolution_events(
			id, agent_id, workspace_id, event_kind, target_field,
			before_json, after_json, diff_json,
			trigger_kind, trigger_source, evidence_json, reason,
			applied, reverted, reverted_by_event_id,
			metadata_json, created_at, applied_at, reverted_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.AgentID, e.WorkspaceID, e.Kind, e.TargetField,
		e.BeforeJSON, e.AfterJSON, e.DiffJSON,
		e.TriggerKind, e.TriggerSource, encodeEvidenceList(e.Evidence), e.Reason,
		boolToInt(e.Applied), boolToInt(e.Reverted), e.RevertedByEventID,
		encodeJSONObject(e.Metadata), e.CreatedAt, e.AppliedAt, e.RevertedAt,
	)
	if err != nil {
		return domain.EvolutionEvent{}, err
	}
	return r.GetEvolutionEvent(e.ID)
}

func (r *EvolutionRepository) GetEvolutionEvent(id string) (domain.EvolutionEvent, error) {
	row := r.db.QueryRow(
		`SELECT id, agent_id, workspace_id, event_kind, target_field,
				before_json, after_json, diff_json,
				trigger_kind, trigger_source, evidence_json, reason,
				applied, reverted, reverted_by_event_id,
				metadata_json, created_at, applied_at, reverted_at
		 FROM agent_evolution_events WHERE id = ?`, id,
	)
	return scanEvolutionEvent(row)
}

func scanEvolutionEvent(s rowScanner) (domain.EvolutionEvent, error) {
	var (
		v        domain.EvolutionEvent
		evidence string
		metadata string
		applied  int
		reverted int
	)
	if err := s.Scan(&v.ID, &v.AgentID, &v.WorkspaceID, &v.Kind, &v.TargetField,
		&v.BeforeJSON, &v.AfterJSON, &v.DiffJSON,
		&v.TriggerKind, &v.TriggerSource, &evidence, &v.Reason,
		&applied, &reverted, &v.RevertedByEventID,
		&metadata, &v.CreatedAt, &v.AppliedAt, &v.RevertedAt); err != nil {
		return domain.EvolutionEvent{}, err
	}
	v.Applied = applied != 0
	v.Reverted = reverted != 0
	v.Evidence = decodeEvidenceList(evidence)
	v.Metadata = decodeJSONObject(metadata)
	return v, nil
}

func (r *EvolutionRepository) ListEvolutionEvents(q contracts.EvolutionEventQuery) ([]domain.EvolutionEvent, int, error) {
	conds := []string{}
	args := []any{}
	if q.AgentID != "" {
		conds = append(conds, "agent_id = ?")
		args = append(args, q.AgentID)
	}
	if q.WorkspaceID != "" {
		conds = append(conds, "workspace_id = ?")
		args = append(args, q.WorkspaceID)
	}
	if q.Kind != "" {
		conds = append(conds, "event_kind = ?")
		args = append(args, q.Kind)
	}
	if q.TriggerKind != "" {
		conds = append(conds, "trigger_kind = ?")
		args = append(args, q.TriggerKind)
	}
	if q.Reverted != nil {
		conds = append(conds, "reverted = ?")
		args = append(args, boolToInt(*q.Reverted))
	}
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}
	var total int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM agent_evolution_events`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	limit := q.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := q.Offset
	if offset < 0 {
		offset = 0
	}
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, limit, offset)
	rows, err := r.db.Query(
		`SELECT id, agent_id, workspace_id, event_kind, target_field,
				before_json, after_json, diff_json,
				trigger_kind, trigger_source, evidence_json, reason,
				applied, reverted, reverted_by_event_id,
				metadata_json, created_at, applied_at, reverted_at
		 FROM agent_evolution_events`+where+` ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		listArgs...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []domain.EvolutionEvent{}
	for rows.Next() {
		v, err := scanEvolutionEvent(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, v)
	}
	return out, total, rows.Err()
}

func (r *EvolutionRepository) MarkEvolutionEventReverted(id, byEventID, atISO string) error {
	if id == "" {
		return errors.New("event id is required")
	}
	if atISO == "" {
		atISO = nowISO()
	}
	_, err := r.db.Exec(
		`UPDATE agent_evolution_events SET reverted = 1, reverted_by_event_id = ?, reverted_at = ? WHERE id = ?`,
		byEventID, atISO, id,
	)
	return err
}

func (r *EvolutionRepository) InsertEvolutionProposal(p domain.EvolutionProposal) (domain.EvolutionProposal, error) {
	if p.ID == "" {
		return domain.EvolutionProposal{}, errors.New("proposal id is required")
	}
	if p.AgentID == "" {
		return domain.EvolutionProposal{}, errors.New("agent id is required")
	}
	if p.Kind == "" {
		return domain.EvolutionProposal{}, errors.New("proposal_kind is required")
	}
	if p.Source == "" {
		p.Source = domain.EvoSourceUser
	}
	if p.Status == "" {
		p.Status = domain.EvoProposalPending
	}
	if p.RiskLevel == "" {
		p.RiskLevel = domain.EvoRiskLow
	}
	if p.DiffJSON == "" {
		p.DiffJSON = "{}"
	}
	now := nowISO()
	if p.CreatedAt == "" {
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	_, err := r.db.Exec(
		`INSERT INTO agent_evolution_proposals(
			id, agent_id, workspace_id, proposal_kind, target_field,
			proposed_value_json, current_value_json, diff_json,
			rationale, evidence_json, expected_impact, risk_level, approval_required,
			status, reviewed_by, reviewed_at, applied_event_id, expires_at,
			source, metadata_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.AgentID, p.WorkspaceID, p.Kind, p.TargetField,
		p.ProposedValueJSON, p.CurrentValueJSON, p.DiffJSON,
		p.Rationale, encodeEvidenceList(p.Evidence), p.ExpectedImpact, p.RiskLevel, boolToInt(p.ApprovalRequired),
		p.Status, p.ReviewedBy, p.ReviewedAt, p.AppliedEventID, p.ExpiresAt,
		p.Source, encodeJSONObject(p.Metadata), p.CreatedAt, p.UpdatedAt,
	)
	if err != nil {
		return domain.EvolutionProposal{}, err
	}
	return r.GetEvolutionProposal(p.ID)
}

func (r *EvolutionRepository) GetEvolutionProposal(id string) (domain.EvolutionProposal, error) {
	row := r.db.QueryRow(
		`SELECT id, agent_id, workspace_id, proposal_kind, target_field,
				proposed_value_json, current_value_json, diff_json,
				rationale, evidence_json, expected_impact, risk_level, approval_required,
				status, reviewed_by, reviewed_at, applied_event_id, expires_at,
				source, metadata_json, created_at, updated_at
		 FROM agent_evolution_proposals WHERE id = ?`, id,
	)
	return scanEvolutionProposal(row)
}

func scanEvolutionProposal(s rowScanner) (domain.EvolutionProposal, error) {
	var (
		v          domain.EvolutionProposal
		evidence   string
		metadata   string
		approval   int
	)
	if err := s.Scan(&v.ID, &v.AgentID, &v.WorkspaceID, &v.Kind, &v.TargetField,
		&v.ProposedValueJSON, &v.CurrentValueJSON, &v.DiffJSON,
		&v.Rationale, &evidence, &v.ExpectedImpact, &v.RiskLevel, &approval,
		&v.Status, &v.ReviewedBy, &v.ReviewedAt, &v.AppliedEventID, &v.ExpiresAt,
		&v.Source, &metadata, &v.CreatedAt, &v.UpdatedAt); err != nil {
		return domain.EvolutionProposal{}, err
	}
	v.ApprovalRequired = approval != 0
	v.Evidence = decodeEvidenceList(evidence)
	v.Metadata = decodeJSONObject(metadata)
	return v, nil
}

func (r *EvolutionRepository) ListEvolutionProposals(q contracts.EvolutionProposalQuery) ([]domain.EvolutionProposal, int, error) {
	conds := []string{}
	args := []any{}
	if q.AgentID != "" {
		conds = append(conds, "agent_id = ?")
		args = append(args, q.AgentID)
	}
	if q.WorkspaceID != "" {
		conds = append(conds, "workspace_id = ?")
		args = append(args, q.WorkspaceID)
	}
	if q.Status != "" {
		conds = append(conds, "status = ?")
		args = append(args, q.Status)
	}
	if q.RiskLevel != "" {
		conds = append(conds, "risk_level = ?")
		args = append(args, q.RiskLevel)
	}
	if q.Source != "" {
		conds = append(conds, "source = ?")
		args = append(args, q.Source)
	}
	if q.TargetField != "" {
		conds = append(conds, "target_field = ?")
		args = append(args, q.TargetField)
	}
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}
	var total int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM agent_evolution_proposals`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	limit := q.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := q.Offset
	if offset < 0 {
		offset = 0
	}
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, limit, offset)
	rows, err := r.db.Query(
		`SELECT id, agent_id, workspace_id, proposal_kind, target_field,
				proposed_value_json, current_value_json, diff_json,
				rationale, evidence_json, expected_impact, risk_level, approval_required,
				status, reviewed_by, reviewed_at, applied_event_id, expires_at,
				source, metadata_json, created_at, updated_at
		 FROM agent_evolution_proposals`+where+` ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		listArgs...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []domain.EvolutionProposal{}
	for rows.Next() {
		v, err := scanEvolutionProposal(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, v)
	}
	return out, total, rows.Err()
}

func (r *EvolutionRepository) UpdateEvolutionProposalStatus(id, status, by, eventID, atISO string) error {
	if id == "" {
		return errors.New("proposal id is required")
	}
	if atISO == "" {
		atISO = nowISO()
	}
	_, err := r.db.Exec(
		`UPDATE agent_evolution_proposals SET status = ?, reviewed_by = ?, reviewed_at = ?, applied_event_id = ?, updated_at = ? WHERE id = ?`,
		status, by, atISO, eventID, atISO, id,
	)
	return err
}

// SupersedeProposalsByTarget 将某智能体在 target_field 上、创建于 `sinceISO` 之前的待处理提案标为已取代。供 §5.5 节流窗口使用。
func (r *EvolutionRepository) SupersedeProposalsByTarget(agentID, targetField, sinceISO string) (int, error) {
	if agentID == "" || targetField == "" {
		return 0, errors.New("agent id and target_field are required")
	}
	if sinceISO == "" {
		sinceISO = nowISO()
	}
	res, err := r.db.Exec(
		`UPDATE agent_evolution_proposals SET status = ?, updated_at = ? WHERE agent_id = ? AND target_field = ? AND status = ? AND created_at < ?`,
		domain.EvoProposalSuperseded, nowISO(), agentID, targetField, domain.EvoProposalPending, sinceISO,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (r *EvolutionRepository) UpsertAgentSkillStat(s domain.AgentSkillStat) (domain.AgentSkillStat, error) {
	if s.AgentID == "" || s.ToolKey == "" {
		return domain.AgentSkillStat{}, errors.New("agent_id and tool_key are required")
	}
	if s.Scope == "" {
		s.Scope = "overall"
	}
	if s.PreferenceScore == 0 {
		s.PreferenceScore = 0.5
	}
	now := nowISO()
	if s.UpdatedAt == "" {
		s.UpdatedAt = now
	}
	_, err := r.db.Exec(
		`INSERT INTO agent_skill_stats(
			agent_id, scope, scope_value, tool_key,
			invocations, successes, failures, user_overrides,
			avg_latency_ms, avg_tokens, preference_score, last_used_at,
			metadata_json, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(agent_id, scope, scope_value, tool_key) DO UPDATE SET
			invocations = excluded.invocations,
			successes = excluded.successes,
			failures = excluded.failures,
			user_overrides = excluded.user_overrides,
			avg_latency_ms = excluded.avg_latency_ms,
			avg_tokens = excluded.avg_tokens,
			preference_score = excluded.preference_score,
			last_used_at = excluded.last_used_at,
			metadata_json = excluded.metadata_json,
			updated_at = excluded.updated_at`,
		s.AgentID, s.Scope, s.ScopeValue, s.ToolKey,
		s.Invocations, s.Successes, s.Failures, s.UserOverrides,
		s.AvgLatencyMS, s.AvgTokens, s.PreferenceScore, s.LastUsedAt,
		encodeJSONObject(s.Metadata), s.UpdatedAt,
	)
	if err != nil {
		return domain.AgentSkillStat{}, err
	}
	return r.GetAgentSkillStat(s.AgentID, s.Scope, s.ScopeValue, s.ToolKey)
}

func (r *EvolutionRepository) GetAgentSkillStat(agentID, scope, scopeValue, toolKey string) (domain.AgentSkillStat, error) {
	if agentID == "" || toolKey == "" {
		return domain.AgentSkillStat{}, errors.New("agent_id and tool_key are required")
	}
	if scope == "" {
		scope = "overall"
	}
	row := r.db.QueryRow(
		`SELECT agent_id, scope, scope_value, tool_key,
				invocations, successes, failures, user_overrides,
				avg_latency_ms, avg_tokens, preference_score, last_used_at,
				metadata_json, updated_at
		 FROM agent_skill_stats WHERE agent_id = ? AND scope = ? AND scope_value = ? AND tool_key = ?`,
		agentID, scope, scopeValue, toolKey,
	)
	return scanAgentSkillStat(row)
}

func scanAgentSkillStat(s rowScanner) (domain.AgentSkillStat, error) {
	var (
		v       domain.AgentSkillStat
		metaRaw string
	)
	if err := s.Scan(&v.AgentID, &v.Scope, &v.ScopeValue, &v.ToolKey,
		&v.Invocations, &v.Successes, &v.Failures, &v.UserOverrides,
		&v.AvgLatencyMS, &v.AvgTokens, &v.PreferenceScore, &v.LastUsedAt,
		&metaRaw, &v.UpdatedAt); err != nil {
		return domain.AgentSkillStat{}, err
	}
	v.Metadata = decodeJSONObject(metaRaw)
	return v, nil
}

func (r *EvolutionRepository) ListAgentSkillStats(agentID string, limit int) ([]domain.AgentSkillStat, error) {
	if agentID == "" {
		return nil, errors.New("agent id is required")
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := r.db.Query(
		`SELECT agent_id, scope, scope_value, tool_key,
				invocations, successes, failures, user_overrides,
				avg_latency_ms, avg_tokens, preference_score, last_used_at,
				metadata_json, updated_at
		 FROM agent_skill_stats WHERE agent_id = ? ORDER BY preference_score DESC, invocations DESC LIMIT ?`,
		agentID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.AgentSkillStat{}
	for rows.Next() {
		v, err := scanAgentSkillStat(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// 重构时防止未使用的 import。
var _ = sql.ErrNoRows
