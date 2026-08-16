package compress

import (
	"strings"
	"testing"
)

// --- 压缩产物双段化（v4）：叙事摘要 + 结构化任务状态块 ---

func TestPromptVersion_V4(t *testing.T) {
	if PromptVersion != "v4" {
		t.Fatalf("got %q want %q", PromptVersion, "v4")
	}
}

func TestDefaultSystemPrompt_InstructsTaskStateBlock(t *testing.T) {
	// v4 契约：Section 9 之后必须输出一个 ```json 任务状态块。
	if !strings.Contains(DefaultSystemPrompt, "task_state") {
		t.Fatal("v4 prompt must instruct the task_state json block")
	}
	if !strings.Contains(DefaultSystemPrompt, `"done"`) || !strings.Contains(DefaultSystemPrompt, `"next"`) {
		t.Fatal("v4 prompt must document done/next keys")
	}
}

func TestExtractTaskState(t *testing.T) {
	narrative := "## 1. User Intent & Goals\nfix vpn\n\n## 9. Current Work State\nmid-way"

	tests := []struct {
		name           string
		in             string
		wantStripped   string
		wantStatus     string
		wantNext       string
		wantDone       []string
		wantBlockers   []string
		wantNilState   bool
		wantUnchanged  bool
	}{
		{
			name: "valid trailing block",
			in: narrative + "\n\n```json\n{\"status\":\"取证完成\",\"done\":[\"确认告警\",\"定位R2\"],\"next\":\"执行清除\",\"blockers\":[\"等待审批\"]}\n```\n",
			wantStripped: narrative,
			wantStatus:   "取证完成",
			wantNext:     "执行清除",
			wantDone:     []string{"确认告警", "定位R2"},
			wantBlockers: []string{"等待审批"},
		},
		{
			name:         "uppercase JSON fence recognized",
			in:           narrative + "\n\n```JSON\n{\"status\":\"进行中\",\"next\":\"继续\"}\n```",
			wantStripped: narrative,
			wantStatus:   "进行中",
			wantNext:     "继续",
		},
		{
			name:          "no block",
			in:            narrative,
			wantNilState:  true,
			wantUnchanged: true,
		},
		{
			name:          "invalid json block kept intact",
			in:            narrative + "\n\n```json\n{not-json}\n```",
			wantNilState:  true,
			wantUnchanged: true,
		},
		{
			name:          "empty object stripped but nil state",
			in:            narrative + "\n\n```json\n{}\n```",
			wantStripped:  narrative,
			wantNilState:  true,
			wantUnchanged: false,
		},
		{
			name: "block with trailing whitespace only",
			in:   narrative + "\n```json\n{\"next\":\"继续\"}\n```\n\n  \n",
			wantStripped: narrative,
			wantNext:     "继续",
		},
		{
			name:          "non-task json block untouched",
			in:            narrative + "\n\n```json\n{\"unrelated\":true}\n```",
			wantNilState:  true,
			wantUnchanged: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stripped, state := ExtractTaskState(tt.in)
			if tt.wantUnchanged {
				if stripped != tt.in {
					t.Errorf("markdown should be unchanged\ngot:  %q\nwant: %q", stripped, tt.in)
				}
			} else if tt.wantStripped != "" && strings.TrimSpace(stripped) != tt.wantStripped {
				t.Errorf("stripped mismatch\ngot:  %q\nwant: %q", stripped, tt.wantStripped)
			}
			if tt.wantNilState {
				if state != nil {
					t.Fatalf("expected nil state, got %+v", state)
				}
				return
			}
			if state == nil {
				t.Fatal("expected non-nil state")
			}
			if state.Status != tt.wantStatus || state.Next != tt.wantNext {
				t.Errorf("state mismatch: %+v", state)
			}
			if strings.Join(state.Done, "|") != strings.Join(tt.wantDone, "|") {
				t.Errorf("done mismatch: %v", state.Done)
			}
			if strings.Join(state.Blockers, "|") != strings.Join(tt.wantBlockers, "|") {
				t.Errorf("blockers mismatch: %v", state.Blockers)
			}
		})
	}
}

func TestExtractTaskState_Caps(t *testing.T) {
	// 防御：LLM 失控输出超长状态时必须截断。
	var done []string
	for i := 0; i < 50; i++ {
		done = append(done, "step")
	}
	longNext := strings.Repeat("n", 1000)
	in := "narrative\n```json\n{\"done\":[\"" + strings.Join(done, `","`) + "\"],\"next\":\"" + longNext + "\"}\n```"
	_, state := ExtractTaskState(in)
	if state == nil {
		t.Fatal("expected non-nil state")
	}
	if len(state.Done) > 8 {
		t.Errorf("done must be capped to 8, got %d", len(state.Done))
	}
	if len([]rune(state.Next)) > 210 {
		t.Errorf("next must be truncated, got %d runes", len([]rune(state.Next)))
	}
}
