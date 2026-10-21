package data

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMaskEvalEvidence_TableDriven(t *testing.T) {
	t.Parallel()
	content := "我的手机号是13812345678，身份证号是110101199001011234，银行卡号是6222021001116245。"
	cases := []struct {
		name         string
		content      string
		query        string
		wantContains []string
		wantAbsent   []string
	}{
		{
			name:    "neutral query masks all categories",
			content: content,
			query:   "介绍一下用户",
			wantContains: []string{
				"138****5678",
				"110101********1234",
				"6222********6245",
			},
			wantAbsent: []string{"13812345678", "110101199001011234", "6222021001116245"},
		},
		{
			name:    "asking phone unmasks phone only",
			content: content,
			query:   "用户的手机号是多少",
			wantContains: []string{
				"13812345678",
				"110101********1234",
				"6222********6245",
			},
			wantAbsent: []string{"110101199001011234", "6222021001116245"},
		},
		{
			name:    "asking id card unmasks id only",
			content: content,
			query:   "用户的身份证号",
			wantContains: []string{
				"110101199001011234",
				"138****5678",
				"6222********6245",
			},
			wantAbsent: []string{"13812345678", "6222021001116245"},
		},
		{
			name:         "asking card unmasks card only",
			content:      content,
			query:        "用户的银行卡号是多少",
			wantContains: []string{"6222021001116245", "138****5678"},
			wantAbsent:   []string{"13812345678"},
		},
		{
			name:         "13 digit card keeps 4+6stars+3",
			content:      "卡号1234567890123",
			query:        "介绍一下",
			wantContains: []string{"1234******123"},
			wantAbsent:   []string{"1234567890123"},
		},
		{
			name:         "email address preference never masked",
			content:      "我的邮箱是alice@example.com，住在杭州西湖区文三路100号，最喜欢蓝色",
			query:        "介绍一下",
			wantContains: []string{"alice@example.com", "杭州西湖区文三路100号", "最喜欢蓝色"},
		},
		{
			name:         "english ask words",
			content:      content,
			query:        "what is the id number",
			wantContains: []string{"110101199001011234"},
			wantAbsent:   []string{},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := MaskEvalEvidence(tc.content, tc.query)
			for _, w := range tc.wantContains {
				if !strings.Contains(got, w) {
					t.Errorf("masked content missing %q: %q", w, got)
				}
			}
			for _, w := range tc.wantAbsent {
				if strings.Contains(got, w) {
					t.Errorf("masked content leaked %q: %q", w, got)
				}
			}
		})
	}
}

func TestEvalMetadataJSON_Shape(t *testing.T) {
	t.Parallel()
	raw := evalMetadataJSON("s-1", 2, 3, []string{"2025-05-06"}, []evalPrevValue{{Statement: "旧句", ValidUntil: "2025-06-01T00:00:00Z"}})
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	if m["eval_session"] != "s-1" {
		t.Errorf("eval_session=%v", m["eval_session"])
	}
	if m["message_index"].(float64) != 2 || m["atom_index"].(float64) != 3 {
		t.Errorf("message_index/atom_index wrong: %v %v", m["message_index"], m["atom_index"])
	}
	dates, _ := m["event_dates"].([]any)
	if len(dates) != 1 || dates[0] != "2025-05-06" {
		t.Errorf("event_dates=%v", dates)
	}
	prevs, _ := m["previous_values"].([]any)
	if len(prevs) != 1 {
		t.Fatalf("previous_values=%v", prevs)
	}
	pv, _ := prevs[0].(map[string]any)
	if pv["statement"] != "旧句" || pv["valid_until"] == "" {
		t.Errorf("previous_values entry wrong: %v", pv)
	}

	empty := evalMetadataJSON("s-2", 0, 0, nil, nil)
	var m2 map[string]any
	if err := json.Unmarshal([]byte(empty), &m2); err != nil {
		t.Fatal(err)
	}
	if _, ok := m2["event_dates"]; ok {
		t.Errorf("event_dates should be omitted when empty: %v", m2)
	}
	if _, ok := m2["previous_values"]; ok {
		t.Errorf("previous_values should be omitted when empty: %v", m2)
	}
}

func TestEvalEvidenceDate_Priority(t *testing.T) {
	t.Parallel()
	meta := `{"event_dates":["2025-05-06"]}`
	if got := EvalEvidenceDate(meta, "2025-06-08T08:00:00Z", "2025-07-09T09:00:00Z"); got != "2025-05-06" {
		t.Errorf("event_dates[0] should win, got %q", got)
	}
	if got := EvalEvidenceDate("{}", "2025-06-08T08:00:00Z", "2025-07-09T09:00:00Z"); got != "2025-06-08" {
		t.Errorf("valid_from fallback wrong, got %q", got)
	}
	if got := EvalEvidenceDate("", "", "2025-07-09T09:00:00Z"); got != "2025-07-09" {
		t.Errorf("created_at fallback wrong, got %q", got)
	}
	if got := EvalEvidenceDate("", "", ""); got != "" {
		t.Errorf("empty inputs should yield empty, got %q", got)
	}
}

func TestEvalItemPrefix_Shapes(t *testing.T) {
	t.Parallel()
	if got := evalItemPrefix("2025-05-06", "s-1"); got != "[2025-05-06 · sess-s-1] " {
		t.Errorf("full prefix wrong: %q", got)
	}
	if got := evalItemPrefix("", "s-1"); got != "[sess-s-1] " {
		t.Errorf("session-only prefix wrong: %q", got)
	}
	if got := evalItemPrefix("2025-05-06", ""); got != "[2025-05-06] " {
		t.Errorf("day-only prefix wrong: %q", got)
	}
	if got := evalItemPrefix("", ""); got != "" {
		t.Errorf("empty prefix should be empty: %q", got)
	}
}

func TestHasEvalCorrectionCue_TableDriven(t *testing.T) {
	t.Parallel()
	cases := []struct {
		stmt string
		want bool
	}{
		{"我改主意了", true},
		{"其实我喜欢红色", true},
		{"不对，应该是周二", true},
		{"错了，会议是八点", true},
		{"不是蓝色而是红色", true},
		{"Actually I prefer tea", true},
		{"correction: the date is May 5", true},
		{"I am no longer using Gmail", true},
		{"in fact I moved last week", true},
		{"我住在杭州，喜欢龙井虾仁", false},
		{"I like coffee in the morning", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := HasEvalCorrectionCue(tc.stmt); got != tc.want {
			t.Errorf("HasEvalCorrectionCue(%q) = %v, want %v", tc.stmt, got, tc.want)
		}
	}
}
