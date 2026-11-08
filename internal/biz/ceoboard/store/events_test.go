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
