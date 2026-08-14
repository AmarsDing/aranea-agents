// Package projectstate provides the update_project_state tool for P2-4
// 中期 project-state JSON（Ensemble QSP 有界记忆对齐）。
//
// When a Team Definition has EnableProjectState=true, the graph runtime
// injects a "project_state" StateField (MergeReducer) into the graph schema
// and members receive this tool. Agents roll structured project state
// (active requests / recent changes / milestones / decision digest) as they
// work; the memory-inject before-model hook renders a budgeted slice into
// member prompts, replacing full conversation-history concatenation on long
// tasks. All caps/truncation live in biz.TeamProjectState — the tool only
// does read-modify-write against graph state.
package projectstate

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"aranea-agents/internal/biz"

	"trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// UpdateProjectStateTool lets an agent roll the team's structured project
// state in graph state. Read-modify-write semantics: the current map is read
// from the invocation (node-start RuntimeState snapshot, falling back to
// session state outside graph runs), updates are applied with caps, and the
// complete map is written back via StateDelta (MergeReducer top-level key
// union ⇒ whole-value replace for this single-key field).
type UpdateProjectStateTool struct{}

// NewUpdateProjectStateTool creates the update_project_state tool.
func NewUpdateProjectStateTool() *UpdateProjectStateTool { return &UpdateProjectStateTool{} }

type changeInput struct {
	Actor   string `json:"actor" jsonschema:"description=Who made the change (your agent key/role). Defaults to the caller identity."`
	Summary string `json:"summary" jsonschema:"description=One-line change summary (≤120 chars, truncated beyond).,required"`
}

type updateProjectStateInput struct {
	// Change rolls one entry into the recent-changes window (newest first,
	// capped at 8). Optional.
	Change *changeInput `json:"change" jsonschema:"description=Optional recent-change entry to roll into the state."`
	// Milestone records a milestone (newest first, capped at 8). Optional.
	Milestone string `json:"milestone" jsonschema:"description=Optional milestone to record (e.g. 'v1 方案评审通过')."`
	// DecisionDigest replaces the decision digest (≤400 chars). Optional.
	DecisionDigest string `json:"decision_digest" jsonschema:"description=Optional replacement for the team decision digest."`
	// ActiveRequests replaces the active-request set (capped at 8). Optional.
	ActiveRequests []string `json:"active_requests" jsonschema:"description=Optional full replacement of the active request list."`
}

type updateProjectStateOutput struct {
	Written bool           `json:"written"`
	Data    map[string]any `json:"data"`
	Updated []string       `json:"updated"`
}

// Declaration returns the tool metadata.
func (t *UpdateProjectStateTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name: "update_project_state",
		Description: "Roll the team's structured project state (graph state project_state field). " +
			"Use it to keep teammates aligned on long tasks WITHOUT re-reading full history: " +
			"record what you changed (change), what the team achieved (milestone), the current " +
			"decision summary (decision_digest), or the live request set (active_requests). " +
			"Each field is capped (8/8/8 entries, digest ≤400 chars); the state is injected into " +
			"every member's prompt as a budgeted slice, so keep entries terse.",
		InputSchema: &trpctool.Schema{
			Type:     "object",
			Required: []string{},
			Properties: map[string]*trpctool.Schema{
				"change": {
					Type:        "object",
					Description: "Optional recent-change entry: {actor, summary}. Summary ≤120 chars.",
					Properties: map[string]*trpctool.Schema{
						"actor":   {Type: "string", Description: "Who made the change."},
						"summary": {Type: "string", Description: "One-line change summary (≤120 chars)."},
					},
					Required: []string{"summary"},
				},
				"milestone": {
					Type:        "string",
					Description: "Optional milestone to record (newest first, capped at 8).",
				},
				"decision_digest": {
					Type:        "string",
					Description: "Optional replacement decision digest (≤400 chars).",
				},
				"active_requests": {
					Type:        "array",
					Description: "Optional full replacement of the active request list (capped at 8).",
					Items:       &trpctool.Schema{Type: "string"},
				},
			},
		},
		OutputSchema: &trpctool.Schema{
			Type:        "object",
			Description: "Confirmation that the project state was rolled.",
			Required:    []string{"written", "updated"},
			Properties: map[string]*trpctool.Schema{
				"written": {Type: "boolean", Description: "Whether the state was written."},
				"data":    {Type: "object", Description: "The complete project-state map after the update."},
				"updated": {Type: "array", Items: &trpctool.Schema{Type: "string"}, Description: "Names of the fields updated by this call."},
			},
		},
	}
}

