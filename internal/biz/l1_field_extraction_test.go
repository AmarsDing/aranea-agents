package biz

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

// --- TestExtractStructuredEpisode ---

func TestExtractStructuredEpisode(t *testing.T) {
	tests := []struct {
		name      string
		snapshot  []byte
		wantTitle string
		wantGoal  string
		wantOSum  string
		wantDecN  int
		wantArtN  int
		wantImp   float64
		wantConf  float64
		wantKind  string
	}{
		{
			name:      "empty JSON returns defaults",
			snapshot:  []byte(``),
			wantTitle: "",
			wantGoal:  "",
			wantOSum:  "",
			wantDecN:  0,
			wantArtN:  0,
			wantImp:   0.5,
			wantConf:  0.6,
			wantKind:  "l1_archive_structured",
		},
		{
			name:      "invalid JSON returns defaults",
			snapshot:  []byte(`{invalid`),
			wantTitle: "",
			wantGoal:  "",
			wantOSum:  "",
			wantDecN:  0,
			wantArtN:  0,
			wantImp:   0.5,
			wantConf:  0.6,
			wantKind:  "l1_archive_structured",
		},
		{
			name: "valid snapshot with completed status",
			snapshot: mustMarshalTest(map[string]any{
				"task": map[string]any{
					"task_title": "Build API",
					"task_goal":  "Create REST endpoints",
					"status":     "completed",
				},
				"fields": []any{},
			}),
			wantTitle: "Build API",
			wantGoal:  "Create REST endpoints",
			wantOSum:  "任务已完成",
			wantDecN:  0,
			wantArtN:  0,
			wantImp:   0.5,
			wantConf:  0.6,
			wantKind:  "l1_archive_structured",
		},
		{
			name: "status mapping cancelled",
			snapshot: mustMarshalTest(map[string]any{
				"task":   map[string]any{"status": "cancelled"},
				"fields": []any{},
			}),
			wantOSum: "任务被取消（空闲超时）",
			wantImp:  0.5,
			wantConf: 0.6,
			wantKind: "l1_archive_structured",
		},
		{
			name: "status mapping failed",
			snapshot: mustMarshalTest(map[string]any{
				"task":   map[string]any{"status": "failed"},
				"fields": []any{},
			}),
			wantOSum: "任务失败",
			wantImp:  0.5,
			wantConf: 0.6,
			wantKind: "l1_archive_structured",
		},
		{
			name: "status mapping timeout",
			snapshot: mustMarshalTest(map[string]any{
				"task":   map[string]any{"status": "timeout"},
				"fields": []any{},
			}),
			wantOSum: "任务超时",
			wantImp:  0.5,
			wantConf: 0.6,
			wantKind: "l1_archive_structured",
		},
		{
			name: "status mapping unknown passes through",
			snapshot: mustMarshalTest(map[string]any{
				"task":   map[string]any{"status": "unknown"},
				"fields": []any{},
			}),
			wantOSum: "unknown",
			wantImp:  0.5,
			wantConf: 0.6,
			wantKind: "l1_archive_structured",
		},
		{
			name: "last_assistant_message appended to outcome",
			snapshot: mustMarshalTest(map[string]any{
				"task": map[string]any{
					"status":                 "completed",
					"last_assistant_message": "All done!",
				},
				"fields": []any{},
			}),
			wantOSum:  "任务已完成",
			wantTitle: "",
			wantGoal:  "",
			wantDecN:  0,
			wantArtN:  0,
			wantImp:   0.5,
			wantConf:  0.6,
			wantKind:  "l1_archive_structured",
		},
		{
			name: "last_assistant_message truncated to 200 chars",
			snapshot: mustMarshalTest(map[string]any{
				"task": map[string]any{
					"status":                 "completed",
					"last_assistant_message": string(make([]byte, 300)),
				},
				"fields": []any{},
			}),
			wantOSum: "任务已完成",
			wantImp:  0.5,
			wantConf: 0.6,
			wantKind: "l1_archive_structured",
		},
		{
			name: "fields with decision kind populate KeyDecisions",
			snapshot: mustMarshalTest(map[string]any{
				"task": map[string]any{"status": "completed"},
				"fields": []any{
					map[string]any{"field_kind": "decision", "field_path": "/dec1", "value_text": "chose A"},
				},
			}),
			wantOSum: "任务已完成",
			wantDecN: 1,
			wantArtN: 0,
			wantImp:  0.5,
			wantConf: 0.6,
			wantKind: "l1_archive_structured",
		},
		{
			name: "fields with artifact/reference kind populate KeyArtifacts",
			snapshot: mustMarshalTest(map[string]any{
				"task": map[string]any{"status": "completed"},
				"fields": []any{
					map[string]any{"field_kind": "artifact", "field_path": "/art1", "value_text": "file.go"},
					map[string]any{"field_kind": "reference", "field_path": "/ref1", "value_text": "doc.md"},
				},
			}),
			wantOSum: "任务已完成",
			wantDecN: 0,
			wantArtN: 2,
			wantImp:  0.5,
			wantConf: 0.6,
			wantKind: "l1_archive_structured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractStructuredEpisode(tt.snapshot)
			if got.Title != tt.wantTitle {
				t.Errorf("Title = %q, want %q", got.Title, tt.wantTitle)
			}
			if got.Goal != tt.wantGoal {
				t.Errorf("Goal = %q, want %q", got.Goal, tt.wantGoal)
			}
			if got.OutcomeSummary != tt.wantOSum {
				t.Errorf("OutcomeSummary = %q, want %q", got.OutcomeSummary, tt.wantOSum)
			}
			if len(got.KeyDecisions) != tt.wantDecN {
				t.Errorf("len(KeyDecisions) = %d, want %d", len(got.KeyDecisions), tt.wantDecN)
			}
			if len(got.KeyArtifacts) != tt.wantArtN {
				t.Errorf("len(KeyArtifacts) = %d, want %d", len(got.KeyArtifacts), tt.wantArtN)
			}
			if got.Importance != tt.wantImp {
				t.Errorf("Importance = %v, want %v", got.Importance, tt.wantImp)
			}
			if got.Confidence != tt.wantConf {
				t.Errorf("Confidence = %v, want %v", got.Confidence, tt.wantConf)
			}
			if got.EpisodeKind != tt.wantKind {
				t.Errorf("EpisodeKind = %q, want %q", got.EpisodeKind, tt.wantKind)
			}
			// Verify non-nil empty slices
			if got.KeyDecisions == nil {
				t.Error("KeyDecisions should not be nil")
			}
			if got.KeyArtifacts == nil {
				t.Error("KeyArtifacts should not be nil")
			}
		})
	}

	// Subtest: verify outcome includes last_assistant_message
	t.Run("outcome includes last_assistant_message", func(t *testing.T) {
		snap := mustMarshalTest(map[string]any{
			"task": map[string]any{
				"status":                 "completed",
				"last_assistant_message": "All done!",
			},
			"fields": []any{},
		})
		got := ExtractStructuredEpisode(snap)
		if got.Outcome != "任务已完成All done!" {
			t.Errorf("Outcome = %q, want %q", got.Outcome, "任务已完成All done!")
		}
	})

	// Subtest: long message truncated
	t.Run("long last_assistant_message truncated to 200", func(t *testing.T) {
		// Use 300 'A' chars to test rune-aware truncation
		longMsg := strings.Repeat("A", 300)
		snap := mustMarshalTest(map[string]any{
			"task": map[string]any{
				"status":                 "completed",
				"last_assistant_message": longMsg,
			},
			"fields": []any{},
		})
		got := ExtractStructuredEpisode(snap)
		// outcome = outcomeSummary + truncatedMsg (200 runes + "…")
		prefix := "任务已完成"
		expectedLen := len(prefix) + 200 + len("…")
		if len(got.Outcome) != expectedLen {
			t.Errorf("Outcome length = %d, want %d", len(got.Outcome), expectedLen)
		}
	})
}

