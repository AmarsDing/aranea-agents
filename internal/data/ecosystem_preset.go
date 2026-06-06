package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

type EcosystemPresetRepo struct {
	data *Data
	lg   loggateway.Logger
}

var _ biz.EcosystemPresetRepo = (*EcosystemPresetRepo)(nil)

// NewEcosystemPresetRepo implements biz.EcosystemPresetRepo.
func NewEcosystemPresetRepo(d *Data) *EcosystemPresetRepo {
	return &EcosystemPresetRepo{data: d, lg: d.lg}
}

func (r *EcosystemPresetRepo) GetEcosystemLoaded(ctx context.Context) (biz.EcosystemLoadedStatus, error) {
	var raw string
	err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx),
		`SELECT ecosystem_loaded FROM system_settings WHERE id = 1`, nil, &raw)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if raw == "" {
		return nil, nil
	}
	var status biz.EcosystemLoadedStatus
	if err := json.Unmarshal([]byte(raw), &status); err != nil {
		return nil, fmt.Errorf("parse ecosystem_loaded: %w", err)
	}
	return status, nil
}

func (r *EcosystemPresetRepo) SetEcosystemLoaded(ctx context.Context, status biz.EcosystemLoadedStatus) error {
	raw, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("marshal ecosystem_loaded: %w", err)
	}
	_, err = r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`UPDATE system_settings SET ecosystem_loaded = ? WHERE id = 1`, string(raw))
	return err
}

func (r *EcosystemPresetRepo) DeleteTaxonomyNodesByIndustry(ctx context.Context, industryKey string) (int, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	// Find the industry node ID
	var industryID string
	err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx),
		`SELECT id FROM industry_taxonomy WHERE taxonomy_key = ? AND level = 'industry' AND deleted_at = ''`,
		[]any{industryKey}, &industryID)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}

	// Find department IDs
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT id FROM industry_taxonomy WHERE parent_id = ? AND deleted_at = ''`, industryID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var deptIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return 0, err
		}
		deptIDs = append(deptIDs, id)
	}

	var positionIDs []string
	if len(deptIDs) > 0 {
		// Find position IDs
		posRows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
			`SELECT id FROM industry_taxonomy WHERE parent_id IN (`+placeholders(len(deptIDs))+`) AND deleted_at = ''`,
			toAnySlice(deptIDs)...)
		if err != nil {
			return 0, err
		}
		defer posRows.Close()
		for posRows.Next() {
			var id string
			if err := posRows.Scan(&id); err != nil {
				return 0, err
			}
			positionIDs = append(positionIDs, id)
		}
	}

	total := 0

	// Soft-delete positions
	if len(positionIDs) > 0 {
		res, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
			`UPDATE industry_taxonomy SET deleted_at = ?, status = 'deleted', updated_at = ? WHERE id IN (`+placeholders(len(positionIDs))+`) AND deleted_at = ''`,
			append([]any{now, now}, toAnySlice(positionIDs)...)...)
		if err != nil {
			return total, err
		}
		n, _ := res.RowsAffected()
		total += int(n)
	}

	// Soft-delete departments
	if len(deptIDs) > 0 {
		res, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
			`UPDATE industry_taxonomy SET deleted_at = ?, status = 'deleted', updated_at = ? WHERE id IN (`+placeholders(len(deptIDs))+`) AND deleted_at = ''`,
			append([]any{now, now}, toAnySlice(deptIDs)...)...)
		if err != nil {
			return total, err
		}
		n, _ := res.RowsAffected()
		total += int(n)
	}

	// Soft-delete industry node
	res, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`UPDATE industry_taxonomy SET deleted_at = ?, status = 'deleted', updated_at = ? WHERE id = ? AND deleted_at = ''`,
		now, now, industryID)
	if err != nil {
		return total, err
	}
	n, _ := res.RowsAffected()
	total += int(n)

	return total, nil
}

func (r *EcosystemPresetRepo) DeleteAgentsByIndustry(ctx context.Context, industryKey string) (int, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	// Find position IDs for the industry
	positionIDs, err := r.findIndustryPositionIDs(ctx, industryKey)
	if err != nil {
		return 0, err
	}
	if len(positionIDs) == 0 {
		return 0, nil
	}

	// Find agent IDs before soft-deleting (needed for cascade cleanup)
	agentIDs, err := r.findEcosystemAgentIDsByPositions(ctx, positionIDs)
	if err != nil {
		return 0, err
	}
	if len(agentIDs) == 0 {
		return 0, nil
	}

	// Soft-delete agents where kind='ecosystem_preset' AND source='imported' AND taxonomy_position_id IN (position IDs)
	positionArgs := toAnySlice(positionIDs)
	res, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`UPDATE agents SET deleted_at = ?, updated_at = ? WHERE kind = 'ecosystem_preset' AND source = 'imported' AND taxonomy_position_id IN (`+placeholders(len(positionIDs))+`) AND deleted_at = ''`,
		append([]any{now, now}, positionArgs...)...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()

	// Cascade cleanup for each deleted agent (runtime_settings, prompt_files, sessions)
	for agentID := range agentIDs {
		cascadeDeleteByAgent(ctx, r.data, agentID)
	}

	return int(n), nil
}

func (r *EcosystemPresetRepo) DeleteTeamsByIndustry(ctx context.Context, industryKey string) (int, int, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	positionIDs, err := r.findIndustryPositionIDs(ctx, industryKey)
	if err != nil {
		return 0, 0, err
	}

	deletedAgentIDs, err := r.findEcosystemAgentIDsByPositions(ctx, positionIDs)
	if err != nil {
		return 0, 0, err
	}

	teamsToDelete, teamsToModify, err := r.classifyTeamsByIndustry(ctx, deletedAgentIDs)
	if err != nil {
		return 0, 0, err
	}

	deleted, err := r.softDeleteTeams(ctx, now, teamsToDelete)
	if err != nil {
		return deleted, len(teamsToModify), err
	}

	modified, err := r.modifyTeamDefinitions(ctx, now, teamsToModify)
	if err != nil {
		return deleted, modified, err
	}

	return deleted, modified, nil
}

// findEcosystemAgentIDsByPositions returns agent IDs for ecosystem_preset agents in the given positions.
func (r *EcosystemPresetRepo) findEcosystemAgentIDsByPositions(ctx context.Context, positionIDs []string) (map[string]bool, error) {
	deletedAgentIDs := map[string]bool{}
	if len(positionIDs) == 0 {
		return deletedAgentIDs, nil
	}
	agentRows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT id FROM agents WHERE kind = 'ecosystem_preset' AND source = 'imported' AND taxonomy_position_id IN (`+placeholders(len(positionIDs))+`) AND deleted_at = ''`,
		toAnySlice(positionIDs)...)
	if err != nil {
		return nil, err
	}
	defer agentRows.Close()
	for agentRows.Next() {
		var id string
		if err := agentRows.Scan(&id); err != nil {
			return nil, err
		}
		deletedAgentIDs[id] = true
	}
	return deletedAgentIDs, nil
}

