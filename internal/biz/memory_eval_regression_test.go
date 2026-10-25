package biz

import (
	"reflect"
	"testing"
	"time"
)

func TestSplitEvalStatements_LosslessBoundaries(t *testing.T) {
	for _, tc := range []struct {
		text string
		want []string
	}{
		{"Mira bought tea. Noah bought coffee. Leo bought milk.", []string{"Mira bought tea.", "Noah bought coffee.", "Leo bought milk."}},
		{"Mira bought tea. 她选了绿茶。Noah left.", []string{"Mira bought tea. 她选了绿茶。", "Noah left."}},
		{"A=1\nB=2\nC=3", []string{"A=1", "B=2", "C=3"}},
	} {
		t.Run(tc.text, func(t *testing.T) {
			if got := SplitEvalStatements(tc.text, 8); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %q; want %q", got, tc.want)
			}
		})
	}
}

func TestEvalSupersession_RequiresSameSubjectAndAttribute(t *testing.T) {
	for _, tc := range []struct {
		old, next string
		want      bool
	}{
		{"user: My favorite color is blue", "user: My favorite color is red", true},
		{"user: Alice lives in Rome", "user: Bob lives in Oslo", false},
		{"user: My friend Alice lives in Rome", "user: My friend Bob lives in Oslo", false},
		{"user: 我的朋友小李住在北京", "user: 我的朋友小陈住在上海", false},
		{"user: My favorite type of music is jazz", "user: My favorite type of music changed to folk", true},
		{"user: I live in Rome", "assistant: I live in Oslo", false},
		{"user: My name is Mira", "user: My employee id is A17", false},
		{"user: I like hiking", "user: I now like tea", false},
		{"user: 我喜欢咖啡", "user: 我现在不喜欢咖啡了", true},
		{"user: I dislike coffee", "user: I now like coffee", true},
		{"user: 我叫小李", "user: 我的工号是5271", false},
		{"user: 我住在杭州", "user: 我现在搬到上海了", true},
		{"user: I live in Rome", "user: If I actually moved to Oslo then I would sell my home", false},
		{"user: 我住在杭州", "user: 我打算搬到上海", false},
	} {
		t.Run(tc.next, func(t *testing.T) {
			if got := ShouldSupersedeEvalFact(ClassifyEvalFactKind(tc.old), tc.old, ClassifyEvalFactKind(tc.next), tc.next); got != tc.want {
				t.Fatalf("supersede %q by %q = %v; want %v", tc.old, tc.next, got, tc.want)
			}
		})
	}
}

func TestEvalCorrectionTargetsFact_ReferenceIsNotContradiction(t *testing.T) {
	for _, tc := range []struct {
		old, next string
		want      bool
	}{
		{"user: My favorite color is blue", "user: Actually, I hate blue now", true},
		{"user: I live in Rome", "user: Actually, I hate Rome", false},
		{"user: My favorite coffee brand is Mornings", "user: Actually, I hate coffee", false},
		{"user: 我最喜欢的颜色是蓝色", "user: 其实我不喜欢蓝色了", true},
	} {
		if got := EvalCorrectionTargetsFact(tc.old, tc.next); got != tc.want {
			t.Errorf("%q -> %q = %v, want %v", tc.old, tc.next, got, tc.want)
		}
	}
}

func TestAnalyzeEvalQuery_CalendarAndCueBoundaries(t *testing.T) {
	anchor := time.Date(2024, 3, 20, 9, 0, 0, 0, time.UTC)
	for _, tc := range []struct{ query, start, end string }{
		{"What happened last month?", "2024-02-01", "2024-02-29"},
		{"上周做了什么", "2024-03-11", "2024-03-17"},
		{"What happened on 2024-02-30?", "", ""},
	} {
		qa := AnalyzeEvalQuery(tc.query, anchor)
		if !dayOK(qa.RangeStart, tc.start) || !dayOK(qa.RangeEnd, tc.end) {
			t.Errorf("%q: got [%v,%v], want [%s,%s]", tc.query, qa.RangeStart, qa.RangeEnd, tc.start, tc.end)
		}
	}
	if AnalyzeEvalQuery("What is my border color?", anchor).Chronological {
		t.Fatal("order matched inside border")
	}
}

func TestResolveEvalDateSpans_PrecisionAndUnits(t *testing.T) {
	a := time.Date(2024, 3, 20, 9, 0, 0, 0, time.UTC)
	for _, tc := range []struct{ text, start, end string }{
		{"十二天前", "2024-03-08", "2024-03-08"},
		{"in 2 weeks", "2024-04-03", "2024-04-03"},
		{"2 months ago", "2024-01-20", "2024-01-20"},
		{"2024年2月", "2024-02-01", "2024-02-29"},
		{"February 2024", "2024-02-01", "2024-02-29"},
		{"8 May, 2023", "2023-05-08", "2023-05-08"},
	} {
		spans := ResolveEvalDateSpans(tc.text, a)
		if len(spans) != 1 || spans[0].Start != tc.start || spans[0].End != tc.end {
			t.Errorf("%q=%v, want [%s,%s]", tc.text, spans, tc.start, tc.end)
		}
	}
}
