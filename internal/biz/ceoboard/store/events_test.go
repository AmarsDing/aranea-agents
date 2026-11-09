// 事件查询语义测试(自 ceo-dashboard/internal/store/events_test.go 移植,
// API 加 ctx;原文保真断言改走 GetEvent,不再裸 SQL)。
package store

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestQueryEventsUsesLiteralRawSubject(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	base := time.Now().UTC()
	for i, subject := range []string{"trade.order", "trade.*", "trade.>"} {
		ev := testEvent("hcf:literal-" + subject)
		ev.Subject = subject
		ev.ReceivedAt = base.Add(time.Duration(i) * time.Second)
		if _, err := s.InsertEvent(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	allowed := []string{"trade.order", "trade.*", "trade.>"}
	for _, subject := range allowed {
		got, err := s.QueryEvents(ctx, EventFilter{Platform: "hcf", AllowedSubjects: allowed, Subject: subject})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].Subject != subject {
			t.Fatalf("raw subject 必须字面精确匹配 %q: %+v", subject, got)
		}
	}
	got, err := s.QueryEvents(ctx, EventFilter{Platform: "hcf"})
	if err != nil || len(got) != 0 {
		t.Fatalf("空白名单必须返回空集: len=%d err=%v", len(got), err)
	}
	if _, err := s.QueryEvents(ctx, EventFilter{Platform: "hcf", AllowedSubjects: allowed, Subject: "other.order"}); err == nil {
		t.Fatal("subject 不属于白名单时必须拒绝")
	}
}

func TestQueryEventsCursorStableDuringInsert(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	at := time.Now().UTC().Truncate(time.Nanosecond)
	for _, uid := range []string{"hcf:a", "hcf:b", "hcf:c"} {
		ev := testEvent(uid)
		ev.ReceivedAt = at
		if _, err := s.InsertEvent(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	filter := EventFilter{Platform: "hcf", AllowedSubjects: []string{"trade.order.v1"}, Limit: 2}
	first, err := s.QueryEvents(ctx, filter)
	if err != nil || len(first) != 2 || first[0].UID != "hcf:c" || first[1].UID != "hcf:b" {
		t.Fatalf("第一页复合排序错误: %+v err=%v", first, err)
	}
	newEvent := testEvent("hcf:d")
	newEvent.ReceivedAt = at.Add(time.Second)
	if _, err := s.InsertEvent(ctx, newEvent); err != nil {
		t.Fatal(err)
	}
	filter.Cursor = &EventCursor{ReceivedAt: first[1].ReceivedAt, UID: first[1].UID}
	second, err := s.QueryEvents(ctx, filter)
	if err != nil || len(second) != 1 || second[0].UID != "hcf:a" {
		t.Fatalf("游标不得受新写入影响: %+v err=%v", second, err)
	}
}

func TestQueryEventsRequiresPlatformForEventID(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	if _, err := s.QueryEvents(ctx, EventFilter{AllowedSubjects: []string{"trade.order.v1"}, EventID: "shared-id"}); err == nil {
		t.Fatal("event_id 查询缺少平台时必须拒绝")
	}
	ev := testEvent("hcf:platform-scoped")
	ev.EventID = "shared-id"
	if _, err := s.InsertEvent(ctx, ev); err != nil {
		t.Fatal(err)
	}
	got, err := s.QueryEvents(ctx, EventFilter{Platform: "hcf", AllowedSubjects: []string{"trade.order.v1"}, EventID: "shared-id"})
	if err != nil || len(got) != 1 || got[0].UID != ev.UID {
		t.Fatalf("平台内 event_id 精确查询错误: %+v err=%v", got, err)
	}
}

func TestGetEventReturnsFullPayload(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	ev := testEvent("hcf:detail")
	ev.Payload = `{"data":"` + strings.Repeat("x", 20*1024) + `"}`
	if _, err := s.insertEvent(ctx, ev, true); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetEvent(ctx, ev.UID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Payload != ev.Payload || !got.PayloadComplete || !got.ImportedFromSpool || got.Subject != ev.Subject {
		t.Fatalf("详情必须返回完整原文和存储标记: len=%d complete=%v spool=%v", len(got.Payload), got.PayloadComplete, got.ImportedFromSpool)
	}
}

func TestInsertEventPreservesFullPayload(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	payload := `{"data":"` + strings.Repeat("x", 16*1024) + `"}`
	ev := testEvent("hcf:full-payload")
	ev.Payload = payload
	if _, err := s.InsertEvent(ctx, ev); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetEvent(ctx, ev.UID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Payload != payload {
		t.Fatalf("新事件原文不得截断: got=%d want=%d", len(got.Payload), len(payload))
	}
	if !got.PayloadComplete {
		t.Fatal("新事件必须显式标 payload_complete")
	}
}

// 评审 2026-11-09 D1:冗余索引 (platform, received_at) 删除收敛后,其原服务面
// (RecentEvents / CountEvents / QueryEvents 游标分页)全部由 (platform,
// received_at, event_uid) 三列索引左前缀覆盖。用真实重复组(同平台同
// received_at 多行,且分页边界切开重复组)证明行为不变:计数精确、排序稳定、
// 分页跨页不重不漏。
func TestEventsDuplicatePlatformReceivedAtGroups(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Millisecond)
	groups := []struct {
		at   time.Time
		uids []string
	}{
		{base, []string{"hcf:g0-a", "hcf:g0-b", "hcf:g0-c"}},
		{base.Add(time.Second), []string{"hcf:g1-a", "hcf:g1-b", "hcf:g1-c", "hcf:g1-d"}},
		{base.Add(2 * time.Second), []string{"hcf:g2-a", "hcf:g2-b"}},
	}
	want := map[string]bool{}
	for _, g := range groups {
		for _, uid := range g.uids {
			ev := testEvent(uid)
			ev.ReceivedAt = g.at
			if _, err := s.InsertEvent(ctx, ev); err != nil {
				t.Fatal(err)
			}
			want[uid] = true
		}
	}
	// 异平台同时戳噪声:不得串进 hcf 的任何口径。
	noise := testEvent("nt:g0-noise")
	noise.Platform = "novatrader"
	noise.ReceivedAt = base
	if _, err := s.InsertEvent(ctx, noise); err != nil {
		t.Fatal(err)
	}

	// CountEvents:平台+窗口谓词(原两列索引的主服务面之一)。
	n, err := s.CountEvents(ctx, "hcf", "", base.Add(-time.Second))
	if err != nil || n != 9 {
		t.Fatalf("CountEvents 应为 9,实际 %d err=%v", n, err)
	}
	// RecentEvents:9 条全回,组间新→旧(组内并列序不做断言——该查询本就
	// 只按 received_at 排序,前后行为一致)。
	recent, err := s.RecentEvents(ctx, "hcf", 20)
	if err != nil || len(recent) != 9 {
		t.Fatalf("RecentEvents 应为 9,实际 %d err=%v", len(recent), err)
	}
	if !recent[0].ReceivedAt.Equal(base.Add(2*time.Second)) || !recent[8].ReceivedAt.Equal(base) {
		t.Fatalf("RecentEvents 组间倒序错误: first=%v last=%v", recent[0].ReceivedAt, recent[8].ReceivedAt)
	}
	// QueryEvents 游标分页 limit=4(页边界切开 g1 重复组):三页 4+4+1,
	// (received_at DESC, uid DESC) 复合序稳定,全程不重不漏。
	filter := EventFilter{Platform: "hcf", AllowedSubjects: []string{"trade.order.v1"}, Limit: 4}
	seen := map[string]int{}
	var prev *Event
	for page := 0; page < 3; page++ {
		got, err := s.QueryEvents(ctx, filter)
		if err != nil {
			t.Fatal(err)
		}
		wantLen := []int{4, 4, 1}[page]
		if len(got) != wantLen {
			t.Fatalf("第 %d 页应 %d 条,实际 %d", page, wantLen, len(got))
		}
		for i := range got {
			ev := got[i]
			seen[ev.UID]++
			if prev != nil {
				if ev.ReceivedAt.After(prev.ReceivedAt) ||
					(ev.ReceivedAt.Equal(prev.ReceivedAt) && ev.UID > prev.UID) {
					t.Fatalf("复合游标序破坏: %s(%v) 排在 %s(%v) 之后", prev.UID, prev.ReceivedAt, ev.UID, ev.ReceivedAt)
				}
			}
			prev = &got[i]
		}
		last := got[len(got)-1]
		filter.Cursor = &EventCursor{ReceivedAt: last.ReceivedAt, UID: last.UID}
	}
	if len(seen) != 9 {
		t.Fatalf("分页应覆盖 9 个不同 uid,实际 %d", len(seen))
	}
	for uid := range want {
		if seen[uid] != 1 {
			t.Fatalf("uid %s 出现 %d 次,必须恰好 1 次", uid, seen[uid])
		}
	}
}

// occurred_at 零值落库为 NULL,读回仍是零值(信封未带时间的口径)。
func TestEventOccurredAtNullRoundTrip(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	ev := testEvent("hcf:no-occ")
	if _, err := s.InsertEvent(ctx, ev); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetEvent(ctx, ev.UID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.OccurredAt.IsZero() {
		t.Fatalf("未带 occurred_at 应读回零值,实际 %v", got.OccurredAt)
	}
	occ := time.Now().Add(-time.Minute)
	ev2 := testEvent("hcf:with-occ")
	ev2.OccurredAt = occ
	if _, err := s.InsertEvent(ctx, ev2); err != nil {
		t.Fatal(err)
	}
	got2, err := s.GetEvent(ctx, ev2.UID)
	if err != nil {
		t.Fatal(err)
	}
	if !got2.OccurredAt.Equal(occ.UTC()) {
		t.Fatalf("occurred_at 应保真往返: %v ≠ %v", got2.OccurredAt, occ)
	}
}
