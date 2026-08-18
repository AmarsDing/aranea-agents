// Package memoryremember provides the memory_remember explicit-memory tool
// (FR-M4). When the user explicitly asks the agent to remember something
// ("记住…/以后都…/不要再…"), the tool writes a preference/constraint fact
// immediately, applying the same conflict governance as the auto-memory
// pipeline (FR-M2).
//
// Identity security: agentID is injected by the assembly closure and userID
// is resolved from the invocation session at call time — the LLM can never
// specify whose memory to write.
package memoryremember

import (
	"context"
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

const (
	sourceKindExplicit = "explicit"
	explicitConfidence = 0.95
	explicitImportance = 0.8
)

// Deps wires the remember tool. Writer and AgentID are required; Detector and
// ConflictStore enable conflict governance (nil disables it gracefully).
type Deps struct {
	Writer        biz.MemoryConsolidationWriter
	Detector      biz.MemoryConflictDetector
	ConflictStore biz.L3ConflictStore
	AgentID       string
	LG            loggateway.Logger
}

type rememberInput struct {
	Statement string `json:"statement" jsonschema:"description=要记住的偏好、身份或工作要求陈述,required"`
	Kind      string `json:"kind" jsonschema:"description=记忆类型：preference（偏好，默认）、user_identity（工号/姓名/职责）或 constraint（硬性约束）,enum=preference,enum=user_identity,enum=constraint"`
}

type rememberOutput struct {
	FactID       string `json:"fact_id"`
	Action       string `json:"action"` // created | superseded | marked_conflict
	Kind         string `json:"kind"`
	TargetFactID string `json:"target_fact_id,omitempty"`
	Statement    string `json:"statement"`
}

// NewRememberTool builds the memory_remember function tool. Returns nil when
// Writer is nil (memory disabled) so the assembly skips registration.
func NewRememberTool(deps Deps) trpctool.CallableTool {
	if deps.Writer == nil {
		return nil
	}
	lg := deps.LG
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	execute := func(ctx context.Context, input rememberInput) (rememberOutput, error) {
		stmt := biz.NormalizeStatementPunctuation(input.Statement)
		if stmt == "" {
			return rememberOutput{}, apierror.BadRequest("MEMORY", "statement is required")
		}
		kind := strings.ToLower(strings.TrimSpace(input.Kind))
		if kind == "" {
			kind = "preference"
		}
		if kind != "preference" && kind != "constraint" && kind != "user_identity" {
			return rememberOutput{}, apierror.BadRequest("MEMORY", "kind must be preference, user_identity, or constraint")
		}
		kind = biz.CanonicalizeFactKind(kind, stmt)
		agentID := strings.TrimSpace(deps.AgentID)
		if agentID == "" {
			return rememberOutput{}, apierror.Internal("MEMORY", "agent identity not injected")
		}
		userID, sessionID := invocationIdentity(ctx)
		if userID == "" {
			return rememberOutput{}, apierror.BadRequest("MEMORY", "user identity unavailable in this context")
		}

		// FR-M2 conflict governance: best-effort, never blocks the write.
		decision := biz.MemoryConflictDecision{Action: biz.ConflictActionNone}
		if deps.Detector != nil {
			if dec, err := deps.Detector.DetectConflict(ctx, agentID, userID, kind, stmt); err == nil {
				decision = dec
			}
		}

		result, err := deps.Writer.UpsertFactsAndEpisodeBatch(ctx, []biz.MemoryFactWrite{{
			ScopeType:       "user",
			ScopeID:         userID,
			UserID:          userID,
			AgentID:         agentID,
			Statement:       stmt,
			FactKind:        kind,
			Confidence:      explicitConfidence,
			Importance:      explicitImportance,
			SourceKind:      sourceKindExplicit,
			SourceSessionID: sessionID,
			Status:          "active",
		}}, nil)
		if err != nil {
			return rememberOutput{}, err
		}

		out := rememberOutput{Action: "created", Kind: kind, Statement: stmt}
		if result != nil && len(result.FactRows) > 0 {
			out.FactID = factIDFromRow(result.FactRows[0])
		}
		out.Action, out.TargetFactID = applyConflictDecision(ctx, deps, decision, out.FactID, lg)
		return out, nil
	}
	return function.NewFunctionTool(
		execute,
		function.WithName("memory_remember"),
		function.WithDescription("当用户明确要求记住某个偏好、身份或工作要求时（如\"记住\"\"以后都…\"\"不要再…\"），立即写入长期记忆。kind=preference 表示偏好（默认），user_identity 表示工号/姓名/职责，constraint 表示硬性约束。不要用于临时上下文或任务信息。"),
	)
}

// invocationIdentity extracts user/session identity from the trpc invocation
// carried by the tool call context. Identity never comes from tool input.
func invocationIdentity(ctx context.Context) (userID, sessionID string) {
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil || inv.Session == nil {
		return "", ""
	}
	return strings.TrimSpace(inv.Session.UserID), strings.TrimSpace(inv.Session.ID)
}

// applyConflictDecision applies the governance action post-write. Failures
// are logged and swallowed — governance must never fail the memory write.
func applyConflictDecision(ctx context.Context, deps Deps, dec biz.MemoryConflictDecision, newFactID string, lg loggateway.Logger) (action, target string) {
	if deps.ConflictStore == nil || dec.TargetFactID == "" {
		return "created", ""
	}
	switch dec.Action {
	case biz.ConflictActionSupersede:
		if newFactID == "" || newFactID == dec.TargetFactID {
			return "created", ""
		}
		if err := deps.ConflictStore.SupersedeFact(ctx, dec.TargetFactID, newFactID); err != nil {
			lg.Warn("memory_remember supersede 失败", loggateway.StepID("memory.remember.supersede"), loggateway.Err(err))
			return "created", ""
		}
		return "superseded", dec.TargetFactID
	case biz.ConflictActionMarkConflict:
		if err := deps.ConflictStore.BatchIncrementConflictCounts(ctx, []string{dec.TargetFactID}); err != nil {
			lg.Warn("memory_remember 冲突标记失败", loggateway.StepID("memory.remember.mark_conflict"), loggateway.Err(err))
			return "created", ""
		}
		return "marked_conflict", dec.TargetFactID
	default:
		return "created", ""
	}
}

func factIDFromRow(raw []byte) string {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	id, _ := m["id"].(string)
	return id
}
