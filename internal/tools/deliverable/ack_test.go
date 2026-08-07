package deliverable

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aranea-agents/internal/biz"

	"trpc.group/trpc-go/trpc-agent-go/agent"
)

// --- set_deliverable 成员级契约校验 ---

func contractForTest() *biz.MemberDeliverableContract {
	return &biz.MemberDeliverableContract{Entries: []biz.MemberDeliverableEntry{
		{Topic: "design", Required: true, RequiredKeys: []string{"arch", "api"}},
	}}
}

func TestSetDeliverableTool_ContractViolation_ReturnsStructuredError(t *testing.T) {
	tl := NewSetDeliverableToolWithContract(contractForTest())
	ctx := ctxWithRuntimeState(map[string]any{})
	_, err := tl.Call(ctx, []byte(`{"topic":"design","data":{"arch":"微服务"}}`))
	if err == nil {
		t.Fatal("expected contract violation error for missing required key")
	}
	verr, ok := err.(*biz.MemberContractViolationError)
	if !ok {
		t.Fatalf("expected *biz.MemberContractViolationError, got %T: %v", err, err)
	}
	if len(verr.Violations) != 1 || verr.Violations[0].Topic != "design" {
		t.Fatalf("unexpected violations: %+v", verr.Violations)
	}
	if !strings.Contains(err.Error(), "api") {
		t.Fatalf("error should name the missing key, got: %s", err.Error())
	}
}

func TestSetDeliverableTool_ContractSatisfied_Writes(t *testing.T) {
	tl := NewSetDeliverableToolWithContract(contractForTest())
	ctx := ctxWithRuntimeState(map[string]any{})
	out, err := tl.Call(ctx, []byte(`{"topic":"design","data":{"arch":"微服务","api":"REST"}}`))
	if err != nil {
		t.Fatalf("satisfied contract should write, got %v", err)
	}
	o := out.(setDeliverableOutput)
	if !o.Written || o.Topic != "design" {
		t.Fatalf("unexpected output: %+v", o)
	}
}

func TestSetDeliverableTool_UncontractedTopic_SkipsValidation(t *testing.T) {
	tl := NewSetDeliverableToolWithContract(contractForTest())
	ctx := ctxWithRuntimeState(map[string]any{})
	if _, err := tl.Call(ctx, []byte(`{"topic":"notes","data":{}}`)); err != nil {
		t.Fatalf("uncontracted topic must not be validated, got %v", err)
	}
	// no-topic writes are never contract-checked (legacy path)
	if _, err := tl.Call(ctx, []byte(`{"data":{}}`)); err != nil {
		t.Fatalf("no-topic write must not be validated, got %v", err)
	}
}

func TestSetDeliverableTool_NilContract_BackwardCompatible(t *testing.T) {
	tl := NewSetDeliverableTool()
	ctx := ctxWithRuntimeState(map[string]any{})
	if _, err := tl.Call(ctx, []byte(`{"topic":"design","data":{}}`)); err != nil {
		t.Fatalf("nil contract must skip validation, got %v", err)
	}
}

// TestSetDeliverableTool_CacheKeyDiscriminator guards the agent build cache:
// a contract-installed set_deliverable validates topic writes (Call rejects
// violations) while the contract-free variant accepts them — same declaration
// name, different behavior. The build cache key must therefore discriminate
// the contract variant, otherwise the team compile path (contract-installed)
// and the graph resolver path (contract-free) collide on one cache entry and
// whichever builds first silently serves the other.
func TestSetDeliverableTool_CacheKeyDiscriminator(t *testing.T) {
	free := NewSetDeliverableTool()
	if d := free.CacheKeyDiscriminator(); d != "" {
		t.Fatalf("contract-free tool must have empty discriminator, got %q", d)
	}

	c1 := NewSetDeliverableToolWithContract(contractForTest())
	c2 := NewSetDeliverableToolWithContract(contractForTest())
	d1, d2 := c1.CacheKeyDiscriminator(), c2.CacheKeyDiscriminator()
	if d1 == "" {
		t.Fatal("contract-installed tool must have a non-empty discriminator")
	}
	if d1 != d2 {
		t.Fatalf("same contract must yield the same discriminator: %q vs %q", d1, d2)
	}

	other := NewSetDeliverableToolWithContract(&biz.MemberDeliverableContract{Entries: []biz.MemberDeliverableEntry{
		{Topic: "research", Required: true, RequiredKeys: []string{"summary"}},
	}})
	if d3 := other.CacheKeyDiscriminator(); d3 == d1 {
		t.Fatal("different contracts must yield different discriminators")
	}
}

// --- ack_deliverable ---

