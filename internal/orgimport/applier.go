package orgimport

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTPError carries the HTTP status code from a failed backend call so the
// CLI top layer can decide between retry / skip / abort. B-5 fix.
type HTTPError struct {
	Method string
	Path   string
	Status int
	Body   string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("%s %s returned %d: %s", e.Method, e.Path, e.Status, e.Body)
}

// IsConflict returns true for 409, signalling an idempotency conflict.
func (e *HTTPError) IsConflict() bool { return e.Status == http.StatusConflict }

// IsNotFound returns true for 404, signalling an absent resource.
func (e *HTTPError) IsNotFound() bool { return e.Status == http.StatusNotFound }

// ApplyOptions controls the apply phase.
type ApplyOptions struct {
	APIURL   string
	APIToken string
	DryRun   bool
	Timeout  time.Duration
	// Refine triggers AI refinement for each description/agent_description.
	Refine bool
	// CorrelationID is sent as X-Correlation-Id on every write request for audit tracing (PGO-4-OBS-01).
	// If empty, a random correlation ID is auto-generated per Applier instance.
	CorrelationID string
}

// ApplyResult records the outcome of the apply phase.
type ApplyResult struct {
	Created int
	Updated int
	Skipped int
	Errors  []string
}

// Applier executes the import plan against the backend HTTP API.
// It only uses HTTP calls, never importing internal biz/data packages. PGO-4.
type Applier struct {
	client        *http.Client
	opts          ApplyOptions
	keyToID       map[string]string // category/agent key → backend ID (populated during apply)
	correlationID string
}

// NewApplier creates an Applier.
func NewApplier(opts ApplyOptions) *Applier {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	cid := opts.CorrelationID
	if cid == "" {
		cid = generateCorrelationID()
	}
	return &Applier{
		client:        &http.Client{Timeout: timeout},
		opts:          opts,
		keyToID:       make(map[string]string),
		correlationID: cid,
	}
}

// generateCorrelationID generates an import correlation ID with timestamp and
// random suffix to avoid collisions across concurrent CLI runs.
func generateCorrelationID() string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("cli-import-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("cli-import-%d-%s", time.Now().UnixMilli(), hex.EncodeToString(b[:]))
}

// Apply executes the plan for a given spec.
func (a *Applier) Apply(spec *Spec) (*ApplyResult, error) {
	result := &ApplyResult{}

	// Phase 1: categories (industry → dept → position order).
	for _, ind := range spec.Spec.Companies {
		desc := ind.Description
		if a.opts.Refine {
			if refined, err := a.refineText("category.industry", "", desc, ""); err == nil && refined != "" {
				desc = refined
			}
		}
		id, err := a.upsertCategory(ind.Key, ind.Name, desc, "", "industry", result)
		if err != nil {
			return result, err
		}
		a.keyToID[ind.Key] = id

		for _, dept := range ind.Departments {
			dDesc := dept.Description
			if a.opts.Refine {
				if refined, err := a.refineText("category.department", "", dDesc, ""); err == nil && refined != "" {
					dDesc = refined
				}
			}
			dID, err := a.upsertCategory(dept.Key, dept.Name, dDesc, id, "department", result)
			if err != nil {
				return result, err
			}
			a.keyToID[dept.Key] = dID

			for _, pos := range dept.Positions {
				pDesc := pos.Description
				if a.opts.Refine {
					if refined, err := a.refineText("category.position", "", pDesc, ""); err == nil && refined != "" {
						pDesc = refined
					}
				}
				pID, err := a.upsertCategory(pos.Key, pos.Name, pDesc, dID, "position", result)
				if err != nil {
					return result, err
				}
				a.keyToID[pos.Key] = pID
			}
		}
	}

	// Phase 2: agents.
	for _, ag := range spec.Spec.Agents {
		if err := a.upsertAgent(ag, spec, result); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("agent %s: %v", ag.Key, err))
		}
	}

	// Phase 3: teams.
	for _, team := range spec.Spec.Teams {
		if err := a.upsertTeam(team, result); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("team %s: %v", team.Key, err))
		}
	}

	return result, nil
}

// ─── Category ─────────────────────────────────────────────────────────────────

// upsertCategory creates the category if absent, otherwise updates it.
// B-3 fix: previously this was POST-only, breaking idempotency.
// API note: categories are organization nodes (OrganizationService); spec level
// "industry" maps to organization level "company" (company/department/position).
func (a *Applier) upsertCategory(key, name, description, parentID, level string, result *ApplyResult) (string, error) {
	if a.opts.DryRun {
		result.Skipped++
		return "dry-run-" + key, nil
	}
	payload := map[string]any{
		"org_key":     key,
		"name":        name,
		"description": description,
		"parent_id":   parentID,
		"level":       orgLevel(level),
		"status":      "active",
		"enabled":     true,
	}
	existingID, err := a.lookupCategoryByKey(key)
	if err != nil {
		return "", err
	}
	if existingID != "" {
		// UpdateOrganization: PATCH /v1/organization/{id}, body is the node itself.
		if _, err := a.patch(fmt.Sprintf("/v1/organization/%s", existingID), payload); err != nil {
			return "", err
		}
		result.Updated++
		return existingID, nil
	}
	resp, err := a.post("/v1/organization", payload)
	if err != nil {
		// Race: another import created it between lookup and post → fall through to update.
		var httpErr *HTTPError
		if errors.As(err, &httpErr) && httpErr.IsConflict() {
			if existingID, lookupErr := a.lookupCategoryByKey(key); lookupErr == nil && existingID != "" {
				if _, patchErr := a.patch(fmt.Sprintf("/v1/organization/%s", existingID), payload); patchErr == nil {
					result.Updated++
					return existingID, nil
				}
			}
		}
		return "", err
	}
	id, _ := resp["id"].(string)
	result.Created++
	return id, nil
}

