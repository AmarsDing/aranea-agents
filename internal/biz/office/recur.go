package office

// 重复日程：支持日历软件里最常用的一块 RRULE（RFC 5545）——
// FREQ=DAILY|WEEKLY|MONTHLY|YEARLY、INTERVAL、BYDAY（只用于 WEEKLY）、COUNT、UNTIL（YYYYMMDD，含当天）。
// 系列在库里只有一行；按范围查询时展开成一次次（墙上时间按系列时区算，跨夏令时不漂）；
// EXDATES 是跳过的日期（某一次单独改了 / 删了）。

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

// maxRecurSteps 展开时最多走这么多步，防止坏规则把请求拖死。
const maxRecurSteps = 5000

type recurRule struct {
	freq     string
	interval int
	byDay    []time.Weekday
	count    int
	until    string // YYYYMMDD，空 = 不限
}

var weekdayCodes = map[string]time.Weekday{
	"MO": time.Monday, "TU": time.Tuesday, "WE": time.Wednesday, "TH": time.Thursday,
	"FR": time.Friday, "SA": time.Saturday, "SU": time.Sunday,
}

// parseRecur 解析规则；ok=false 表示不认识（含不支持的部件）。
func parseRecur(s string) (recurRule, bool) {
	r := recurRule{interval: 1}
	for _, part := range strings.Split(strings.TrimPrefix(strings.TrimSpace(s), "RRULE:"), ";") {
		k, v, found := strings.Cut(part, "=")
		if !found {
			return r, false
		}
		switch strings.ToUpper(k) {
		case "FREQ":
			r.freq = strings.ToUpper(v)
		case "INTERVAL":
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > 999 {
				return r, false
			}
			r.interval = n
		case "COUNT":
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > maxRecurSteps {
				return r, false
			}
			r.count = n
		case "UNTIL":
			u := v
			if len(u) > 8 {
				u = u[:8] // 20261231T235959Z 只看日期
			}
			if _, err := time.Parse("20060102", u); err != nil {
				return r, false
			}
			r.until = u
		case "BYDAY":
			for _, d := range strings.Split(strings.ToUpper(v), ",") {
				wd, ok := weekdayCodes[d]
				if !ok {
					return r, false
				}
				r.byDay = append(r.byDay, wd)
			}
		default:
			return r, false
		}
	}
	switch r.freq {
	case "DAILY", "WEEKLY", "MONTHLY", "YEARLY":
	default:
		return r, false
	}
	if len(r.byDay) > 0 && r.freq != "WEEKLY" {
		return r, false
	}
	// 周一起，按星期排好，展开时同一周里按先后出
	sort.Slice(r.byDay, func(i, j int) bool { return (r.byDay[i]+6)%7 < (r.byDay[j]+6)%7 })
	return r, true
}

// ValidRRule 空串（不重复）或认得的规则。
func ValidRRule(s string) bool {
	if strings.TrimSpace(s) == "" {
		return true
	}
	_, ok := parseRecur(s)
	return ok
}

// recurStarts 系列在 [from, to) 里各次的开始时间（含与窗口相交的：开始早于 to 且结束晚于 from）。
// 跳过 EXDATES；COUNT 按 RFC 连同被跳过的一起数。
func recurStarts(ev *Event, from, to time.Time) []time.Time {
	r, ok := parseRecur(ev.RRule)
	if !ok {
		return nil
	}
	loc := locOrUTC(ev.Timezone)
	s := ev.StartAt.In(loc)
	dur := ev.EndAt.Sub(ev.StartAt)
	skip := map[string]bool{}
	for _, d := range strings.Split(ev.ExDates, ",") {
		if d = strings.TrimSpace(d); d != "" {
			skip[d] = true
		}
	}
	at := func(y int, m time.Month, d int) (time.Time, bool) {
		t := time.Date(y, m, d, s.Hour(), s.Minute(), s.Second(), 0, loc)
		return t, t.Day() == d // 2 月 30 日之类的不存在的日子跳过（RFC 5545）
	}
	var out []time.Time
	n := 0
	emit := func(t time.Time) bool { // 返回 false = 到头了
		if r.until != "" && t.Format("20060102") > r.until {
			return false
		}
		if !t.Before(to) {
			return false
		}
		n++
		if t.Add(dur).After(from) && !skip[t.Format("2006-01-02")] {
			out = append(out, t)
		}
		return r.count == 0 || n < r.count
	}
	for step := 0; step < maxRecurSteps; step++ {
		switch r.freq {
		case "DAILY":
			t, _ := at(s.Year(), s.Month(), s.Day()+step*r.interval)
			if !emit(t) {
				return out
			}
		case "WEEKLY":
			if len(r.byDay) == 0 {
				t, _ := at(s.Year(), s.Month(), s.Day()+7*step*r.interval)
				if !emit(t) {
					return out
				}
				continue
			}
			monday := s.Day() - int((s.Weekday()+6)%7) + 7*step*r.interval
			for _, wd := range r.byDay {
				t, _ := at(s.Year(), s.Month(), monday+int((wd+6)%7))
				if t.Before(s) {
					continue
				}
				if !emit(t) {
					return out
				}
			}
		case "MONTHLY":
			y, m := s.Year(), s.Month()+time.Month(step*r.interval)
			if t, ok := at(y, m, s.Day()); ok && !emit(t) {
				return out
			}
		case "YEARLY":
			if t, ok := at(s.Year()+step*r.interval, s.Month(), s.Day()); ok && !emit(t) {
				return out
			}
		}
	}
	return out
}

// expandRecurring 把范围里的重复系列展开成一次次（复制一份、改起止、标上是哪天）；不重复的原样返回。
func expandRecurring(rows []*Event, from, to time.Time) []*Event {
	out := make([]*Event, 0, len(rows))
	for _, ev := range rows {
		if ev.RRule == "" {
			out = append(out, ev)
			continue
		}
		dur := ev.EndAt.Sub(ev.StartAt)
		loc := locOrUTC(ev.Timezone)
		for _, st := range recurStarts(ev, from, to) {
			occ := *ev
			occ.StartAt = st.UTC()
			occ.EndAt = st.Add(dur).UTC()
			occ.RecurrenceDay = st.In(loc).Format("2006-01-02")
			out = append(out, &occ)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].StartAt.Before(out[j].StartAt) })
	return out
}

// nextRecurReminder 重复计划下一次该提醒的那次：提醒时刻已到、还没提醒过（晚于 remindedAt）、
// 且不早于 now 前一小时（停机久了不补发一串旧提醒）。没有返回零值。
func nextRecurReminder(ev *Event, now time.Time) time.Time {
	lead := time.Duration(ev.ReminderMinutes) * time.Minute
	from := now.Add(-time.Hour)
	if ev.RemindedAt != nil && ev.RemindedAt.After(from) {
		from = *ev.RemindedAt
	}
	for _, st := range recurStarts(ev, from, now.Add(lead).Add(time.Second)) {
		if st.After(from) && !st.Add(-lead).After(now) && (ev.RemindedAt == nil || st.After(*ev.RemindedAt)) {
			return st
		}
	}
	return time.Time{}
}
