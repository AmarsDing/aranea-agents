package data

import (
	"context"
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

type agentRepo struct {
	data *Data
}

var _ biz.AgentRepository = (*agentRepo)(nil)

// NewAgentRepo implements biz.AgentRepository.
func NewAgentRepo(d *Data) biz.AgentRepository {
	return &agentRepo{data: d}
}

func normalizeJSONList(value string, lg loggateway.Logger) string {
	if strings.TrimSpace(value) == "" {
		return "[]"
	}
	if json.Valid([]byte(value)) {
		return value
	}
	b, err := json.Marshal([]string{value})
	if err != nil {
		lg.Warn("normalize json list marshal failed", loggateway.StepID("data.agent.normalize_json"), loggateway.Err(err))
		return "[]"
	}
	return string(b)
}

func normalizeJSONObj(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "{}"
	}
	if json.Valid([]byte(value)) {
		return value
	}
	return "{}"
}

func sanitizePromptFileID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return '_'
	}, value)
	return strings.Trim(value, "_")
}

func (r *agentRepo) ExecInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return r.data.ExecInTx(ctx, fn)
}

// ReorderAgents is a stub: manual ordering is not persisted (P3, LIST-07).
// Implementation requires a sort_order column migration + proto RPC + frontend wiring.
func (r *agentRepo) ReorderAgents(ctx context.Context, ids []string) error {
	return nil
}

// mustMarshalString serializes v to a JSON string. Returns "[]" on nil or error.
func mustMarshalString(v any) string {
	if v == nil {
		return "[]"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(b)
}
