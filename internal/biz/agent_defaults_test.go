package biz

import (
	"encoding/json"
	"testing"
)

func TestDefaultAgentRuntimeSettings_L1HistoryEnabled(t *testing.T) {
	s := DefaultAgentRuntimeSettings()
	if s.L1HistoryEnabled {
		t.Error("L1HistoryEnabled should default to false")
	}
}

// N2 (2026-08-13 链路审查): 压缩级联必须默认开。审查发现 __spirit__ 平均
// prompt 60K tokens（context rot 区），根因是 context_compaction_enabled /
// session_summary_enabled 默认 false：Aranea 压缩后经 EnqueueFrameworkSummary
// 同步的框架摘要从未被消费（AddSessionSummary=false → 历史不按 cutoff 截断），
// 框架请求级 compaction 也从未启用，历史无上限增长。
func TestDefaultAgentRuntimeSettings_CompressionStackOn(t *testing.T) {
	s := DefaultAgentRuntimeSettings()
	if !s.ContextCompactionEnabled {
		t.Error("ContextCompactionEnabled should default to true (N2)")
	}
	if !s.MemoryCompactEnabled {
		t.Error("MemoryCompactEnabled should default to true (N2)")
	}
	if !s.SessionSummaryEnabled {
		t.Error("SessionSummaryEnabled should default to true (N2)")
	}
}

// FR-12/P2: L2 召回默认开（评审 V7）。回归守卫：新 agent 的 standard 记忆
// 档位必须包含 L2 召回，否则回到「五个层里两个半在干活」的默认配置。
func TestDefaultAgentRuntimeSettings_L2RecallEnabled(t *testing.T) {
	s := DefaultAgentRuntimeSettings()
	if !s.L2RecallEnabled {
		t.Error("L2RecallEnabled should default to true (FR-12/P2)")
	}
	p := ResolveMemoryRuntimePolicy(&s)
	if !p.RecallL2 {
		t.Error("resolved policy should recall L2 by default")
	}
}

// P0-3 (2026-08-08): L4 injection must default to ON. The L4 entity graph was
// never injected into any prompt because l0_inject_l4 defaulted to false and
// all 55 existing rows kept that default; downstream guards (0.3 confidence
// gate + maxPaths cap) already bound the risk.
func TestDefaultAgentRuntimeSettings_L0InjectL4(t *testing.T) {
	s := DefaultAgentRuntimeSettings()
	if !s.L0InjectL4 {
		t.Error("L0InjectL4 should default to true (P0-3)")
	}
	p := ResolveMemoryRuntimePolicy(&s)
	if !p.InjectL4 {
		t.Error("resolved policy should inject L4 by default")
	}
}

// P0-4 (2026-08-08): default L3 minScore 0.55 false-kills typical relevant
// hits (weighted Total ≈ 0.4-0.5 under keyword .25/vector .30/importance
// .20/recency .15/quality .10). Default drops to 0.35; explicit 0.00 rows
// (filter disabled by user) are untouched.
func TestDefaultAgentRuntimeSettings_L3RecallMinScore(t *testing.T) {
	s := DefaultAgentRuntimeSettings()
	if s.L3RecallMinScore != 0.35 {
		t.Errorf("L3RecallMinScore = %v, want 0.35 (P0-4)", s.L3RecallMinScore)
	}
}

func TestDefaultToolsDenyFrameworkMemory(t *testing.T) {
	// Verify the constant is valid JSON
	var list []string
	if err := json.Unmarshal([]byte(DefaultToolsDenyFrameworkMemory), &list); err != nil {
		t.Fatalf("DefaultToolsDenyFrameworkMemory is not valid JSON: %v", err)
	}
	// Verify it contains exactly the 5 framework memory tools
	expected := []string{"memory_add", "memory_update", "memory_delete", "memory_search", "memory_load"}
	if len(list) != len(expected) {
		t.Errorf("got %d items, want %d", len(list), len(expected))
	}
	for _, e := range expected {
		found := false
		for _, v := range list {
			if v == e {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing %q in DefaultToolsDenyFrameworkMemory", e)
		}
	}
}

func TestDefaultAgentRuntimeSettings_ToolsDenyFrameworkMemory(t *testing.T) {
	s := DefaultAgentRuntimeSettings()
	if s.ToolsDenyJSON != DefaultToolsDenyFrameworkMemory {
		t.Errorf("ToolsDenyJSON = %q, want %q", s.ToolsDenyJSON, DefaultToolsDenyFrameworkMemory)
	}
	// Verify it denies framework memory tools
	var list []string
	json.Unmarshal([]byte(s.ToolsDenyJSON), &list)
	if len(list) != 5 {
		t.Errorf("expected 5 denied tools, got %d", len(list))
	}
}

func TestDefaultAgentRuntimeSettings_ToolsRetryEnabled(t *testing.T) {
	s := DefaultAgentRuntimeSettings()
	if !s.ToolsRetryEnabled {
		t.Error("ToolsRetryEnabled should default to true (selective retry for ConcurrentSafe tools)")
	}
}

func TestDefaultAgentRuntimeSettings_ToolsParallelEnabled(t *testing.T) {
	s := DefaultAgentRuntimeSettings()
	if !s.ToolsParallelEnabled {
		t.Error("ToolsParallelEnabled should default to true")
	}
}