// --- TestExtractKeyDecisions ---

func TestExtractKeyDecisions(t *testing.T) {
	t.Run("Layer0 field_kind=decision", func(t *testing.T) {
		fields := []map[string]any{
			{"field_kind": "decision", "field_path": "/d1", "value_text": "chose A"},
			{"field_kind": "info", "field_path": "/i1", "value_text": "some info"},
			{"field_kind": "decision", "field_path": "/d2", "value_text": "chose B"},
		}
		got := ExtractKeyDecisions(fields)
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2", len(got))
		}
		if got[0].Path != "/d1" || got[1].Path != "/d2" {
			t.Errorf("paths = %q, %q, want /d1, /d2", got[0].Path, got[1].Path)
		}
	})

	t.Run("Layer1 path patterns when no decision kind", func(t *testing.T) {
		fields := []map[string]any{
			{"field_kind": "info", "field_path": "/decision/step1", "value_text": "dec text"},
			{"field_kind": "info", "field_path": "/choice/pick", "value_text": "choice text"},
			{"field_kind": "info", "field_path": "/approach/method", "value_text": "approach text"},
			{"field_kind": "info", "field_path": "/option/a", "value_text": "option text"},
			{"field_kind": "info", "field_path": "/other/thing", "value_text": "other"},
		}
		got := ExtractKeyDecisions(fields)
		if len(got) != 4 {
			t.Fatalf("len = %d, want 4", len(got))
		}
	})

	t.Run("Layer2 pin_to_prompt and visibility=prompt top 3", func(t *testing.T) {
		fields := []map[string]any{
			{"field_path": "/a", "value_text": "va", "pin_to_prompt": true, "visibility": "prompt", "updated_at": "2024-01-01"},
			{"field_path": "/b", "value_text": "vb", "pin_to_prompt": true, "visibility": "prompt", "updated_at": "2024-01-03"},
			{"field_path": "/c", "value_text": "vc", "pin_to_prompt": true, "visibility": "prompt", "updated_at": "2024-01-02"},
			{"field_path": "/d", "value_text": "vd", "pin_to_prompt": true, "visibility": "prompt", "updated_at": "2024-01-04"},
		}
		got := ExtractKeyDecisions(fields)
		if len(got) != 3 {
			t.Fatalf("len = %d, want 3", len(got))
		}
		// Should be sorted by updated_at desc, so top 3 are /d, /b, /c
		if got[0].Path != "/d" {
			t.Errorf("first = %q, want /d", got[0].Path)
		}
	})

	t.Run("Layer3 recent visibility=prompt top 5", func(t *testing.T) {
		fields := []map[string]any{
			{"field_path": "/a", "value_text": "va", "visibility": "prompt", "updated_at": "2024-01-01"},
			{"field_path": "/b", "value_text": "vb", "visibility": "prompt", "updated_at": "2024-01-06"},
			{"field_path": "/c", "value_text": "vc", "visibility": "prompt", "updated_at": "2024-01-03"},
			{"field_path": "/d", "value_text": "vd", "visibility": "prompt", "updated_at": "2024-01-05"},
			{"field_path": "/e", "value_text": "ve", "visibility": "prompt", "updated_at": "2024-01-04"},
			{"field_path": "/f", "value_text": "vf", "visibility": "prompt", "updated_at": "2024-01-02"},
			{"field_path": "/g", "value_text": "vg", "visibility": "hidden", "updated_at": "2024-01-07"},
		}
		got := ExtractKeyDecisions(fields)
		if len(got) != 5 {
			t.Fatalf("len = %d, want 5", len(got))
		}
		// Top by updated_at desc: /b, /d, /e, /c, /f
		if got[0].Path != "/b" {
			t.Errorf("first = %q, want /b", got[0].Path)
		}
	})

	t.Run("empty fields returns empty slice not nil", func(t *testing.T) {
		got := ExtractKeyDecisions(nil)
		if got == nil {
			t.Error("got nil, want empty slice")
		}
		if len(got) != 0 {
			t.Errorf("len = %d, want 0", len(got))
		}
	})
}

