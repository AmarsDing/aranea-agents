package knowledge

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/ctxuser"
)

// ── G-15：审计足迹扩列（rank/score/latency + 请求上下文透传）────────────────
// 契约：同 doc 多 chunk 记首开排位与最终分；session/agent 取自 SearchQuery，
// user 缺省时从请求 ctx 认证身份回填；latency 随条目落库。

func TestAccessEntries_G15_RankScoreAndIdentity(t *testing.T) {
	chunks := []biz.KnowledgeChunk{
		{ID: "c1", DocID: "d1", CollectionID: "col", Score: 0.9},
		{ID: "c2", DocID: "d1", CollectionID: "col", Score: 0.8},
		{ID: "c3", DocID: "d2", CollectionID: "col", Score: 0.7},
	}
	q := biz.KnowledgeSearchQuery{
		CollectionID: "col", Query: "q", TopK: 5,
		SessionID: "sess-1", AgentID: "agent-a", UserID: "u-9",
	}
	entries := accessEntriesFromChunks(q, newAccessContext(context.Background(), q, 42*time.Millisecond), chunks)
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2（按 doc 去重）", len(entries))
	}
	byDoc := map[string]struct {
		rank  int
		score float64
	}{}
	for _, e := range entries {
		byDoc[e.DocID] = struct {
			rank  int
			score float64
		}{e.Rank, e.Score}
		for _, check := range []struct{ name, got, want string }{
			{"collection", e.CollectionID, "col"},
			{"session", e.SessionID, "sess-1"},
			{"agent", e.AgentID, "agent-a"},
			{"user", e.UserID, "u-9"},
		} {
			if check.got != check.want {
				t.Errorf("%s = %q, want %q", check.name, check.got, check.want)
			}
		}
		if e.LatencyMs != 42 {
			t.Errorf("latency_ms = %d, want 42", e.LatencyMs)
		}
		if e.QueryHash != accessQueryHash("q") {
			t.Errorf("query_hash 与 Router 不同指纹")
		}
	}
	// d1 首个 chunk 排第 1、分 0.9；d2 首个 chunk 排第 3、分 0.7（chunk Score 为 float32）。
	if got := byDoc["d1"]; got.rank != 1 || got.score != float64(float32(0.9)) {
		t.Errorf("d1 = (rank=%d, score=%v), want (1, 0.9)", got.rank, got.score)
	}
	if got := byDoc["d2"]; got.rank != 3 || got.score != float64(float32(0.7)) {
		t.Errorf("d2 = (rank=%d, score=%v), want (3, 0.7)", got.rank, got.score)
	}
}

func TestAccessEntries_G15_UserFallbacksToContextAuth(t *testing.T) {
	chunks := []biz.KnowledgeChunk{{ID: "c1", DocID: "d1", CollectionID: "col", Score: 0.5}}
	// SearchQuery.UserID 缺省 → 从 ctx 认证身份回填。
	q := biz.KnowledgeSearchQuery{CollectionID: "col", Query: "q", TopK: 5}
	ctx := ctxuser.WithUserID(context.Background(), "u-ctx")
	entries := accessEntriesFromChunks(q, newAccessContext(ctx, q, 3*time.Millisecond), chunks)
	if len(entries) != 1 || entries[0].UserID != "u-ctx" {
		t.Fatalf("user fallback = %+v, want UserID=u-ctx", entries)
	}
	// q.UserID 优先于 ctx（调用方显式透传不被覆盖）。
	explicitQ := biz.KnowledgeSearchQuery{CollectionID: "col", Query: "q", TopK: 5, UserID: "u-explicit"}
	entries = accessEntriesFromChunks(explicitQ, newAccessContext(ctx, explicitQ, 0), chunks)
	if entries[0].UserID != "u-explicit" {
		t.Fatalf("explicit user = %q, want u-explicit", entries[0].UserID)
	}
}
