package office

import (
	"context"
	"strings"
	"testing"
	"time"
)

func recurEv(start time.Time, rule, ex string) *Event {
	return &Event{ID: "ev_r", UserID: "u", Title: "站会", StartAt: start, EndAt: start.Add(30 * time.Minute),
		Timezone: "Asia/Shanghai", RRule: rule, ExDates: ex, Kind: KindPlan, Status: StatusTodo}
}

func days(ts []time.Time) string {
	loc := locOrUTC("Asia/Shanghai")
	var out []string
	for _, t := range ts {
		out = append(out, t.In(loc).Format("01-02 15:04"))
	}
	return strings.Join(out, ",")
}

func TestRecurRules(t *testing.T) {
	loc := locOrUTC("Asia/Shanghai")
	mon := time.Date(2026, 11, 16, 9, 0, 0, 0, loc) // 周一
	from, to := mon.Add(-time.Hour), mon.AddDate(0, 0, 14)
	cases := []struct {
		rule, ex, want string
	}{
		{"FREQ=DAILY;COUNT=3", "", "11-16 09:00,11-17 09:00,11-18 09:00"},
		{"FREQ=DAILY;INTERVAL=5", "", "11-16 09:00,11-21 09:00,11-26 09:00"},
		{"FREQ=WEEKLY;BYDAY=MO,WE,FR;UNTIL=20261120", "", "11-16 09:00,11-18 09:00,11-20 09:00"},
		{"FREQ=WEEKLY;BYDAY=FR,MO", "2026-11-20", "11-16 09:00,11-23 09:00,11-27 09:00"},
		{"FREQ=WEEKLY", "", "11-16 09:00,11-23 09:00"},
	}
	for _, c := range cases {
		if got := days(recurStarts(recurEv(mon, c.rule, c.ex), from, to)); got != c.want {
			t.Errorf("%s ex=%s: got %s want %s", c.rule, c.ex, got, c.want)
		}
	}
	// 每月 31 号：没有 31 号的月份跳过（不挪到下月初）
	jan31 := time.Date(2026, 1, 31, 9, 0, 0, 0, loc)
	got := days(recurStarts(recurEv(jan31, "FREQ=MONTHLY;COUNT=3", ""), jan31.Add(-time.Hour), jan31.AddDate(1, 0, 0)))
	if got != "01-31 09:00,03-31 09:00,05-31 09:00" {
		t.Errorf("monthly 31st: %s", got)
	}
	for _, bad := range []string{"FREQ=HOURLY", "FREQ=DAILY;BYDAY=MO", "FREQ=WEEKLY;BYDAY=XX", "FREQ=DAILY;INTERVAL=0", "FOO=1"} {
		if ValidRRule(bad) {
			t.Errorf("%q should be invalid", bad)
		}
	}
	if !ValidRRule("") || !ValidRRule("RRULE:FREQ=YEARLY") {
		t.Error("empty / prefixed rule should be valid")
	}
}

// TestScheduleRecurringExpandAndSearch 范围查询展开重复系列（标 RecurrenceDay），坏规则 400，搜索走仓储。
func TestScheduleRecurringExpandAndSearch(t *testing.T) {
	repo := newFakeEventRepo()
	uc := NewScheduleUsecase(repo)
	ctx := context.Background()
	loc := locOrUTC("Asia/Shanghai")
	start := time.Date(2026, 11, 16, 9, 0, 0, 0, loc)
	if _, err := uc.CreateEvent(ctx, &Event{UserID: "u", Title: "坏规则", StartAt: start, EndAt: start.Add(time.Hour), RRule: "FREQ=HOURLY"}); err != ErrInvalidRRule {
		t.Fatalf("bad rule: %v", err)
	}
	ev, err := uc.CreateEvent(ctx, &Event{UserID: "u", Title: "站会", Location: "3 楼", StartAt: start, EndAt: start.Add(15 * time.Minute),
		Timezone: "Asia/Shanghai", RRule: "FREQ=DAILY;COUNT=5"})
	if err != nil {
		t.Fatal(err)
	}
	// 重复的不挡同一时段的普通计划（shortcut：重复日程不参与重叠检测）
	if _, err := uc.CreateEvent(ctx, &Event{UserID: "u", Title: "同时段", StartAt: start, EndAt: start.Add(time.Hour)}); err != nil {
		t.Fatalf("plain plan alongside recurring: %v", err)
	}
	rows, err := uc.ListEventsRange(ctx, "u", start.AddDate(0, 0, 1), start.AddDate(0, 0, 3))
	if err != nil {
		t.Fatal(err)
	}
	var occ []string
	for _, r := range rows {
		if r.ID == ev.ID {
			occ = append(occ, r.RecurrenceDay)
		}
	}
	if strings.Join(occ, ",") != "2026-11-17,2026-11-18" {
		t.Fatalf("occurrences: %v", occ)
	}
	// 跳过一天
	ex := "2026-11-17"
	if _, err := uc.UpdateEvent(ctx, "u", &Event{ID: ev.ID}, EventPatch{ExDates: &ex}); err != nil {
		t.Fatal(err)
	}
	rows, _ = uc.ListEventsRange(ctx, "u", start.AddDate(0, 0, 1), start.AddDate(0, 0, 3))
	n := 0
	for _, r := range rows {
		if r.ID == ev.ID {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("exdate not applied: %d", n)
	}
	if hits, _ := uc.SearchEvents(ctx, "u", "3 楼", 0); len(hits) != 1 {
		t.Fatalf("search by location: %d", len(hits))
	}
}

func TestNextRecurReminder(t *testing.T) {
	loc := locOrUTC("Asia/Shanghai")
	start := time.Date(2026, 11, 16, 9, 0, 0, 0, loc)
	ev := recurEv(start, "FREQ=DAILY", "")
	ev.ReminderMinutes = 15
	now := time.Date(2026, 11, 18, 8, 50, 0, 0, loc) // 18 号 9 点那次提前 15 分钟已到
	got := nextRecurReminder(ev, now)
	if !got.Equal(time.Date(2026, 11, 18, 9, 0, 0, 0, loc)) {
		t.Fatalf("next reminder: %v", got)
	}
	ev.RemindedAt = &got
	if again := nextRecurReminder(ev, now); !again.IsZero() {
		t.Fatalf("should not remind twice: %v", again)
	}
	if early := nextRecurReminder(ev, time.Date(2026, 11, 19, 8, 0, 0, 0, loc)); !early.IsZero() {
		t.Fatalf("too early for 19th: %v", early)
	}
}

func TestICalExportsRecurrence(t *testing.T) {
	loc := locOrUTC("Asia/Shanghai")
	ev := recurEv(time.Date(2026, 11, 16, 9, 0, 0, 0, loc), "FREQ=WEEKLY;BYDAY=MO", "2026-11-23")
	var b strings.Builder
	icalExDates(&b, ev)
	if b.String() != "EXDATE:20261123T010000Z\r\n" {
		t.Fatalf("exdate: %q", b.String())
	}
}