// --- TestExtractKeyArtifacts ---

func TestExtractKeyArtifacts(t *testing.T) {
	t.Run("Layer0 artifact and reference kinds", func(t *testing.T) {
		fields := []map[string]any{
			{"field_kind": "artifact", "field_path": "/a1", "value_text": "file.go"},
			{"field_kind": "reference", "field_path": "/r1", "value_text": "doc.md"},
			{"field_kind": "info", "field_path": "/i1", "value_text": "note"},
		}
		got := ExtractKeyArtifacts(fields)
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2", len(got))
		}
		if got[0].Kind != "artifact" || got[1].Kind != "reference" {
			t.Errorf("kinds = %q, %q, want artifact, reference", got[0].Kind, got[1].Kind)
		}
	})

	t.Run("Layer1 path patterns with reference kind (direct)", func(t *testing.T) {
		// Layer1 is unreachable via ExtractKeyArtifacts when Layer0 matches reference fields,
		// so test extractArtifactsByPathAndKind directly.
		fields := []map[string]any{
			{"field_kind": "reference", "field_path": "/file/output", "value_text": "out.txt"},
			{"field_kind": "reference", "field_path": "/path/to/config", "value_text": "cfg.yaml"},
			{"field_kind": "reference", "field_path": "/no/match", "value_text": "skip"},
			{"field_kind": "info", "field_path": "/file/here", "value_text": "not-ref"},
		}
		got := extractArtifactsByPathAndKind(fields, []string{"file", "path", "config", "output"})
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2", len(got))
		}
		for _, a := range got {
			if a.Kind != "reference" {
				t.Errorf("kind = %q, want reference", a.Kind)
			}
		}
	})

	t.Run("Layer2 any reference kind", func(t *testing.T) {
		fields := []map[string]any{
			{"field_kind": "reference", "field_path": "/x", "value_text": "ref1"},
			{"field_kind": "reference", "field_path": "/y", "value_text": "ref2"},
			{"field_kind": "info", "field_path": "/z", "value_text": "info1"},
		}
		got := ExtractKeyArtifacts(fields)
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2", len(got))
		}
	})

	t.Run("empty fields returns empty slice", func(t *testing.T) {
		got := ExtractKeyArtifacts(nil)
		if got == nil {
			t.Error("got nil, want empty slice")
		}
		if len(got) != 0 {
			t.Errorf("len = %d, want 0", len(got))
		}
	})
}

