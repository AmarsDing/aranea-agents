package biz

// D6 L-3 embedding 密级闸测试（验收 #7：L3+ 内容走 cloud embed → 拒+审计）。

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

func jsonHasKey(raw, key string) bool {
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return false
	}
	_, ok := m[key]
	return ok
}

type embedGateFakePolicyRepo struct {
	policy *SecopsModelPolicy
	err    error
}

func (r *embedGateFakePolicyRepo) GetPolicy(context.Context) (*SecopsModelPolicy, error) {
	return r.policy, r.err
}

func (r *embedGateFakePolicyRepo) SetPolicy(context.Context, int, string) error { return nil }

type embedGateAuditSink struct {
	entries []AuditLog
	err     error
}

func (s *embedGateAuditSink) InsertAuditLog(_ context.Context, e AuditLog) error {
	if s.err != nil {
		return s.err
	}
	s.entries = append(s.entries, e)
	return nil
}

func newEmbedGate(policy *SecopsModelPolicy, policyErr error, sink *embedGateAuditSink) *SecopsEmbeddingGate {
	uc := NewSecopsModelPolicyUsecase(&embedGateFakePolicyRepo{policy: policy, err: policyErr}, loggateway.NewNoop())
	g := NewSecopsEmbeddingGate(uc, loggateway.NewNoop())
	if sink != nil {
		g.SetAuditRecorder(sink)
	}
	return g
}

func TestSecopsEmbeddingGate_DisabledNil(t *testing.T) {
	var g *SecopsEmbeddingGate
	if err := g.CheckEmbed(context.Background(), EmbedGateInfo{Classification: 5}); err != nil {
		t.Fatalf("nil gate must pass through, got %v", err)
	}
	// policy 未装配 = 停用。
	g2 := NewSecopsEmbeddingGate(nil, nil)
	if err := g2.CheckEmbed(context.Background(), EmbedGateInfo{Classification: 5}); err != nil {
		t.Fatalf("unwired gate must pass through, got %v", err)
	}
}

func TestSecopsEmbeddingGate_AllowMatrix(t *testing.T) {
	policy := &SecopsModelPolicy{LocalThreshold: 3}
	sink := &embedGateAuditSink{}
	g := newEmbedGate(policy, nil, sink)
	cases := []struct {
		name  string
		info  EmbedGateInfo
	}{
		{"unclassified", EmbedGateInfo{Classification: 0}},
		{"below threshold", EmbedGateInfo{Classification: 2}},
		{"l3 but local channel", EmbedGateInfo{Classification: 3, Local: true}},
		{"l5 local channel", EmbedGateInfo{Classification: 5, Local: true}},
	}
	for _, tc := range cases {
		if err := g.CheckEmbed(context.Background(), tc.info); err != nil {
			t.Fatalf("%s: want allow, got %v", tc.name, err)
		}
	}
	if len(sink.entries) != 0 {
		t.Fatalf("allow path must not audit, got %d entries", len(sink.entries))
	}
}

func TestSecopsEmbeddingGate_BlocksCloudAtThreshold(t *testing.T) {
	policy := &SecopsModelPolicy{LocalThreshold: 3}
	sink := &embedGateAuditSink{}
	g := newEmbedGate(policy, nil, sink)
	err := g.CheckEmbed(context.Background(), EmbedGateInfo{
		Provider: "openai", BaseURL: "https://api.example.com", Local: false,
		Classification: 3, Subject: "doc-1", BatchSize: 4,
	})
	if !apierror.IsCode(err, apierror.CodeForbidden) {
		t.Fatalf("L3 cloud must be Forbidden, got %v", err)
	}
	if len(sink.entries) != 1 {
		t.Fatalf("blocked must audit once, got %d", len(sink.entries))
	}
	entry := sink.entries[0]
	if entry.Action != SecopsEmbeddingActionBlocked || entry.ResourceID != "doc-1" || entry.Severity != "warning" {
		t.Fatalf("audit entry = %+v", entry)
	}
	// 审计 detail 不得携带文本内容（明文三不进）——只含密级/阈值/通道/计数。
	for _, k := range []string{"classification", "threshold", "provider", "batch_size"} {
		if !jsonHasKey(entry.Detail, k) {
			t.Fatalf("audit detail missing %s: %s", k, entry.Detail)
		}
	}
}

func TestSecopsEmbeddingGate_BlocksAboveThreshold(t *testing.T) {
	g := newEmbedGate(&SecopsModelPolicy{LocalThreshold: 3}, nil, nil)
	if err := g.CheckEmbed(context.Background(), EmbedGateInfo{Classification: 5}); !apierror.IsCode(err, apierror.CodeForbidden) {
		t.Fatalf("L5 cloud must be Forbidden, got %v", err)
	}
}

func TestSecopsEmbeddingGate_AuditFailureStillBlocks(t *testing.T) {
	sink := &embedGateAuditSink{err: errors.New("audit db down")}
	g := newEmbedGate(&SecopsModelPolicy{LocalThreshold: 3}, nil, sink)
	// 审计失败不翻转拒绝决策（best-effort：不存在「放行依赖审计成功」的面）。
	if err := g.CheckEmbed(context.Background(), EmbedGateInfo{Classification: 4}); !apierror.IsCode(err, apierror.CodeForbidden) {
		t.Fatalf("audit failure must not flip block decision, got %v", err)
	}
}

func TestSecopsEmbeddingGate_PolicyReadFailureFallsBackDefault(t *testing.T) {
	g := newEmbedGate(nil, errors.New("policy db down"), nil)
	// 默认阈值 3：L3 仍被拦（fail-closed 向），L2 放行。
	if err := g.CheckEmbed(context.Background(), EmbedGateInfo{Classification: 3}); !apierror.IsCode(err, apierror.CodeForbidden) {
		t.Fatalf("policy read failure must fall back to default threshold 3 (L3 blocked), got %v", err)
	}
	if err := g.CheckEmbed(context.Background(), EmbedGateInfo{Classification: 2}); err != nil {
		t.Fatalf("L2 must pass under default threshold, got %v", err)
	}
}

func TestSecopsEmbeddingGate_CustomThreshold(t *testing.T) {
	g := newEmbedGate(&SecopsModelPolicy{LocalThreshold: 5}, nil, nil)
	if err := g.CheckEmbed(context.Background(), EmbedGateInfo{Classification: 4}); err != nil {
		t.Fatalf("L4 < threshold 5 must pass, got %v", err)
	}
	if err := g.CheckEmbed(context.Background(), EmbedGateInfo{Classification: 5}); !apierror.IsCode(err, apierror.CodeForbidden) {
		t.Fatalf("L5 >= threshold 5 must block, got %v", err)
	}
}
