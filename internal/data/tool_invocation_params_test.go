package data

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// TestToolRepo_RecordToolInvocationParams_RoundTrip covers the restored
// tool_invocation_params write path: write → read back → redaction → upsert
// idempotence, plus the tool_invocations.invocation_id backfill that lets the
// frontend locate the params row.
func TestToolRepo_RecordToolInvocationParams_RoundTrip(t *testing.T) {
	ctx := context.Background()
	repo, _ := newToolTestRepo(t)

	// 1) Recording an invocation backfills tool_invocations.invocation_id from
	// ToolCallID so the runs list can drive GET /v1/tools/runs/{id}/params.
	if err := repo.RecordToolInvocation(ctx, biz.ToolInvocationWrite{
		ToolKey:    "web_fetch",
		Status:     "success",
		ToolCallID: "tc-params-1",
		Source:     biz.ToolInvocationSourceRuntime,
	}); err != nil {
		t.Fatalf("RecordToolInvocation: %v", err)
	}
	runs, err := repo.SearchToolInvocations(ctx, biz.ToolRunQuery{ToolKey: "web_fetch", Limit: 10})
	if err != nil {
		t.Fatalf("SearchToolInvocations: %v", err)
	}
	if len(runs.Items) != 1 || runs.Items[0].InvocationID != "tc-params-1" {
		t.Fatalf("invocation_id backfill: got %+v", runs.Items)
	}

	// 2) Params row written through the usecase must be redacted (no secret
	// plaintext in params_json) and readable via GetToolInvocationParams.
	uc := biz.NewToolUsecase(repo, nil, loggateway.NewNoop())
	err = uc.RecordToolInvocationParams(ctx, biz.ToolInvocationParamWrite{
		InvocationID:     "tc-params-1",
		ToolKey:          "web_fetch",
		ParamsJSON:       `{"url":"https://example.com","api_key":"sk-live-secret-12345"}`,
		RedactionApplied: true,
	})
	if err != nil {
		t.Fatalf("RecordToolInvocationParams: %v", err)
	}
	p, err := repo.GetToolInvocationParams(ctx, "tc-params-1")
	if err != nil {
		t.Fatalf("GetToolInvocationParams: %v", err)
	}
	if p.ToolKey != "web_fetch" || p.InvocationID != "tc-params-1" {
		t.Fatalf("params identity mismatch: %+v", p)
	}
	if !p.RedactionApplied {
		t.Fatal("redaction_applied: want true")
	}
	if strings.Contains(p.ParamsJSON, "sk-live-secret-12345") {
		t.Fatalf("params_json leaked secret plaintext: %s", p.ParamsJSON)
	}
	if !strings.Contains(p.ParamsJSON, "https://example.com") {
		t.Fatalf("params_json lost non-secret content: %s", p.ParamsJSON)
	}

	// 3) Upsert semantics: re-recording the same invocation_id must not error
	// nor produce duplicate rows.
	if err := uc.RecordToolInvocationParams(ctx, biz.ToolInvocationParamWrite{
		InvocationID:     "tc-params-1",
		ToolKey:          "web_fetch",
		ParamsJSON:       `{"url":"https://example.com"}`,
		RedactionApplied: true,
	}); err != nil {
		t.Fatalf("RecordToolInvocationParams (dup): %v", err)
	}
	var count int
	if err := entQueryRowScan(repo.(*toolRepo).data.RW().Read(ctx), ctx,
		`SELECT COUNT(1) FROM tool_invocation_params WHERE invocation_id = $1`, []any{"tc-params-1"}, &count); err != nil {
		t.Fatalf("count params rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("duplicate params rows: want 1, got %d", count)
	}

	// 4) Empty invocation_id is a no-op.
	if err := repo.RecordToolInvocationParams(ctx, biz.ToolInvocationParamWrite{ToolKey: "x"}); err != nil {
		t.Fatalf("empty invocation_id must be a no-op: %v", err)
	}
}