// --- TestMapStatusToOutcome ---

func TestMapStatusToOutcome(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{"completed", "任务已完成"},
		{"cancelled", "任务被取消（空闲超时）"},
		{"failed", "任务失败"},
		{"timeout", "任务超时"},
		{"unknown", "unknown"},
		{"", ""},
		{"running", "running"},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			got := mapStatusToOutcome(tt.status)
			if got != tt.want {
				t.Errorf("mapStatusToOutcome(%q) = %q, want %q", tt.status, got, tt.want)
			}
		})
	}
}

// --- TestShouldTriggerPathB ---

func TestShouldTriggerPathB(t *testing.T) {
	tests := []struct {
		name string
		sig  EpisodeSignals
		want bool
	}{
		{
			name: "importance >= 0.7 triggers",
			sig:  EpisodeSignals{Importance: 0.7},
			want: true,
		},
		{
			name: "importance < 0.7 does not trigger alone",
			sig:  EpisodeSignals{Importance: 0.69},
			want: false,
		},
		{
			name: "critic_score >= 0.8 triggers",
			sig:  EpisodeSignals{CriticScore: 0.8},
			want: true,
		},
		{
			name: "critic_score < 0.8 does not trigger alone",
			sig:  EpisodeSignals{CriticScore: 0.79},
			want: false,
		},
		{
			name: "tool_call_count >= 20 triggers",
			sig:  EpisodeSignals{ToolCallCount: 20},
			want: true,
		},
		{
			name: "tool_call_count < 20 does not trigger alone",
			sig:  EpisodeSignals{ToolCallCount: 19},
			want: false,
		},
		{
			name: "duration_ms >= 300000 triggers",
			sig:  EpisodeSignals{DurationMs: 300000},
			want: true,
		},
		{
			name: "duration_ms < 300000 does not trigger alone",
			sig:  EpisodeSignals{DurationMs: 299999},
			want: false,
		},
		{
			name: "user_mark=star triggers",
			sig:  EpisodeSignals{UserMark: "star"},
			want: true,
		},
		{
			name: "user_mark=consolidate triggers",
			sig:  EpisodeSignals{UserMark: "consolidate"},
			want: true,
		},
		{
			name: "user_mark=other does not trigger",
			sig:  EpisodeSignals{UserMark: "other"},
			want: false,
		},
		{
			name: "all below thresholds does not trigger",
			sig: EpisodeSignals{
				Importance:    0.1,
				CriticScore:   0.2,
				ToolCallCount: 5,
				DurationMs:    10000,
				UserMark:      "",
			},
			want: false,
		},
		{
			name: "multiple conditions met triggers",
			sig: EpisodeSignals{
				Importance:    0.8,
				CriticScore:   0.9,
				ToolCallCount: 25,
				DurationMs:    400000,
				UserMark:      "star",
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShouldTriggerPathB(tt.sig)
			if got != tt.want {
				t.Errorf("ShouldTriggerPathB(%+v) = %v, want %v", tt.sig, got, tt.want)
			}
		})
	}
}