func TestAckDeliverableTool_WritesTopLevelAckKey(t *testing.T) {
	ctx := ctxWithRuntimeState(map[string]any{
		biz.DeliverableStateKey: map[string]any{"design": map[string]any{"arch": "x"}},
	})
	ctx = ctxWithAgentName(ctx, "reviewer-agent")
	tl := NewAckDeliverableTool()
	out, err := tl.Call(ctx, []byte(`{"topic":"design","status":"accepted","comment":"架构合理"}`))
	if err != nil {
		t.Fatal(err)
	}
	o := out.(ackDeliverableOutput)
	if !o.Acked || o.Topic != "design" || o.Status != "accepted" {
		t.Fatalf("unexpected output: %+v", o)
	}
	// StateDelta must carry a top-level "ack/design" key (parallel-safe under MergeReducer)
	delta := tl.StateDelta("call-1", nil, mustJSON(t, out))
	if len(delta) != 1 {
		t.Fatalf("expected 1 delta key, got %v", delta)
	}
	var m map[string]any
	if err := json.Unmarshal(delta[biz.DeliverableStateKey], &m); err != nil {
		t.Fatal(err)
	}
	ackRaw, ok := m[AckKeyPrefix+"design"].(map[string]any)
	if !ok {
		t.Fatalf("delta map missing top-level ack key, got: %v", m)
	}
	if ackRaw["status"] != "accepted" || ackRaw["by"] != "reviewer-agent" || ackRaw["comment"] != "架构合理" {
		t.Fatalf("unexpected ack payload: %v", ackRaw)
	}
	if _, hasAt := ackRaw["at"]; !hasAt {
		t.Fatal("ack payload should carry timestamp")
	}
}

func TestAckDeliverableTool_RejectsInvalidStatus(t *testing.T) {
	tl := NewAckDeliverableTool()
	ctx := ctxWithRuntimeState(map[string]any{})
	for _, bad := range []string{"", "ok", "ACCEPTED", "pending"} {
		if _, err := tl.Call(ctx, []byte(`{"topic":"design","status":"`+bad+`"}`)); err == nil {
			t.Fatalf("status %q should be rejected", bad)
		}
	}
}

func TestAckDeliverableTool_RejectsInvalidTopic(t *testing.T) {
	tl := NewAckDeliverableTool()
	ctx := ctxWithRuntimeState(map[string]any{})
	if _, err := tl.Call(ctx, []byte(`{"topic":"Bad Topic!","status":"accepted"}`)); err == nil {
		t.Fatal("invalid topic name should be rejected")
	}
	if _, err := tl.Call(ctx, []byte(`{"topic":"summary","status":"accepted"}`)); err == nil {
		t.Fatal("reserved topic should be rejected")
	}
}

func TestAckDeliverableTool_RejectionCarriesComment(t *testing.T) {
	ctx := ctxWithRuntimeState(map[string]any{})
	tl := NewAckDeliverableTool()
	out, err := tl.Call(ctx, []byte(`{"topic":"design","status":"rejected","comment":"缺少错误处理章节"}`))
	if err != nil {
		t.Fatal(err)
	}
	o := out.(ackDeliverableOutput)
	if o.Status != "rejected" {
		t.Fatalf("unexpected status: %+v", o)
	}
}

func TestAckDeliverableTool_ByFallsBackToUnknown(t *testing.T) {
	tl := NewAckDeliverableTool()
	out, err := tl.Call(context.Background(), []byte(`{"topic":"design","status":"accepted"}`))
	if err != nil {
		t.Fatal(err)
	}
	delta := tl.StateDelta("call-1", nil, mustJSON(t, out))
	var m map[string]any
	if err := json.Unmarshal(delta[biz.DeliverableStateKey], &m); err != nil {
		t.Fatal(err)
	}
	ack := m[AckKeyPrefix+"design"].(map[string]any)
	if ack["by"] != "unknown" {
		t.Fatalf("by should fall back to unknown, got %v", ack["by"])
	}
}

// get_deliverable 现有 key 过滤即可读 ack（零新读 API）
func TestGetDeliverableTool_ReadsAckByKey(t *testing.T) {
	ctx := ctxWithRuntimeState(map[string]any{
		biz.DeliverableStateKey: map[string]any{
			"ack/design": map[string]any{"status": "accepted", "by": "reviewer"},
		},
	})
	tl := NewGetDeliverableTool()
	out, err := tl.Call(ctx, []byte(`{"key":"ack/design"}`))
	if err != nil {
		t.Fatal(err)
	}
	o := out.(getDeliverableOutput)
	if !o.Found {
		t.Fatal("ack key should be readable via existing key filter")
	}
}

// --- helpers ---

func ctxWithAgentName(ctx context.Context, name string) context.Context {
	inv, ok := agent.InvocationFromContext(ctx)
	if !ok || inv == nil {
		inv = agent.NewInvocation()
	}
	inv.AgentName = name
	return agent.NewInvocationContext(context.Background(), inv)
}
