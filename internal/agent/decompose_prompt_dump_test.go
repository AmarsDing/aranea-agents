package agent

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDumpDecomposePrompt 是一次性诊断探针（非验收测试）：
// 将生产真实使用的 buildDecompositionPrompt 输出落盘，供
// test/synthesis-idem-probe/cmd/llmprobe 以完全一致的 prompt 测量
// decompose LLM 调用的延迟构成（TTFT / 推理段 / 内容段）。
//
// 仅在 ARANEA_DUMP_DECOMPOSE_PROMPT=1 时运行，默认跳过。
func TestDumpDecomposePrompt(t *testing.T) {
	if os.Getenv("ARANEA_DUMP_DECOMPOSE_PROMPT") != "1" {
		t.Skip("probe disabled (set ARANEA_DUMP_DECOMPOSE_PROMPT=1)")
	}
	msg := "请组建两个团队并行工作：团队一写一首关于春天的短诗，团队二写一首关于秋天的短诗，最后汇总成一份对比赏析。"
	prompt := buildDecompositionPrompt(msg, nil, 2)
	out := filepath.Join("..", "..", "test", "synthesis-idem-probe", "out", "decompose_prompt.txt")
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, []byte(prompt), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("prompt bytes=%d written to %s", len(prompt), out)
}
