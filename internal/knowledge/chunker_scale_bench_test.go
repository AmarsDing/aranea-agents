package knowledge_test

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"aranea-agents/internal/knowledge"
)

// BenchmarkChunkerScale 规模基准（U1-4 / C-T1）：对约 1MB 的确定性 markdown
// 语料分块，覆盖 char 512 与 markdown 两种策略，经 b.SetBytes 报告 MB/s。
// 语料由 math/rand.New(rand.NewSource(1)) 固定种子生成，跨运行可复现。
//
// 运行：go test ./internal/knowledge/ -run=NONE -bench=BenchmarkChunkerScale -benchmem -count=1
func BenchmarkChunkerScale(b *testing.B) {
	text := buildScaleBenchMarkdown(b, 1<<20)
	b.Logf("corpus bytes=%d", len(text))

	b.Run("char512", func(b *testing.B) {
		c := knowledge.NewChunker(512, knowledge.DefaultChunkOverlap, knowledge.ChunkByChar)
		probe := c.Split(text)
		if len(probe) == 0 {
			b.Fatal("char chunker returned no chunks")
		}
		assertChunkIndicesOrdered(b, probe)
		b.SetBytes(int64(len(text)))
		b.ResetTimer()
		var chunks int
		for i := 0; i < b.N; i++ {
			chunks = len(c.Split(text))
		}
		b.StopTimer()
		b.ReportMetric(float64(chunks), "chunks")
	})

	b.Run("markdown512", func(b *testing.B) {
		probe, err := knowledge.SplitWithStrategy(knowledge.ChunkByMarkdown, text, 512, knowledge.DefaultChunkOverlap)
		if err != nil {
			b.Fatalf("markdown strategy: %v", err)
		}
		if len(probe) == 0 {
			b.Fatal("markdown chunker returned no chunks")
		}
		b.SetBytes(int64(len(text)))
		b.ResetTimer()
		var chunks int
		for i := 0; i < b.N; i++ {
			out, err := knowledge.SplitWithStrategy(knowledge.ChunkByMarkdown, text, 512, knowledge.DefaultChunkOverlap)
			if err != nil {
				b.Fatalf("markdown strategy: %v", err)
			}
			chunks = len(out)
		}
		b.StopTimer()
		b.ReportMetric(float64(chunks), "chunks")
	})
}

// buildScaleBenchMarkdown 生成约 sizeBytes 的确定性 markdown 语料，混合
// 标题 / 段落 / 列表 / 代码块四类块以贴近真实 vault 文档形态。
func buildScaleBenchMarkdown(tb testing.TB, sizeBytes int) string {
	tb.Helper()
	rng := rand.New(rand.NewSource(1))
	topics := []string{"值班巡检", "变更管理", "应急预案", "安全基线", "容量规划", "监控告警", "备份恢复", "网络割接"}
	keywords := []string{"UPS", "PDU", "核心机房", "交换机", "防火墙", "CN2 专线", "TACACS+", "BGP", "存储阵列", "虚拟化集群"}
	phases := []string{"例行", "专项", "抽查"}

	var sb strings.Builder
	sb.Grow(sizeBytes + 1<<12)
	for section := 1; sb.Len() < sizeBytes; section++ {
		fmt.Fprintf(&sb, "%s %s 章节 %d\n\n",
			strings.Repeat("#", 1+rng.Intn(3)), topics[rng.Intn(len(topics))], section)
		for p, paras := 0, 1+rng.Intn(3); p < paras; p++ {
			for s, sentences := 0, 2+rng.Intn(3); s < sentences; s++ {
				fmt.Fprintf(&sb, "%s 流程要求对 %s 执行%s核查，结果记录归档；",
					topics[rng.Intn(len(topics))], keywords[rng.Intn(len(keywords))], phases[rng.Intn(len(phases))])
			}
			sb.WriteString("\n\n")
		}
		if rng.Intn(2) == 0 {
			for i, items := 0, 3+rng.Intn(5); i < items; i++ {
				fmt.Fprintf(&sb, "- %s 检查项 %d：%s 状态确认与记录。\n",
					topics[rng.Intn(len(topics))], i+1, keywords[rng.Intn(len(keywords))])
			}
			sb.WriteString("\n")
		}
		if rng.Intn(3) == 0 {
			sb.WriteString("```bash\n")
			for i, lines := 0, 2+rng.Intn(4); i < lines; i++ {
				fmt.Fprintf(&sb, "show interface Gi0/%d | include %s\n", rng.Intn(48), keywords[rng.Intn(len(keywords))])
			}
			sb.WriteString("```\n\n")
		}
	}
	return sb.String()
}

func assertChunkIndicesOrdered(tb testing.TB, chunks []knowledge.Chunk) {
	tb.Helper()
	for i, ch := range chunks {
		if ch.ChunkIndex != i {
			tb.Fatalf("chunk %d has wrong ChunkIndex %d", i, ch.ChunkIndex)
		}
	}
}
