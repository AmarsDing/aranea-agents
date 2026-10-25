package biz

import (
	"testing"
	"time"
)

func TestEvalSupersession_PastStateIsNotCurrentUpdate(t *testing.T) {
	for _, past := range []string{"user: 我以前住在北京", "user: I used to live in Berlin", "user: I previously worked at Alder"} {
		old := "user: 我住在杭州"
		if past[6] == 'I' {
			old = "user: I live in Rome"
		}
		if ShouldSupersedeEvalFact(ClassifyEvalFactKind(old), old, ClassifyEvalFactKind(past), past) {
			t.Errorf("past state changed the present: %q", past)
		}
	}
}

func TestEvalSupersession_IndependentSingleValueAttributes(t *testing.T) {
	for _, tc := range []struct {
		old, next string
		want      bool
	}{
		{"My email is a@example.com", "My email is b@example.com", true},
		{"My timezone is Europe/Rome", "My timezone is Asia/Shanghai", true},
		{"My preferred language is English", "My preferred language is Chinese", true},
		{"Alice's email is a@example.com", "Bob's email is b@example.com", false},
		{"My email is a@example.com", "My timezone is UTC", false},
	} {
		if got := ShouldSupersedeEvalFact(ClassifyEvalFactKind(tc.old), tc.old, ClassifyEvalFactKind(tc.next), tc.next); got != tc.want {
			t.Errorf("%q -> %q = %v, want %v", tc.old, tc.next, got, tc.want)
		}
	}
}

func TestResolveEvalDateSpans_CurrentPeriodsAndYearlessDates(t *testing.T) {
	a := time.Date(2024, 3, 20, 9, 0, 0, 0, time.UTC)
	for _, tc := range []struct{ text, start, end string }{
		{"this week", "2024-03-18", "2024-03-24"},
		{"this month", "2024-03-01", "2024-03-31"},
		{"本月", "2024-03-01", "2024-03-31"},
		{"2023年", "2023-01-01", "2023-12-31"},
		{"in 2023", "2023-01-01", "2023-12-31"},
		{"March 12", "2024-03-12", "2024-03-12"},
		{"3月12日", "2024-03-12", "2024-03-12"},
	} {
		spans := ResolveEvalDateSpans(tc.text, a)
		if len(spans) != 1 || spans[0].Start != tc.start || spans[0].End != tc.end {
			t.Errorf("%q = %v, want [%s,%s]", tc.text, spans, tc.start, tc.end)
		}
	}
}

func TestResolveEvalDateSpans_InvalidDateDoesNotBecomeCoarserPeriod(t *testing.T) {
	for _, text := range []string{"in 2024-02-30", "2024年2月30日", "February 30, 2024"} {
		if spans := ResolveEvalDateSpans(text, time.Date(2025, 3, 20, 0, 0, 0, 0, time.UTC)); len(spans) != 0 {
			t.Errorf("invalid date became valid: %q = %v", text, spans)
		}
	}
}

func TestAnalyzeEvalQuery_LearnedFunctionAndFullHistory(t *testing.T) {
	if qa := AnalyzeEvalQuery("What is f(8)?", time.Now()); !qa.Procedural {
		t.Fatal("one-letter learned function not recognized")
	}
	if qa := AnalyzeEvalQuery("Summarize all conversations", time.Now()); !qa.Aggregate || !qa.Historical {
		t.Fatal("global history summary lost past states")
	}
	if got := EvalRuleAnchors("if (ready) calculate F (8)"); len(got) != 1 || got[0] != "f(" {
		t.Fatalf("rule anchors=%v", got)
	}
}