// Call applies the requested updates and returns the complete rolled map.
// The actual graph state write happens via StateDelta, which the flow layer
// calls after Call succeeds.
func (t *UpdateProjectStateTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	if t == nil {
		return nil, errors.New("update_project_state is not configured")
	}
	var in updateProjectStateInput
	if err := json.Unmarshal(jsonArgs, &in); err != nil {
		return nil, err
	}
	if in.Change == nil && strings.TrimSpace(in.Milestone) == "" &&
		strings.TrimSpace(in.DecisionDigest) == "" && len(in.ActiveRequests) == 0 {
		return nil, errors.New("at least one of change/milestone/decision_digest/active_requests is required")
	}

	base, _ := readProjectStateMap(ctx)
	ps := biz.TeamProjectStateFromMap(base)
	updated := make([]string, 0, 4)
	if in.Change != nil {
		ps.RollChange(in.Change.Actor, in.Change.Summary)
		updated = append(updated, "recent_changes")
	}
	if s := strings.TrimSpace(in.Milestone); s != "" {
		ps.RecordMilestone(s)
		updated = append(updated, "milestones")
	}
	if s := strings.TrimSpace(in.DecisionDigest); s != "" {
		ps.SetDecisionDigest(s)
		updated = append(updated, "decision_digest")
	}
	if len(in.ActiveRequests) > 0 {
		ps.SetActiveRequests(in.ActiveRequests)
		updated = append(updated, "active_requests")
	}

	merged := ps.ToMap()
	storeLocalProjectState(ctx, merged)
	return updateProjectStateOutput{Written: true, Data: merged, Updated: updated}, nil
}

// StateDelta returns the graph state delta after a successful Call.
// The flow layer calls this via duck typing (see internal/flow/processor/functioncall.go).
// Returns nil when toolCallID is empty, resultJSON is invalid, or Written=false.
func (t *UpdateProjectStateTool) StateDelta(toolCallID string, _ []byte, resultJSON []byte) map[string][]byte {
	toolCallID = strings.TrimSpace(toolCallID)
	if toolCallID == "" || len(resultJSON) == 0 {
		return nil
	}
	var out updateProjectStateOutput
	if err := json.Unmarshal(resultJSON, &out); err != nil {
		return nil
	}
	if !out.Written || len(out.Data) == 0 {
		return nil
	}
	b, err := json.Marshal(out.Data)
	if err != nil {
		return nil
	}
	return map[string][]byte{
		biz.ProjectStateKey: b,
	}
}

// readProjectStateMap resolves the current project-state map for tool Calls.
// Precedence mirrors the deliverable tool: (1) the invocation's RuntimeState
// (node-start snapshot inside a graph run — authoritative even when empty, so
// stale session data never leaks into a running graph); (2) session state
// (the only copy outside graph runs and after run completion).
func readProjectStateMap(ctx context.Context) (map[string]any, bool) {
	inv, ok := agent.InvocationFromContext(ctx)
	if !ok || inv == nil {
		return nil, false
	}
	if inv.RunOptions.RuntimeState != nil {
		if raw, found := inv.RunOptions.RuntimeState[biz.ProjectStateKey]; found {
			if m, ok := toStringAnyMap(raw); ok {
				return m, true
			}
		}
	}
	if inv.Session != nil {
		if raw, found := inv.Session.GetState(biz.ProjectStateKey); found && len(raw) > 0 {
			var out map[string]any
			if err := json.Unmarshal(raw, &out); err == nil {
				return out, true
			}
		}
	}
	return nil, false
}

// toStringAnyMap normalizes a state value: decoded maps pass through, raw
// JSON bytes (session-seeded state) are unmarshaled.
func toStringAnyMap(v any) (map[string]any, bool) {
	switch t := v.(type) {
	case map[string]any:
		return t, true
	case []byte:
		if len(t) == 0 {
			return nil, false
		}
		var out map[string]any
		if err := json.Unmarshal(t, &out); err != nil {
			return nil, false
		}
		return out, true
	}
	return nil, false
}

// storeLocalProjectState installs the freshly written map into the
// invocation's RuntimeState so same-node reads observe read-your-writes.
// The key is replaced (never mutated in place) because the node-start
// snapshot may share sub-maps with sibling nodes. The authoritative
// cross-node write still flows via StateDelta → graph channels.
func storeLocalProjectState(ctx context.Context, merged map[string]any) {
	inv, ok := agent.InvocationFromContext(ctx)
	if !ok || inv == nil || inv.RunOptions.RuntimeState == nil {
		return
	}
	inv.RunOptions.RuntimeState[biz.ProjectStateKey] = merged
}
