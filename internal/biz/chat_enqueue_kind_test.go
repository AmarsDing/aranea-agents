package biz

import (
	"strings"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// ─── P2-3 Inbox 三级注入语义（steer / followup / inject）─────────────────────
//
// steer：框架级插话，下一 step 边界消费（现有默认）。
// followup：显式追问，跳过 steer 直接入 pending 队列，当前 turn 结束后作为新
// turn 输入。
// inject：系统上下文静默排队——无活动 run 也接受、不尝试 steer、不单独唤醒
// turn，仅随下一条 followup 作为上下文前缀合入。

// kindRecordingQueue 记录入队 kind 的 pending 队列桩。
type kindRecordingQueue struct {
	stubChatPendingQueue
	entries []PendingQueueEntry
}

func (q *kindRecordingQueue) List(string) []PendingQueueEntry {
	return append([]PendingQueueEntry(nil), q.entries...)
}

func (q *kindRecordingQueue) Enqueue(_, content string) string {
	q.entries = append(q.entries, PendingQueueEntry{ID: "p1", Content: content})
	return "p1"
}

func (q *kindRecordingQueue) EnqueueFollowup(_, content string) string {
	q.entries = append(q.entries, PendingQueueEntry{ID: "p1", Content: content, Kind: ChatEnqueueKindFollowup})
	return "p1"
}

func (q *kindRecordingQueue) EnqueueInject(_, content string) string {
	q.entries = append(q.entries, PendingQueueEntry{ID: "p1", Content: content, Kind: ChatEnqueueKindInject})
	return "p1"
}

// steerRecordingGateway 记录 steer 尝试的运行网关桩。
type steerRecordingGateway struct {
	stubChatRunGateway
	steerCapable bool
	steerCalls   int
}

func (g *steerRecordingGateway) EnqueueUserMessage(_, _ string) (bool, error) {
	g.steerCalls++
	return g.steerCapable, nil
}

func newKindTestUsecase(g *steerRecordingGateway, q *kindRecordingQueue) *ChatUsecase {
	return NewChatUsecase(g, stubChatSessionLocker{}, q, &stubChatPersister{}, &stubChatEventPublisher{}, loggateway.NewNoop())
}

func TestEnqueueUserMessageWithKind_DefaultSteersFirst(t *testing.T) {
	g := &steerRecordingGateway{steerCapable: true}
	g.hasActive = true
	q := &kindRecordingQueue{}
	uc := newKindTestUsecase(g, q)

	accepted, queued, _, _, err := uc.EnqueueUserMessageWithKind("s1", "hi", "", false)
	if err != nil || !accepted || queued {
		t.Fatalf("accepted=%v queued=%v err=%v, want steer accepted", accepted, queued, err)
	}
	if g.steerCalls != 1 {
		t.Fatalf("steerCalls = %d, want 1", g.steerCalls)
	}
	if len(q.entries) != 0 {
		t.Fatalf("pending entries = %d, want 0 (steered, not queued)", len(q.entries))
	}
}

func TestEnqueueUserMessageWithKind_FollowupSkipsSteer(t *testing.T) {
	g := &steerRecordingGateway{steerCapable: true}
	g.hasActive = true
	q := &kindRecordingQueue{}
	uc := newKindTestUsecase(g, q)

	accepted, queued, pid, _, err := uc.EnqueueUserMessageWithKind("s1", "after this, do X", ChatEnqueueKindFollowup, false)
	if err != nil || !accepted || !queued || pid == "" {
		t.Fatalf("accepted=%v queued=%v pid=%q err=%v, want queued followup", accepted, queued, pid, err)
	}
	if g.steerCalls != 0 {
		t.Fatalf("steerCalls = %d, want 0 (explicit followup must not steer)", g.steerCalls)
	}
	// 空 Kind 即 followup（SplitLeadingInjects 将无 kind 旧条目按 followup 处理）；
	// 这里断言"不得是 inject 静默语义"即可。
	if len(q.entries) != 1 || q.entries[0].Kind == ChatEnqueueKindInject {
		t.Fatalf("entries = %+v, want 1 non-inject (followup) entry", q.entries)
	}
}

func TestEnqueueUserMessageWithKind_InjectAcceptedWithoutActiveRun(t *testing.T) {
	g := &steerRecordingGateway{} // no active run
	q := &kindRecordingQueue{}
	uc := newKindTestUsecase(g, q)

	accepted, queued, pid, reason, err := uc.EnqueueUserMessageWithKind("s1", "sys ctx", ChatEnqueueKindInject, false)
	if err != nil || !accepted || !queued || pid == "" {
		t.Fatalf("accepted=%v queued=%v pid=%q err=%v, want silent accept", accepted, queued, pid, err)
	}
	if reason != ChatEnqueueRejectNone {
		t.Fatalf("rejectReason = %q, want none (inject must not require active run)", reason)
	}
	if len(q.entries) != 1 || q.entries[0].Kind != ChatEnqueueKindInject {
		t.Fatalf("entries = %+v, want 1 inject entry", q.entries)
	}
}

func TestEnqueueUserMessageWithKind_InjectWithActiveRunDoesNotSteer(t *testing.T) {
	g := &steerRecordingGateway{steerCapable: true}
	g.hasActive = true
	q := &kindRecordingQueue{}
	uc := newKindTestUsecase(g, q)

	accepted, queued, _, _, err := uc.EnqueueUserMessageWithKind("s1", "sys ctx", ChatEnqueueKindInject, false)
	if err != nil || !accepted || !queued {
		t.Fatalf("accepted=%v queued=%v err=%v, want queued inject", accepted, queued, err)
	}
	if g.steerCalls != 0 {
		t.Fatalf("steerCalls = %d, want 0 (inject is silent)", g.steerCalls)
	}
	if q.entries[0].Kind != ChatEnqueueKindInject {
		t.Fatalf("kind = %q, want inject", q.entries[0].Kind)
	}
}

func TestEnqueueUserMessageWithKind_FollowupRejectedWithoutActiveRun(t *testing.T) {
	g := &steerRecordingGateway{} // no active run
	q := &kindRecordingQueue{}
	uc := newKindTestUsecase(g, q)

	accepted, queued, _, reason, err := uc.EnqueueUserMessageWithKind("s1", "hi", ChatEnqueueKindFollowup, false)
	if err != nil || accepted || queued {
		t.Fatalf("accepted=%v queued=%v err=%v, want reject", accepted, queued, err)
	}
	if reason != ChatEnqueueRejectNoActiveRun {
		t.Fatalf("rejectReason = %q, want no_active_run", reason)
	}
}

// ─── SplitLeadingInjects 纯函数 ─────────────────────────────────────────────

func TestSplitLeadingInjects(t *testing.T) {
	inj := func(c string) PendingQueueEntry { return PendingQueueEntry{Content: c, Kind: ChatEnqueueKindInject} }
	fol := func(c string) PendingQueueEntry { return PendingQueueEntry{Content: c, Kind: ChatEnqueueKindFollowup} }

	t.Run("injects before followup", func(t *testing.T) {
		injects, followup, lead, ok := SplitLeadingInjects([]PendingQueueEntry{inj("a"), inj("b"), fol("q")})
		if !ok || lead != 2 || followup.Content != "q" || strings.Join(injects, ",") != "a,b" {
			t.Fatalf("injects=%v followup=%+v lead=%d ok=%v", injects, followup, lead, ok)
		}
	})
	t.Run("only injects → not ok", func(t *testing.T) {
		if _, _, _, ok := SplitLeadingInjects([]PendingQueueEntry{inj("a"), inj("b")}); ok {
			t.Fatal("only injects must return ok=false (stay queued silently)")
		}
	})
	t.Run("head is followup", func(t *testing.T) {
		injects, followup, lead, ok := SplitLeadingInjects([]PendingQueueEntry{fol("q"), inj("a")})
		if !ok || lead != 0 || len(injects) != 0 || followup.Content != "q" {
			t.Fatalf("injects=%v followup=%+v lead=%d ok=%v", injects, followup, lead, ok)
		}
	})
	t.Run("empty", func(t *testing.T) {
		if _, _, _, ok := SplitLeadingInjects(nil); ok {
			t.Fatal("empty must return ok=false")
		}
	})
	t.Run("legacy entries without kind are followup", func(t *testing.T) {
		_, followup, lead, ok := SplitLeadingInjects([]PendingQueueEntry{{Content: "old"}})
		if !ok || lead != 0 || followup.Content != "old" {
			t.Fatalf("lead=%d followup=%+v ok=%v", lead, followup, ok)
		}
	})
}

func TestMergeInjectContext(t *testing.T) {
	got := MergeInjectContext([]string{"ctx-a", "ctx-b"}, "question")
	if !strings.Contains(got, "ctx-a") || !strings.Contains(got, "ctx-b") || !strings.HasSuffix(got, "question") {
		t.Fatalf("merged = %q", got)
	}
	if MergeInjectContext(nil, "question") != "question" {
		t.Fatal("no injects must return content unchanged")
	}
}