// orgLevel maps spec category levels to OrganizationNode levels.
// Spec uses "industry"; the organization API uses "company".
func orgLevel(level string) string {
	if level == "industry" {
		return "company"
	}
	return level
}

// lookupCategoryByKey returns the organization node ID for the given key, or "" if absent.
// ListOrganization has no filter params, so we fetch all nodes and exact-match orgKey client-side.
func (a *Applier) lookupCategoryByKey(key string) (string, error) {
	resp, err := a.get("/v1/organization")
	if err != nil {
		var httpErr *HTTPError
		if errors.As(err, &httpErr) && httpErr.IsNotFound() {
			return "", nil
		}
		return "", err
	}
	if items, ok := resp["items"].([]any); ok {
		for _, it := range items {
			node, ok := it.(map[string]any)
			if !ok {
				continue
			}
			if mapString(node, "orgKey", "org_key") == key {
				if id, ok := node["id"].(string); ok {
					return id, nil
				}
			}
		}
	}
	return "", nil
}

// mapString reads a string field from a protojson-decoded map, accepting both
// camelCase (server default) and snake_case keys.
func mapString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// ─── Agent ────────────────────────────────────────────────────────────────────

// upsertAgent creates the agent if absent, otherwise updates it. B-3 fix.
func (a *Applier) upsertAgent(ag AgentSpec, spec *Spec, result *ApplyResult) error {
	if a.opts.DryRun {
		result.Skipped++
		return nil
	}
	desc := ag.AgentDescription
	if a.opts.Refine && desc != "" {
		if refined, err := a.refineText("agent.description", "", desc, ""); err == nil && refined != "" {
			desc = refined
		}
	}

	catPositionID := a.resolvePositionID(ag.CategoryPosition, spec)
	payload := map[string]any{
		"agent_key":          ag.Key,
		"display_name":       ag.DisplayName,
		"provider":           ag.Provider,
		"model":              ag.Model,
		"agent_description":  desc,
		"position_id":        catPositionID,
		"system_prompt_mode": ag.SystemPromptMode,
		"status":             "active",
	}

	existingID, err := a.lookupAgentByKey(ag.Key)
	if err != nil {
		return err
	}
	var agentID string
	if existingID != "" {
		// UpdateAgent: PATCH /v1/agents/{id}, body is the Agent message itself.
		if _, err := a.patch(fmt.Sprintf("/v1/agents/%s", existingID), payload); err != nil {
			return err
		}
		agentID = existingID
		result.Updated++
	} else {
		resp, err := a.post("/v1/agents", payload)
		if err != nil {
			return err
		}
		agentID, _ = resp["id"].(string)
		result.Created++
	}
	a.keyToID["agent:"+ag.Key] = agentID

	// Upload files if specified; otherwise let the backend apply V2 defaults.
	for fileName, body := range ag.Files {
		filePayload := map[string]any{
			"name": fileName,
			"body": body,
		}
		// File upsert is delegated to the backend; conflicts there are non-fatal
		// for the import as a whole.
		if _, err := a.post(fmt.Sprintf("/v1/agents/%s/files", agentID), filePayload); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("agent %s / file %s: %v", ag.Key, fileName, err))
		}
	}
	return nil
}

// lookupAgentByKey returns the agent ID for the given key, or "" if absent.
// ListAgents has no agent_key filter (keyword is fuzzy), so we paginate and
// exact-match agentKey client-side — picking items[0] from an unfiltered list
// would silently return an arbitrary agent.
func (a *Applier) lookupAgentByKey(key string) (string, error) {
	const pageSize = 200
	for offset := 0; ; offset += pageSize {
		resp, err := a.get(fmt.Sprintf("/v1/agents?limit=%d&offset=%d", pageSize, offset))
		if err != nil {
			var httpErr *HTTPError
			if errors.As(err, &httpErr) && httpErr.IsNotFound() {
				return "", nil
			}
			return "", err
		}
		items, _ := resp["items"].([]any)
		for _, it := range items {
			ag, ok := it.(map[string]any)
			if !ok {
				continue
			}
			if mapString(ag, "agentKey", "agent_key") == key {
				if id, ok := ag["id"].(string); ok {
					return id, nil
				}
			}
		}
		if len(items) < pageSize {
			return "", nil
		}
	}
}

