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
	"net/url"
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
func (a *Applier) upsertCategory(key, name, description, parentID, level string, result *ApplyResult) (string, error) {
	if a.opts.DryRun {
		result.Skipped++
		return "dry-run-" + key, nil
	}
	payload := map[string]any{
		"key":         key,
		"name":        name,
		"description": description,
		"parent_id":   parentID,
		"level":       level,
		"status":      "active",
		"enabled":     true,
	}
	existingID, err := a.lookupCategoryByKey(key)
	if err != nil {
		return "", err
	}
	if existingID != "" {
		if _, err := a.put(fmt.Sprintf("/v1/agent-categories/%s", existingID), payload); err != nil {
			return "", err
		}
		result.Updated++
		return existingID, nil
	}
	resp, err := a.post("/v1/agent-categories", payload)
	if err != nil {
		// Race: another import created it between lookup and post → fall through to update.
		var httpErr *HTTPError
		if errors.As(err, &httpErr) && httpErr.IsConflict() {
			if existingID, lookupErr := a.lookupCategoryByKey(key); lookupErr == nil && existingID != "" {
				if _, putErr := a.put(fmt.Sprintf("/v1/agent-categories/%s", existingID), payload); putErr == nil {
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

// lookupCategoryByKey returns the category ID for the given key, or "" if absent.
func (a *Applier) lookupCategoryByKey(key string) (string, error) {
	resp, err := a.get("/v1/agent-categories?key=" + url.QueryEscape(key))
	if err != nil {
		var httpErr *HTTPError
		if errors.As(err, &httpErr) && httpErr.IsNotFound() {
			return "", nil
		}
		return "", err
	}
	if items, ok := resp["items"].([]any); ok && len(items) > 0 {
		if first, ok := items[0].(map[string]any); ok {
			if id, ok := first["id"].(string); ok {
				return id, nil
			}
		}
	}
	if id, ok := resp["id"].(string); ok {
		return id, nil
	}
	return "", nil
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
		if _, err := a.put(fmt.Sprintf("/v1/agents/%s", existingID), payload); err != nil {
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
func (a *Applier) lookupAgentByKey(key string) (string, error) {
	resp, err := a.get("/v1/agents?agent_key=" + url.QueryEscape(key))
	if err != nil {
		var httpErr *HTTPError
		if errors.As(err, &httpErr) && httpErr.IsNotFound() {
			return "", nil
		}
		return "", err
	}
	if items, ok := resp["items"].([]any); ok && len(items) > 0 {
		if first, ok := items[0].(map[string]any); ok {
			if id, ok := first["id"].(string); ok {
				return id, nil
			}
		}
	}
	if id, ok := resp["id"].(string); ok {
		return id, nil
	}
	return "", nil
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
func (a *Applier) upsertTeam(team TeamSpec, result *ApplyResult) error {
	if a.opts.DryRun {
		result.Skipped++
		return nil
	}
	memberIDs := make([]map[string]string, 0, len(team.Members))
	for _, m := range team.Members {
		memberIDs = append(memberIDs, map[string]string{
			"agent_id": a.keyToID["agent:"+m.AgentKey],
			"role":     m.Role,
		})
	}
	payload := map[string]any{
		"key":     team.Key,
		"name":    team.Name,
		"members": memberIDs,
	}
	existingID, err := a.lookupTeamByKey(team.Key)
	if err != nil {
		return err
	}
	if existingID != "" {
		if _, err := a.put(fmt.Sprintf("/v1/teams/%s", existingID), payload); err != nil {
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
func (a *Applier) lookupTeamByKey(key string) (string, error) {
	resp, err := a.get("/v1/teams?key=" + url.QueryEscape(key))
	if err != nil {
		var httpErr *HTTPError
		if errors.As(err, &httpErr) && httpErr.IsNotFound() {
			return "", nil
		}
		return "", err
	}
	if items, ok := resp["items"].([]any); ok && len(items) > 0 {
		if first, ok := items[0].(map[string]any); ok {
			if id, ok := first["id"].(string); ok {
				return id, nil
			}
		}
	}
	if id, ok := resp["id"].(string); ok {
		return id, nil
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

func (a *Applier) put(path string, payload any) (map[string]any, error) {
	return a.do("PUT", path, payload)
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
