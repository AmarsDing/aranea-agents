package data

import (
	"context"
	"strings"
	"testing"
	"time"
)

// seedSearchSession inserts one sessions row with the given workspace/agent.
func seedSearchSession(t *testing.T, d *Data, id, workspaceID, agentID string, deleted bool) {
	t.Helper()
	ctx := context.Background()
	deletedAt := ""
	if deleted {
		deletedAt = "2026-07-01T00:00:00Z"
	}
	_, err := d.RW().Write(ctx).Session.Create().
		SetID(id).
		SetTitle("session " + id).
		SetWorkspaceID(workspaceID).
		SetAgentID(agentID).
		SetDeletedAt(deletedAt).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed session %s: %v", id, err)
	}
}

// seedSearchStep inserts one steps_v2 row.
func seedSearchStep(t *testing.T, d *Data, id, sessionID, kind, content string, startedAt time.Time) {
	t.Helper()
	ctx := context.Background()
	_, err := d.RW().Write(ctx).StepV2.Create().
		SetID(id).
		SetTurnID("turn-" + id).
		SetTaskID("task-" + id).
		SetSessionID(sessionID).
		SetSpiritSessionID(sessionID).
		SetKind(kind).
		SetContent(content).
		SetStartedAt(startedAt).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed step %s: %v", id, err)
	}
}

func TestSearchGlobalMessages_WorkspaceScoped(t *testing.T) {
	d := openTestDataWithRawDB(t)
	repo := NewGlobalMessageSearchRepo(d)
	ctx := context.Background()

	base := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	// ws-a: 两个命中（reply 较早、task 较晚）+ 一个 thinking（不命中）+ 已删除会话的命中。
	seedSearchSession(t, d, "s-a1", "ws-a", "agent-1", false)
	seedSearchStep(t, d, "st-1", "s-a1", "reply", "hello 数据", base)
	seedSearchStep(t, d, "st-2", "s-a1", "task", "hello again", base.Add(time.Hour))
	seedSearchStep(t, d, "st-3", "s-a1", "thinking", "hello hidden", base.Add(2*time.Hour))
	seedSearchSession(t, d, "s-del", "ws-a", "agent-1", true)
	seedSearchStep(t, d, "st-4", "s-del", "reply", "hello deleted", base.Add(3*time.Hour))
	// ws-b: 同关键词命中，绝不能泄露到 ws-a 的结果里。
	seedSearchSession(t, d, "s-b1", "ws-b", "agent-2", false)
	seedSearchStep(t, d, "st-5", "s-b1", "reply", "hello secret", base.Add(4*time.Hour))

	// ws-a 检索：只返回 st-2、st-1（新→旧），排除 thinking/已删除/跨租户。
	hits, err := repo.SearchGlobalMessages(ctx, "hello", "", "ws-a", 20)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 2 || hits[0].ID != "st-2" || hits[1].ID != "st-1" {
		t.Fatalf("unexpected hits: %+v", hits)
	}
	if !strings.Contains(hits[0].Snippet, "hello") {
		t.Fatalf("snippet should contain keyword: %q", hits[0].Snippet)
	}

	// agent 过滤叠加 workspace。
	hits, err = repo.SearchGlobalMessages(ctx, "hello", "agent-1", "ws-a", 20)
	if err != nil || len(hits) != 2 {
		t.Fatalf("agent filter: hits=%+v err=%v", hits, err)
	}
	// 同 workspace 下不存在的 agent → 空。
	hits, err = repo.SearchGlobalMessages(ctx, "hello", "agent-2", "ws-a", 20)
	if err != nil || len(hits) != 0 {
		t.Fatalf("cross-tenant agent must be invisible in ws-a: hits=%+v err=%v", hits, err)
	}

	// ws-b 只能看到自己的。
	hits, err = repo.SearchGlobalMessages(ctx, "hello", "", "ws-b", 20)
	if err != nil || len(hits) != 1 || hits[0].ID != "st-5" {
		t.Fatalf("ws-b hits: %+v err=%v", hits, err)
	}

	// 空 workspaceID（system caller）→ 不过滤，看到全部非删除会话命中。
	hits, err = repo.SearchGlobalMessages(ctx, "hello", "", "", 20)
	if err != nil || len(hits) != 3 {
		t.Fatalf("system caller should see all non-deleted hits: %+v err=%v", hits, err)
	}
}