// --- TestEpisodeScore ---

func TestEpisodeScore(t *testing.T) {
	t.Run("with critic_score present", func(t *testing.T) {
		sig := EpisodeSignals{
			Importance:    1.0,
			CriticScore:   0.8,
			ToolCallCount: 20,
			DurationMs:    300000,
			UserMark:      "star",
		}
		got := EpisodeScore(sig)
		// 0.30*1 + 0.25*min(0.8/0.8,1) + 0.15*min(20/20,1) + 0.15*min(300000/300000,1) + 0.15*1
		// = 0.30 + 0.25 + 0.15 + 0.15 + 0.15 = 1.0
		if math.Abs(got-1.0) > 1e-9 {
			t.Errorf("EpisodeScore = %v, want 1.0", got)
		}
	})

	t.Run("without critic_score (missing=-1)", func(t *testing.T) {
		sig := EpisodeSignals{
			Importance:    1.0,
			CriticScore:   -1,
			ToolCallCount: 20,
			DurationMs:    300000,
			UserMark:      "star",
		}
		got := EpisodeScore(sig)
		// 0.40*1 + 0.20*min(20/20,1) + 0.20*min(300000/300000,1) + 0.20*1
		// = 0.40 + 0.20 + 0.20 + 0.20 = 1.0
		if math.Abs(got-1.0) > 1e-9 {
			t.Errorf("EpisodeScore = %v, want 1.0", got)
		}
	})

	t.Run("all zeros score 0", func(t *testing.T) {
		sig := EpisodeSignals{
			Importance:    0,
			CriticScore:   -1,
			ToolCallCount: 0,
			DurationMs:    0,
			UserMark:      "",
		}
		got := EpisodeScore(sig)
		if got != 0 {
			t.Errorf("EpisodeScore = %v, want 0", got)
		}
	})

	t.Run("all zeros with critic_score=0", func(t *testing.T) {
		sig := EpisodeSignals{
			Importance:    0,
			CriticScore:   0,
			ToolCallCount: 0,
			DurationMs:    0,
			UserMark:      "",
		}
		got := EpisodeScore(sig)
		if got != 0 {
			t.Errorf("EpisodeScore = %v, want 0", got)
		}
	})

	t.Run("weight sum = 1.0 with critic", func(t *testing.T) {
		// 0.30 + 0.25 + 0.15 + 0.15 + 0.15 = 1.0
		sum := 0.30 + 0.25 + 0.15 + 0.15 + 0.15
		if math.Abs(sum-1.0) > 1e-9 {
			t.Errorf("weight sum with critic = %v, want 1.0", sum)
		}
	})

	t.Run("weight sum = 1.0 without critic", func(t *testing.T) {
		// 0.40 + 0.20 + 0.20 + 0.20 = 1.0
		sum := 0.40 + 0.20 + 0.20 + 0.20
		if math.Abs(sum-1.0) > 1e-9 {
			t.Errorf("weight sum without critic = %v, want 1.0", sum)
		}
	})

	t.Run("partial values with critic", func(t *testing.T) {
		sig := EpisodeSignals{
			Importance:    0.5,
			CriticScore:   0.4,
			ToolCallCount: 10,
			DurationMs:    150000,
			UserMark:      "",
		}
		got := EpisodeScore(sig)
		// 0.30*0.5 + 0.25*min(0.4/0.8,1) + 0.15*min(10/20,1) + 0.15*min(150000/300000,1) + 0.15*0
		// = 0.15 + 0.25*0.5 + 0.15*0.5 + 0.15*0.5 + 0
		// = 0.15 + 0.125 + 0.075 + 0.075 + 0 = 0.425
		want := 0.30*0.5 + 0.25*math.Min(0.4/0.8, 1) + 0.15*math.Min(10.0/20, 1) + 0.15*math.Min(150000.0/300000, 1) + 0.15*0
		if math.Abs(got-want) > 1e-9 {
			t.Errorf("EpisodeScore = %v, want %v", got, want)
		}
	})

	t.Run("partial values without critic", func(t *testing.T) {
		sig := EpisodeSignals{
			Importance:    0.5,
			CriticScore:   -1,
			ToolCallCount: 10,
			DurationMs:    150000,
			UserMark:      "star",
		}
		got := EpisodeScore(sig)
		// 0.40*0.5 + 0.20*min(10/20,1) + 0.20*min(150000/300000,1) + 0.20*1
		// = 0.20 + 0.10 + 0.10 + 0.20 = 0.60
		want := 0.40*0.5 + 0.20*math.Min(10.0/20, 1) + 0.20*math.Min(150000.0/300000, 1) + 0.20*1
		if math.Abs(got-want) > 1e-9 {
			t.Errorf("EpisodeScore = %v, want %v", got, want)
		}
	})

	t.Run("critic_score capped at 1.0", func(t *testing.T) {
		sig := EpisodeSignals{
			Importance:    1.0,
			CriticScore:   1.6, // > 0.8, min(1.6/0.8,1) = 1.0
			ToolCallCount: 20,
			DurationMs:    300000,
			UserMark:      "star",
		}
		got := EpisodeScore(sig)
		if math.Abs(got-1.0) > 1e-9 {
			t.Errorf("EpisodeScore = %v, want 1.0", got)
		}
	})
}

