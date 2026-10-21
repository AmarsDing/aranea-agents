package biz

import (
	"testing"
	"time"
)

func evalQueryTestAnchor() time.Time { return evalTimeTestAnchor } // 2025-05-07 Wed

func dayOK(got *time.Time, want string) bool {
	if want == "" {
		return got == nil
	}
	return got != nil && got.Format("2006-01-02") == want
}

func TestAnalyzeEvalQuery_ChronologicalCues(t *testing.T) {
	t.Parallel()
	a := evalQueryTestAnchor()
	cases := []struct {
		name  string
		query string
		want  bool
	}{
		{"先后", "我搬家和入职的先后顺序", true},
		{"顺序", "按时间顺序列出我的体检记录", true},
		{"第几", "这是我第几次换工作", true},
		{"最早", "我最早住在哪个城市", true},
		{"最近第一次", "最近第一次复查是什么时候", true},
		{"英文 order", "list my doctors visits in order", true},
		{"英文 first time", "when was the first time I moved", true},
		{"普通查询不命中", "我喜欢什么水果", false},
		{"无线索", "summary of nothing at all", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			qa := AnalyzeEvalQuery(tc.query, a)
			if qa.Chronological != tc.want {
				t.Errorf("Chronological=%v, want %v", qa.Chronological, tc.want)
			}
		})
	}
}

func TestAnalyzeEvalQuery_AggregateCues(t *testing.T) {
	t.Parallel()
	a := evalQueryTestAnchor()
	cases := []struct {
		name  string
		query string
		want  bool
	}{
		{"总结", "总结一下我们聊了什么", true},
		{"一共", "我一共去过几次医院", true},
		{"总共", "总共搬过几次家", true},
		{"概括", "概括一下我的饮食偏好", true},
		{"英文 summary", "give me a summary of my trips", true},
		{"英文 overall", "overall what do I like", true},
		{"英文 how many times", "how many times did I change jobs", true},
		{"普通查询不命中", "我最喜欢的颜色", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			qa := AnalyzeEvalQuery(tc.query, a)
			if qa.Aggregate != tc.want {
				t.Errorf("Aggregate=%v, want %v", qa.Aggregate, tc.want)
			}
		})
	}
}

func TestAnalyzeEvalQuery_DateRanges(t *testing.T) {
	t.Parallel()
	a := evalQueryTestAnchor() // 2025-05-07 Wed
	cases := []struct {
		name      string
		query     string
		wantStart string
		wantEnd   string
	}{
		{"无日期", "我喜欢什么水果", "", ""},
		{"中文绝对日期单日", "我在2025年5月6日做了什么", "2025-05-06", "2025-05-06"},
		{"ISO 单日", "what happened on 2025-05-05", "2025-05-05", "2025-05-05"},
		{"US 单日", "anything on 05/04/2025", "2025-05-04", "2025-05-04"},
		{"相对日期单日", "昨天我去哪了", "2025-05-06", "2025-05-06"},
		{"之后区间起点", "2025年5月6日之后的事", "2025-05-06", ""},
		{"since 英文区间", "what did I do since yesterday", "2025-05-06", ""},
		{"以前区间终点", "上周三以前我去了哪里", "", "2025-04-30"},
		{"between 两端", "between 2025-05-01 and 2025-05-05", "2025-05-01", "2025-05-05"},
		{"从…到… 两端", "从2025年5月1日到2025年5月5日的事", "2025-05-01", "2025-05-05"},
		{"两日期无连接词取 min..max", "2025-05-05 和 2025-05-01 的事", "2025-05-01", "2025-05-05"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			qa := AnalyzeEvalQuery(tc.query, a)
			if !dayOK(qa.RangeStart, tc.wantStart) || !dayOK(qa.RangeEnd, tc.wantEnd) {
				t.Errorf("range=[%v,%v], want [%q,%q]", qa.RangeStart, qa.RangeEnd, tc.wantStart, tc.wantEnd)
			}
		})
	}
}
