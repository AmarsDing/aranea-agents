package biz

import (
	"reflect"
	"testing"
	"time"
)

// evalTimeTestAnchor is 2025-05-07 (a Wednesday) 15:30 UTC.
var evalTimeTestAnchor = time.Date(2025, 5, 7, 15, 30, 0, 0, time.UTC)

func TestResolveEvalRelativeTimes_Chinese(t *testing.T) {
	t.Parallel()
	a := evalTimeTestAnchor
	cases := []struct {
		name string
		stmt string
		want []string
	}{
		{"empty", "", nil},
		{"no date expr", "我喜欢咖啡", nil},
		{"今天", "今天天气不错", []string{"2025-05-07"}},
		{"昨天", "昨天我去了医院检查身体", []string{"2025-05-06"}},
		{"前天", "前天我搬到了新家", []string{"2025-05-05"}},
		{"大前天", "大前天我提交了报告", []string{"2025-05-04"}},
		{"明天", "明天我要出差去上海", []string{"2025-05-08"}},
		{"后天", "后天我要参加婚礼", []string{"2025-05-09"}},
		// anchor is Wednesday 2025-05-07; natural week Monday = 2025-05-05
		{"本周三", "本周三开会", []string{"2025-05-07"}},
		{"本周日", "本周日休息", []string{"2025-05-11"}},
		{"上周三", "上周三我见了客户", []string{"2025-04-30"}},
		{"上周天", "上周天爬山去了", []string{"2025-05-04"}},
		{"下周一", "下周一交周报", []string{"2025-05-12"}},
		{"上个周日", "上个周日去钓鱼", []string{"2025-05-04"}},
		{"上个月", "上个月我把城市改成上海", []string{"2025-04-01"}},
		{"下个月", "下个月房租到期", []string{"2025-06-01"}},
		{"去年", "去年我在罗马工作", []string{"2024-05-07"}},
		{"明年", "明年打算换工作", []string{"2026-05-07"}},
		{"三天前", "三天前我买了新手机", []string{"2025-05-04"}},
		{"10天后", "10天后项目上线", []string{"2025-05-17"}},
		{"两周前", "两周前开始了健身", []string{"2025-04-23"}},
		{"3周后", "3周后放假", []string{"2025-05-28"}},
		{"一个月前", "一个月前入职了新公司", []string{"2025-04-07"}},
		{"两个月后", "两个月后搬家", []string{"2025-07-07"}},
		{"multiple deduped", "昨天和前天都加班，昨天还熬夜了", []string{"2025-05-06", "2025-05-05"}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ResolveEvalRelativeTimes(tc.stmt, a)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ResolveEvalRelativeTimes(%q) = %v, want %v", tc.stmt, got, tc.want)
			}
		})
	}
}

func TestResolveEvalRelativeTimes_English(t *testing.T) {
	t.Parallel()
	a := evalTimeTestAnchor // Wednesday 2025-05-07
	cases := []struct {
		name string
		stmt string
		want []string
	}{
		{"today", "I have a meeting today", []string{"2025-05-07"}},
		{"yesterday", "I visited the doctor yesterday", []string{"2025-05-06"}},
		{"tomorrow", "I fly to Rome tomorrow", []string{"2025-05-08"}},
		{"last week", "I changed jobs last week", []string{"2025-04-28"}},
		{"next week", "Ship release next week", []string{"2025-05-12"}},
		{"last month", "I moved last month", []string{"2025-04-01"}},
		{"next month", "Rent is due next month", []string{"2025-06-01"}},
		// anchor Wednesday 2025-05-07 → most recent Monday strictly before = 2025-05-05
		{"last monday", "I signed the lease last Monday", []string{"2025-05-05"}},
		// last Wednesday strictly before today = one full week back
		{"last wednesday", "We met last Wednesday", []string{"2025-04-30"}},
		{"in 5 days", "The release ships in 5 days", []string{"2025-05-12"}},
		{"3 days ago", "I bought a phone 3 days ago", []string{"2025-05-04"}},
		{"case insensitive", "YESTERDAY was busy, TOMORROW is free", []string{"2025-05-06", "2025-05-08"}},
		{"no english date", "I like espresso", nil},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ResolveEvalRelativeTimes(tc.stmt, a)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ResolveEvalRelativeTimes(%q) = %v, want %v", tc.stmt, got, tc.want)
			}
		})
	}
}