// --- TestExtractEpisodeSignals ---

func TestExtractEpisodeSignals(t *testing.T) {
	t.Run("empty JSON returns defaults", func(t *testing.T) {
		got := extractEpisodeSignals([]byte(`{}`), 0.5)
		if got.Importance != 0.5 {
			t.Errorf("Importance = %v, want 0.5", got.Importance)
		}
		if got.CriticScore != -1 {
			t.Errorf("CriticScore = %v, want -1", got.CriticScore)
		}
		if got.ToolCallCount != 0 {
			t.Errorf("ToolCallCount = %d, want 0", got.ToolCallCount)
		}
		if got.DurationMs != 0 {
			t.Errorf("DurationMs = %d, want 0", got.DurationMs)
		}
		if got.UserMark != "" {
			t.Errorf("UserMark = %q, want empty", got.UserMark)
		}
	})

	t.Run("invalid JSON returns defaults", func(t *testing.T) {
		got := extractEpisodeSignals([]byte(`not json`), 0.3)
		if got.Importance != 0.3 {
			t.Errorf("Importance = %v, want 0.3", got.Importance)
		}
		if got.CriticScore != -1 {
			t.Errorf("CriticScore = %v, want -1", got.CriticScore)
		}
	})

	t.Run("tool_call_count parsed", func(t *testing.T) {
		raw := mustMarshalTest(map[string]any{"tool_call_count": float64(15)})
		got := extractEpisodeSignals(raw, 0)
		if got.ToolCallCount != 15 {
			t.Errorf("ToolCallCount = %d, want 15", got.ToolCallCount)
		}
	})

	t.Run("duration_ms parsed", func(t *testing.T) {
		raw := mustMarshalTest(map[string]any{"duration_ms": float64(120000)})
		got := extractEpisodeSignals(raw, 0)
		if got.DurationMs != 120000 {
			t.Errorf("DurationMs = %d, want 120000", got.DurationMs)
		}
	})

	t.Run("critic_score parsed", func(t *testing.T) {
		raw := mustMarshalTest(map[string]any{"critic_score": 0.85})
		got := extractEpisodeSignals(raw, 0)
		if got.CriticScore != 0.85 {
			t.Errorf("CriticScore = %v, want 0.85", got.CriticScore)
		}
	})

	t.Run("user_mark parsed", func(t *testing.T) {
		raw := mustMarshalTest(map[string]any{"user_mark": "star"})
		got := extractEpisodeSignals(raw, 0)
		if got.UserMark != "star" {
			t.Errorf("UserMark = %q, want star", got.UserMark)
		}
	})

	t.Run("all fields populated", func(t *testing.T) {
		raw := mustMarshalTest(map[string]any{
			"critic_score":    0.9,
			"tool_call_count": float64(25),
			"duration_ms":     float64(400000),
			"user_mark":       "consolidate",
		})
		got := extractEpisodeSignals(raw, 0.8)
		if got.Importance != 0.8 {
			t.Errorf("Importance = %v, want 0.8", got.Importance)
		}
		if got.CriticScore != 0.9 {
			t.Errorf("CriticScore = %v, want 0.9", got.CriticScore)
		}
		if got.ToolCallCount != 25 {
			t.Errorf("ToolCallCount = %d, want 25", got.ToolCallCount)
		}
		if got.DurationMs != 400000 {
			t.Errorf("DurationMs = %d, want 400000", got.DurationMs)
		}
		if got.UserMark != "consolidate" {
			t.Errorf("UserMark = %q, want consolidate", got.UserMark)
		}
	})
}

