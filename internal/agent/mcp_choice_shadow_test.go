package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/decision"
	"aranea-agents/internal/decide"
	"aranea-agents/pkg/loggateway"
)

type captureDecision struct{ recs []decision.Record }

func (c *captureDecision) Emit(_ context.Context, r decision.Record) { c.recs = append(c.recs, r) }

func TestNewMCPChoiceShadowBeforeHook_NilWithoutDeps(t *testing.T) {
	if hook := newMCPChoiceShadowBeforeHook(biz.Agent{ID: "a"}, TRPCBuilderDeps{}); hook != nil {
		t.Fatal("missing MCP tooling and collector must yield nil hook")
	}
}

func TestEmitMCPServerChoiceShadow_MountAllOmitsSecrets(t *testing.T) {
	cap := &captureDecision{}
	rec := decide.NewRecorder(cap, loggateway.NewNoop())
	emitMCPServerChoiceShadow(context.Background(), rec, "查仓库 issue", []biz.EffectiveMCPServer{
		{ServerKey: "github", ConfigJSON: `{"headers":{"Authorization":"secret"}}`},
		{ServerKey: "linear", ConfigJSON: `{"token":"secret"}`},
	})
	if len(cap.recs) != 1 {
		t.Fatalf("want 1 record, got %d", len(cap.recs))
	}
	got := cap.recs[0]
	if err := got.Validate(); err != nil {
		t.Fatal(err)
	}
	if got.Category != decision.CategoryJEVShadow {
		t.Fatalf("category=%q", got.Category)
	}
	if got.Outcome != "q=mcp_server_choice;k=mount_all" {
		t.Fatalf("outcome=%q", got.Outcome)
	}
	raw := mustJSON(t, got.Metadata)
	if strings.Contains(raw, "secret") || strings.Contains(raw, "Authorization") {
		t.Fatalf("shadow must not carry config secrets: %s", raw)
	}
	if !strings.Contains(raw, "github") || !strings.Contains(raw, "linear") {
		t.Fatalf("server names missing: %s", raw)
	}
}

func TestEmitMCPServerChoiceShadow_SingleAndNone(t *testing.T) {
	cap := &captureDecision{}
	rec := decide.NewRecorder(cap, loggateway.NewNoop())
	emitMCPServerChoiceShadow(context.Background(), rec, "task", []biz.EffectiveMCPServer{{ServerKey: "github"}})
	emitMCPServerChoiceShadow(context.Background(), rec, "task", nil)
	emitMCPServerChoiceShadow(context.Background(), rec, "  ", []biz.EffectiveMCPServer{{ServerKey: "github"}})
	if len(cap.recs) != 2 {
		t.Fatalf("want 2 records (blank task dropped), got %d", len(cap.recs))
	}
	if cap.recs[0].Outcome != "q=mcp_server_choice;k=github" {
		t.Fatalf("single outcome=%q", cap.recs[0].Outcome)
	}
	if cap.recs[1].Outcome != "q=mcp_server_choice;k=none" {
		t.Fatalf("empty outcome=%q", cap.recs[1].Outcome)
	}
}

func TestEmitMCPServerChoiceShadow_LongKeyFallsBack(t *testing.T) {
	cap := &captureDecision{}
	rec := decide.NewRecorder(cap, loggateway.NewNoop())
	long := strings.Repeat("s", 80)
	emitMCPServerChoiceShadow(context.Background(), rec, "task", []biz.EffectiveMCPServer{{ServerKey: long}})
	if len(cap.recs) != 1 {
		t.Fatal("expected record")
	}
	if cap.recs[0].Outcome != "q=mcp_server_choice;k=mount_all" {
		t.Fatalf("outcome=%q", cap.recs[0].Outcome)
	}
	if err := cap.recs[0].Validate(); err != nil {
		t.Fatal(err)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
