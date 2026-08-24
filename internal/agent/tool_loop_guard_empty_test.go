package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

func mustJSONValue(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("bad test json: %v", err)
	}
	return v
}

// 空结果熔断必须覆盖全部检索类工具：sh-04 事故模式（检索为空 → 模型换词重试
// → 空转烧预算）此前仅 knowledge_search 注册空判定谓词，其余检索类工具的
// 同类风险开放。空判定为通用集合字段探测（results/items/memories/skills/
// hits/matches/entries/documents 空数组），与各工具真实序列化形态对齐：
// memory_search=SearchMemoryResponse{results}、duckduckgo={results}、
// web_research={results}、wikipedia={results}、arxiv={results}。
func TestLoopGuardEmptyBreaker_RetrievalTools(t *testing.T) {
	cases := []struct {
		tool      string
		emptyJSON string
	}{
		{"knowledge_search", `{"chunks": []}`},
		{"memory_search", `{"results": []}`},
		{"skill_search", `{"skills": []}`},
		{"duckduckgo_search", `{"results": []}`},
		{"web_research", `{"results": []}`},
		{"google_search", `{"results": []}`},
		{"arxiv_search", `{"results": []}`},
		{"wikipedia_search", `{"results": []}`},
	}
	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			g := newToolLoopGuard(nil)
			ctx := newTestInvocationContext("inv-empty-" + tc.tool)
			empty := mustJSONValue(t, tc.emptyJSON)
			// 连续 2 次空结果（换参重试，空结果熔断无视参数差异）。
			for i, q := range []string{"q1", "q2"} {
				if err := runLoopGuardTurn(t, g, ctx, tc.tool, `{"query":"`+q+`"}`, empty, nil); err != nil {
					t.Fatalf("empty call %d should pass, got blocked: %v", i+1, err)
				}
			}
			// 第 3 次（连续空已达阈值）必须熔断。
			err := runLoopGuardTurn(t, g, ctx, tc.tool, `{"query":"q3"}`, empty, nil)
			if err == nil {
				t.Fatalf("%s: call after %d consecutive empties must be blocked", tc.tool, loopGuardEmptyStreakThreshold)
			}
			if !strings.HasPrefix(err.Error(), loopGuardMarker) {
				t.Fatalf("blocked error should carry loop guard marker, got %q", err.Error())
			}
		})
	}
}

// 非空结果立即清零连续空计数（熔断只针对「库中确无资料仍重试」的空转）。
func TestLoopGuardEmptyBreaker_NonEmptyResetsStreak(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-empty-reset")
	tool := "memory_search"
	empty := mustJSONValue(t, `{"results": []}`)
	filled := mustJSONValue(t, `{"results": [{"id": "m1"}]}`)

	if err := runLoopGuardTurn(t, g, ctx, tool, `{"query":"a"}`, empty, nil); err != nil {
		t.Fatalf("call 1: %v", err)
	}
	if err := runLoopGuardTurn(t, g, ctx, tool, `{"query":"b"}`, filled, nil); err != nil {
		t.Fatalf("call 2: %v", err)
	}
	// streak 已清零：再次空结果只记 1，不触发熔断。
	if err := runLoopGuardTurn(t, g, ctx, tool, `{"query":"c"}`, empty, nil); err != nil {
		t.Fatalf("streak must reset on non-empty result, got blocked: %v", err)
	}
}

// 含 error 字段的结果不判空——失败重试归熔断器治理，不与空结果熔断混淆。
func TestLoopGuardEmptyBreaker_ErrorResultNotEmpty(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-empty-error")
	tool := "web_research"
	errResult := mustJSONValue(t, `{"error": "provider timeout", "results": []}`)

	for i := 0; i < loopGuardEmptyStreakThreshold+1; i++ {
		args := `{"query":"x` + string(rune('a'+i)) + `"}`
		if err := runLoopGuardTurn(t, g, ctx, tool, args, errResult, nil); err != nil {
			t.Fatalf("error-shaped result must not trip empty breaker, call %d blocked: %v", i+1, err)
		}
	}
}
