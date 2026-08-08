package knowledge

import (
	"fmt"
	"runtime"
	"testing"
)

// ── SP1-D D-3：内存基准（NFR-SP1-4：10 万边 < 100MB） ───────────────────────

// buildBenchEdges 构造 n 条仿真边：每 10 边一文档（含 1 dangling + 1 embed），
// ID/文本长度贴近生产形态（块 ID ~20 字符、raw_target ~24、context ~60）。
func buildBenchEdges(n int) []KnowledgeBlockRefEdge {
	edges := make([]KnowledgeBlockRefEdge, 0, n)
	for i := 0; i < n; i++ {
		doc := fmt.Sprintf("doc-%08x", i/10)
		e := KnowledgeBlockRefEdge{
			CollectionID:    fmt.Sprintf("coll-%04x", i/2000),
			SrcBlockID:      fmt.Sprintf("blk-%010x", i),
			SrcDocID:        doc,
			DstCollectionID: fmt.Sprintf("coll-%04x", (i+977)/2000),
			DstDocID:        fmt.Sprintf("doc-%08x", (i*37+13)%(n/10)),
			DstBlockID:      fmt.Sprintf("blk-%010x", (i*31+7)%n),
			RawTarget:       fmt.Sprintf("notes/target-note-%06x", i),
			EdgeType:        "ref",
			Context:         fmt.Sprintf("参见 [[notes/target-note-%06x]] 一节的相关论述与上下文片段。", i),
		}
		switch i % 10 {
		case 3:
			e.DstCollectionID, e.DstDocID, e.DstBlockID = "", "", "" // dangling
		case 7:
			e.EdgeType = "embed"
		}
		edges = append(edges, e)
	}
	return edges
}

func heapAllocBytes() uint64 {
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

// TestLinkIndex_MemoryFootprint100K NFR-SP1-4 门控：10 万边内存图存活堆增量
// < 100MB（边单分配多索引共享指针；读侧拷贝切片不计入常驻）。
func TestLinkIndex_MemoryFootprint100K(t *testing.T) {
	const edgeCount = 100_000
	const limitBytes = 100 << 20

	before := heapAllocBytes()
	x := NewLinkIndex()
	x.LoadAll(buildBenchEdges(edgeCount))
	after := heapAllocBytes()

	used := after - before
	t.Logf("LinkIndex 100k edges heap delta = %.1f MB (limit 100 MB)", float64(used)/float64(1<<20))
	if used >= limitBytes {
		t.Fatalf("内存超标：%d bytes >= %d bytes", used, limitBytes)
	}
	// 防编译器优化掉整个图。
	if got := len(x.OutEdges("blk-0000000000", nil)); got != 1 {
		t.Fatalf("OutEdges = %d, want 1", got)
	}
}

// BenchmarkLinkIndex_LoadAll 全量构建吞吐（启动加载量级参考）。
func BenchmarkLinkIndex_LoadAll(b *testing.B) {
	edges := buildBenchEdges(100_000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		x := NewLinkIndex()
		x.LoadAll(edges)
	}
}

// BenchmarkLinkIndex_ApplyDocDelta 单文档增量 apply（10 出边，图内已有 10 万边）。
func BenchmarkLinkIndex_ApplyDocDelta(b *testing.B) {
	x := NewLinkIndex()
	x.LoadAll(buildBenchEdges(100_000))
	delta := buildBenchEdges(10)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		x.ApplyDocDelta("doc-00000000", delta)
	}
}
