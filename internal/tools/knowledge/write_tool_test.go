package knowledge

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

// ── knowledge_write（P1，2026-08-15 评审修订） ──────────────────────────────

func writeToolUsecase() *biz.KnowledgeUsecase {
	repo := &mockKnowledgeRepo{}
	return biz.NewKnowledgeUsecase(repo, repo, repo)
}

func TestNewWriteTool_NilOrUnavailable(t *testing.T) {
	if tl := NewWriteTool(nil); tl != nil {
		t.Fatal("nil usecase must skip registration")
	}
	if tl := NewWriteTool(biz.NewKnowledgeUsecase(nil, nil, nil)); tl != nil {
		t.Fatal("unavailable usecase must skip registration")
	}
	if tl := NewWriteTool(writeToolUsecase()); tl == nil {
		t.Fatal("ready usecase must build the tool")
	}
}

func TestKnowledgeWriteDerivedFactID(t *testing.T) {
	a := knowledgeWriteDerivedFactID("发布窗口定在周五")
	if !strings.HasPrefix(a, "kw-") {
		t.Fatalf("derived id prefix: %q", a)
	}
	// 归一化：大小写 + 折叠空白后同陈述同键（幂等重放）。
	b := knowledgeWriteDerivedFactID("  发布窗口定在周五  ")
	if a != b {
		t.Fatalf("normalized statement must derive same id: %q vs %q", a, b)
	}
	c := knowledgeWriteDerivedFactID("Deep   Work 制度")
	d := knowledgeWriteDerivedFactID("deep work 制度")
	if c != d {
		t.Fatalf("case/space folding must derive same id: %q vs %q", c, d)
	}
	if knowledgeWriteDerivedFactID("另一条完全不同的事实陈述") == a {
		t.Fatal("distinct statements must not collide")
	}
}

func callWriteTool(t *testing.T, in knowledgeWriteInput) (knowledgeWriteOutput, error) {
	t.Helper()
	tl := NewWriteTool(writeToolUsecase())
	if tl == nil {
		t.Fatal("tool must build")
	}
	args, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	res, err := tl.Call(context.Background(), args)
	if err != nil {
		return knowledgeWriteOutput{}, err
	}
	out, ok := res.(knowledgeWriteOutput)
	if !ok {
		t.Fatalf("expected knowledgeWriteOutput, got %T", res)
	}
	return out, nil
}

func TestWriteTool_Validation(t *testing.T) {
	valid := knowledgeWriteInput{Statement: "灰度发布必须带熔断开关", Tags: []string{"灰度发布"}}
	cases := []struct {
		name string
		mut  func(*knowledgeWriteInput)
	}{
		{"empty statement", func(in *knowledgeWriteInput) { in.Statement = "  " }},
		{"short statement", func(in *knowledgeWriteInput) { in.Statement = "太短" }},
		{"bad kind", func(in *knowledgeWriteInput) { in.FactKind = "hunch" }},
		{"missing tags", func(in *knowledgeWriteInput) { in.Tags = nil }},
		{"only blank tags", func(in *knowledgeWriteInput) { in.Tags = []string{" ", ""} }},
		{"confidence below floor", func(in *knowledgeWriteInput) { in.Confidence = 0.5 }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := valid
			c.mut(&in)
			if _, err := callWriteTool(t, in); err == nil {
				t.Fatalf("expected rejection for %s", c.name)
			}
		})
	}
}

func TestWriteTool_LowConfidenceGoesPendingReview(t *testing.T) {
	out, err := callWriteTool(t, knowledgeWriteInput{
		Statement:  "灰度发布周五窗口可能改为周四，待确认",
		Tags:       []string{"灰度发布"},
		Confidence: 0.7,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != "pending_review" {
		t.Fatalf("0.6~0.85 band must route to pending_review, got %q", out.Status)
	}
	if !strings.HasPrefix(out.FactID, "kw-") || out.Entry != "" {
		t.Fatalf("pending receipt must carry derived fact id and no entry: %+v", out)
	}
}

func TestWriteTool_HighConfidenceWritesEntry(t *testing.T) {
	out, err := callWriteTool(t, knowledgeWriteInput{
		Statement: "灰度发布必须带熔断开关，违者回滚",
		Tags:      []string{"灰度发布"},
		FactKind:  "constraint",
		// Confidence 留空 → 默认 0.95 → 直写。
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != "written" {
		t.Fatalf("default confidence must direct-write, got %q", out.Status)
	}
	if out.Entry != "灰度发布" {
		t.Fatalf("entry must be first tag landing page, got %q", out.Entry)
	}

	// 幂等：显式同一 fact_id 再写入仍 written（走整段替换，不重复追加）。
	out2, err := callWriteTool(t, knowledgeWriteInput{
		Statement: "灰度发布必须带熔断开关，违者回滚",
		Tags:      []string{"灰度发布"},
		FactID:    out.FactID,
	})
	if err != nil {
		t.Fatalf("idempotent rewrite: %v", err)
	}
	if out2.Status != "written" || out2.FactID != out.FactID {
		t.Fatalf("idempotent rewrite receipt: %+v", out2)
	}
}
