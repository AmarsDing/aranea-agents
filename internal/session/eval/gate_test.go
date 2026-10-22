package eval

// gate_test.go — 压缩金闸门驱动（A8 / P0-3 设计 §4.7）：装载 testdata/ 三枚
// 冻结 fixture，经 agent.RunToolResultPruneForEval（线上 hook 同源剪枝体）
// 折叠后跑 CheckCompressionGoldGate，断言零违规；token 缩减为软指标只打
// 日志（浮动不构成红）。另含一条合成反例钉死门禁的可失败性（防 vacuous gate）。

import (
	"path/filepath"
	"testing"
)

// 三枚冻结 fixture 全量过闸：硬指标（保护项零丢失 + 回读 100%）任何违例
// 都会以非空 Violation 列表浮出。
func TestCompressionGoldGate_FrozenFixtures(t *testing.T) {
	fixtures := []string{
		"retrieval-heavy.json",
		"tool-loop-heavy.json",
		"chit-chat.json",
	}
	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			tc, err := LoadTranscript(filepath.Join("testdata", name))
			if err != nil {
				t.Fatalf("load fixture: %v", err)
			}
			out, archive := RunPruneCollapse(tc)
			violations := CheckCompressionGoldGate(tc, out, func(locator string) (string, bool) {
				s, ok := archive[locator]
				return s, ok
			})
			for _, v := range violations {
				t.Errorf("gold gate violation: %s", v)
			}
			// 软指标：工具结果载荷字节缩减率只打日志（fixture 是灵敏度探针，
			// 缩减率浮动源于文本形状而非回归）。
			var before, after int64
			for i, m := range tc.Messages {
				before += int64(len(m.Content))
				after += int64(len(out.Messages[i].Content))
			}
			ratio := float64(0)
			if before > 0 {
				ratio = float64(before-after) / float64(before) * 100
			}
			t.Logf("fixture %s: folded=%d prune_bytes=%d payload %d -> %d bytes (%.1f%% reduction, soft metric)",
				tc.Name, out.Meta.Count, out.Meta.Bytes, before, after, ratio)
		})
	}
}

// 合成反例：保护项被改动时门禁必须红——钉死 gate 不是恒绿摆设。
func TestCompressionGoldGate_DetectsProtectedMutation(t *testing.T) {
	tc, err := LoadTranscript(filepath.Join("testdata", "retrieval-heavy.json"))
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	out, archive := RunPruneCollapse(tc)
	if len(tc.ProtectedIndices) == 0 {
		t.Fatal("fixture must declare protected indices for mutation probe")
	}
	mutated := tc.ProtectedIndices[0]
	out.Messages[mutated].Content = "tampered: retrieval result lost"
	violations := CheckCompressionGoldGate(tc, out, func(locator string) (string, bool) {
		s, ok := archive[locator]
		return s, ok
	})
	found := false
	for _, v := range violations {
		if v.Kind == ViolationProtectedMutated {
			found = true
		}
	}
	if !found {
		t.Fatalf("gate must flag protected mutation, got %v", violations)
	}
}

// 合成反例：回读缺失时门禁必须红（硬指标「回读 100%」的可失败性）。
func TestCompressionGoldGate_DetectsReadbackLoss(t *testing.T) {
	tc, err := LoadTranscript(filepath.Join("testdata", "tool-loop-heavy.json"))
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	out, _ := RunPruneCollapse(tc)
	if len(out.Folded) == 0 {
		t.Fatal("tool-loop fixture must fold at least one result")
	}
	violations := CheckCompressionGoldGate(tc, out, func(string) (string, bool) {
		return "", false // 归档丢失：readback 永远失败
	})
	found := false
	for _, v := range violations {
		if v.Kind == ViolationReadbackMismatch {
			found = true
		}
	}
	if !found {
		t.Fatalf("gate must flag readback loss, got %v", violations)
	}
}