// --- helper ---

func mustMarshalTest(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// --- TestExtractStructuredEpisodeFromMessages ---

func TestExtractStructuredEpisodeFromMessages(t *testing.T) {
	t.Run("empty messages returns defaults", func(t *testing.T) {
		got := ExtractStructuredEpisodeFromMessages(nil)
		if got.EpisodeKind != "auto_memory_structured" {
			t.Errorf("EpisodeKind = %q, want auto_memory_structured", got.EpisodeKind)
		}
		if got.Importance != 0.5 {
			t.Errorf("Importance = %v, want 0.5", got.Importance)
		}
	})

	t.Run("extracts title from first user message", func(t *testing.T) {
		msgs := []ConsolidateMessage{
			{Role: "user", Content: "Help me build an API"},
			{Role: "assistant", Content: "Sure, I'll help"},
		}
		got := ExtractStructuredEpisodeFromMessages(msgs)
		if got.Title != "Help me build an API" {
			t.Errorf("Title = %q, want 'Help me build an API'", got.Title)
		}
		if got.Goal != "Help me build an API" {
			t.Errorf("Goal = %q, want 'Help me build an API'", got.Goal)
		}
	})

	t.Run("extracts outcome from last assistant message", func(t *testing.T) {
		msgs := []ConsolidateMessage{
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi there"},
			{Role: "user", Content: "Build API"},
			{Role: "assistant", Content: "API is ready"},
		}
		got := ExtractStructuredEpisodeFromMessages(msgs)
		if got.OutcomeSummary != "API is ready" {
			t.Errorf("OutcomeSummary = %q, want 'API is ready'", got.OutcomeSummary)
		}
	})

	t.Run("falls back to first message if no user message", func(t *testing.T) {
		msgs := []ConsolidateMessage{
			{Role: "assistant", Content: "Hello world"},
		}
		got := ExtractStructuredEpisodeFromMessages(msgs)
		if got.Title != "Hello world" {
			t.Errorf("Title = %q, want 'Hello world'", got.Title)
		}
	})
}

func TestTruncateRunes(t *testing.T) {
	tests := []struct {
		input string
		max   int
		want  string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello…"},
		{"你好世界", 2, "你好…"},
		{"", 5, ""},
		{"abc", 0, "abc"},
	}
	for _, tt := range tests {
		got := truncateRunes(tt.input, tt.max)
		if got != tt.want {
			t.Errorf("truncateRunes(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.want)
		}
	}
}