// resolvePositionID looks up the position's backend ID given a "ind/dept/pos" path.
func (a *Applier) resolvePositionID(path string, _ *Spec) string {
	if path == "" {
		return ""
	}
	parts := strings.Split(path, "/")
	if len(parts) != 3 {
		return ""
	}
	return a.keyToID[parts[2]]
}

// ─── Team ─────────────────────────────────────────────────────────────────────

// upsertTeam creates the team if absent, otherwise updates it. B-3 fix.
// API note: CreateTeamRequest/Team use team_key + display_name; members live in
// definition_json as an OrchestrationSpec ({version,mode,members[]}), not a
// top-level "members" field.
func (a *Applier) upsertTeam(team TeamSpec, result *ApplyResult) error {
	if a.opts.DryRun {
		result.Skipped++
		return nil
	}
	members := make([]map[string]any, 0, len(team.Members))
	for i, m := range team.Members {
		agentID := a.keyToID["agent:"+m.AgentKey]
		if agentID == "" {
			// 成员可能引用本次导入范围之外的既有 agent，按键精确查询兜底；
			// 解析失败则整队放弃（避免产出残缺团队定义），由 Apply 聚合错误后继续下一队。
			id, err := a.lookupAgentByKey(m.AgentKey)
			if err != nil {
				return fmt.Errorf("member %s: lookup agent: %w", m.AgentKey, err)
			}
			if id == "" {
				return fmt.Errorf("member %s: agent not found", m.AgentKey)
			}
			agentID = id
		}
		members = append(members, map[string]any{
			"agent_id":   agentID,
			"role":       m.Role,
			"sort_order": i,
		})
	}
	definition, err := json.Marshal(map[string]any{
		"version": 2,
		"mode":    "sequential",
		"members": members,
	})
	if err != nil {
		return fmt.Errorf("marshal team definition: %w", err)
	}
	payload := map[string]any{
		"team_key":        team.Key,
		"display_name":    team.Name,
		"definition_json": string(definition),
	}
	existingID, err := a.lookupTeamByKey(team.Key)
	if err != nil {
		return err
	}
	if existingID != "" {
		// UpdateTeam: PATCH /v1/teams/{id}, body is the Team message itself.
		if _, err := a.patch(fmt.Sprintf("/v1/teams/%s", existingID), payload); err != nil {
			return err
		}
		result.Updated++
		return nil
	}
	if _, err := a.post("/v1/teams", payload); err != nil {
		return err
	}
	result.Created++
	return nil
}

// lookupTeamByKey returns the team ID for the given key, or "" if absent.
// ListTeams has no key filter, so we fetch all and exact-match teamKey client-side.
func (a *Applier) lookupTeamByKey(key string) (string, error) {
	resp, err := a.get("/v1/teams")
	if err != nil {
		var httpErr *HTTPError
		if errors.As(err, &httpErr) && httpErr.IsNotFound() {
			return "", nil
		}
		return "", err
	}
	if items, ok := resp["items"].([]any); ok {
		for _, it := range items {
			t, ok := it.(map[string]any)
			if !ok {
				continue
			}
			if mapString(t, "teamKey", "team_key", "key") == key {
				if id, ok := t["id"].(string); ok {
					return id, nil
				}
			}
		}
	}
	return "", nil
}

// ─── Refine ───────────────────────────────────────────────────────────────────

func (a *Applier) refineText(scope, resourceID, text, hint string) (string, error) {
	if text == "" || a.opts.APIURL == "" {
		return "", nil
	}
	payload := map[string]string{
		"scope":         scope,
		"resource_id":   resourceID,
		"original_text": text,
		"user_hint":     hint,
	}
	resp, err := a.post("/v1/ai/refine", payload)
	if err != nil {
		return "", err
	}
	refined, _ := resp["refined"].(string)
	return refined, nil
}

// ─── HTTP helpers ─────────────────────────────────────────────────────────────

func (a *Applier) post(path string, payload any) (map[string]any, error) {
	return a.do("POST", path, payload)
}

func (a *Applier) patch(path string, payload any) (map[string]any, error) {
	return a.do("PATCH", path, payload)
}

func (a *Applier) get(path string) (map[string]any, error) {
	return a.do("GET", path, nil)
}

// do performs the HTTP request and returns *HTTPError on non-2xx responses
// so callers can branch on status (e.g. retry on conflict, skip on not-found).
// B-5 fix.
func (a *Applier) do(method, path string, payload any) (map[string]any, error) {
	var bodyReader io.Reader
	if payload != nil {
		body, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, a.opts.APIURL+path, bodyReader)
	if err != nil {
		return nil, err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if a.opts.APIToken != "" {
		req.Header.Set("Authorization", "Bearer "+a.opts.APIToken)
	}
	// PGO-4-OBS-01: audit headers for every CLI-initiated write.
	req.Header.Set("X-Correlation-Id", a.correlationID)
	req.Header.Set("X-Source", "cli_import")
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, &HTTPError{
			Method: method,
			Path:   path,
			Status: resp.StatusCode,
			Body:   strings.TrimSpace(string(raw)),
		}
	}
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decode response from %s %s: %w", method, path, err)
	}
	return result, nil
}