type teamModifyEntry struct {
	id         string
	newDefJSON string
}

// classifyTeamsByIndustry classifies ecosystem_preset teams into delete vs modify lists.
func (r *EcosystemPresetRepo) classifyTeamsByIndustry(ctx context.Context, deletedAgentIDs map[string]bool) ([]string, []teamModifyEntry, error) {
	teamRows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT id, definition_json FROM teams WHERE kind = 'ecosystem_preset' AND source = 'imported' AND deleted_at = ''`)
	if err != nil {
		return nil, nil, err
	}
	defer teamRows.Close()

	var teamsToDelete []string
	var teamsToModify []teamModifyEntry

	for teamRows.Next() {
		var id, defJSON string
		if err := teamRows.Scan(&id, &defJSON); err != nil {
			return nil, nil, err
		}

		memberIDs := extractMemberAgentIDs(defJSON, r.lg)
		allBelongToIndustry := true
		anyBelongsToIndustry := false

		for _, mid := range memberIDs {
			if deletedAgentIDs[mid] {
				anyBelongsToIndustry = true
			} else {
				allBelongToIndustry = false
			}
		}

		if allBelongToIndustry && anyBelongsToIndustry {
			teamsToDelete = append(teamsToDelete, id)
		} else if anyBelongsToIndustry {
			newDefJSON, err := removeMembersFromDefinition(defJSON, deletedAgentIDs)
			if err != nil {
				return nil, nil, fmt.Errorf("modify team %s definition: %w", id, err)
			}
			teamsToModify = append(teamsToModify, teamModifyEntry{id: id, newDefJSON: newDefJSON})
		}
	}
	return teamsToDelete, teamsToModify, nil
}

// softDeleteTeams soft-deletes the given team IDs with cascade cleanup and returns the count deleted.
func (r *EcosystemPresetRepo) softDeleteTeams(ctx context.Context, now string, ids []string) (int, error) {
	deleted := 0
	for _, id := range ids {
		_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
			`UPDATE teams SET deleted_at = ?, updated_at = ? WHERE id = ? AND deleted_at = ''`,
			now, now, id)
		if err != nil {
			return deleted, err
		}
		cascadeDeleteByTeam(ctx, r.data, id)
		deleted++
	}
	return deleted, nil
}

// modifyTeamDefinitions updates definition_json for teams that need member removal.
func (r *EcosystemPresetRepo) modifyTeamDefinitions(ctx context.Context, now string, entries []teamModifyEntry) (int, error) {
	modified := 0
	for _, tm := range entries {
		_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
			`UPDATE teams SET definition_json = ?, updated_at = ? WHERE id = ? AND deleted_at = ''`,
			tm.newDefJSON, now, tm.id)
		if err != nil {
			return modified, err
		}
		modified++
	}
	return modified, nil
}

// findIndustryPositionIDs returns all position-level taxonomy node IDs for the given industry.
func (r *EcosystemPresetRepo) findIndustryPositionIDs(ctx context.Context, industryKey string) ([]string, error) {
	// Find the industry node ID
	var industryID string
	err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx),
		`SELECT id FROM industry_taxonomy WHERE taxonomy_key = ? AND level = 'industry' AND deleted_at = ''`,
		[]any{industryKey}, &industryID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	// Find department IDs
	deptRows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT id FROM industry_taxonomy WHERE parent_id = ? AND deleted_at = ''`, industryID)
	if err != nil {
		return nil, err
	}
	defer deptRows.Close()
	var deptIDs []string
	for deptRows.Next() {
		var id string
		if err := deptRows.Scan(&id); err != nil {
			return nil, err
		}
		deptIDs = append(deptIDs, id)
	}

	if len(deptIDs) == 0 {
		return nil, nil
	}

	// Find position IDs
	posRows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT id FROM industry_taxonomy WHERE parent_id IN (`+placeholders(len(deptIDs))+`) AND deleted_at = ''`,
		toAnySlice(deptIDs)...)
	if err != nil {
		return nil, err
	}
	defer posRows.Close()
	var positionIDs []string
	for posRows.Next() {
		var id string
		if err := posRows.Scan(&id); err != nil {
			return nil, err
		}
		positionIDs = append(positionIDs, id)
	}

	return positionIDs, nil
}

// extractMemberAgentIDs parses the team definition_json and returns member agent IDs.
func extractMemberAgentIDs(defJSON string, lg loggateway.Logger) []string {
	var def struct {
		Members []struct {
			AgentID string `json:"agent_id"`
		} `json:"members"`
	}
	if err := json.Unmarshal([]byte(defJSON), &def); err != nil {
		lg.Warn("failed to parse team definition_json in extractMemberAgentIDs",
			loggateway.Err(err))
		return nil
	}
	ids := make([]string, 0, len(def.Members))
	for _, m := range def.Members {
		if m.AgentID != "" {
			ids = append(ids, m.AgentID)
		}
	}
	return ids
}

// removeMembersFromDefinition removes members whose agent_id is in the deleted set.
func removeMembersFromDefinition(defJSON string, deletedIDs map[string]bool) (string, error) {
	var def map[string]any
	if err := json.Unmarshal([]byte(defJSON), &def); err != nil {
		return "", err
	}
	membersRaw, ok := def["members"]
	if !ok {
		return defJSON, nil
	}
	membersArr, ok := membersRaw.([]any)
	if !ok {
		return defJSON, nil
	}
	var newMembers []any
	for _, m := range membersArr {
		mMap, ok := m.(map[string]any)
		if !ok {
			newMembers = append(newMembers, m)
			continue
		}
		agentID, _ := mMap["agent_id"].(string)
		if deletedIDs[agentID] {
			continue
		}
		newMembers = append(newMembers, m)
	}
	def["members"] = newMembers
	out, err := json.Marshal(def)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// toAnySlice converts a string slice to an any slice.
func toAnySlice(ss []string) []any {
	a := make([]any, len(ss))
	for i, s := range ss {
		a[i] = s
	}
	return a
}
