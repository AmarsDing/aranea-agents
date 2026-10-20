package data

import (
	"context"
	crand "crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"math"
	"math/rand"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// BenchmarkKnowledgeScale 规模基准（U1-4 / C-T1）：10k chunks 集合上分别量
// dense ANN（pgvector HNSW 余弦，ef_search 走生产默认）与 SearchChunksBM25
// 词法路径的延迟，经分位桶近似上报 p50/p95。
//
// 需要一个可写的 Postgres（pgvector + pg_trgm 扩展可装），DSN 经环境变量传入；
// 基准在库内自建/自删隔离 schema，不触碰既有数据：
//
//	PowerShell: $env:KRATOS_TEST_PG_DSN = "postgres://postgres:123456@127.0.0.1:55432/aranea_test?sslmode=disable"
//	bash:       export KRATOS_TEST_PG_DSN="postgres://postgres:123456@127.0.0.1:55432/aranea_test?sslmode=disable"
//
// 向量维度默认 1536，可用 KRATOS_TEST_EMBED_DIM 覆盖。运行手册与容量规划
// 见 test/knowledge-scale-bench/README.md。
//
//	go test ./internal/data/ -run=NONE -bench=BenchmarkKnowledgeScale -benchtime=100x -count=1
func BenchmarkKnowledgeScale(b *testing.B) {
	fx := setupKnowledgeScaleBench(b)

	b.Run("denseANN", func(b *testing.B) {
		ctx := context.Background()
		// 32 个确定性查询向量轮换，避免单查询页缓存假象。
		qrng := rand.New(rand.NewSource(2))
		queries := make([][]float32, 32)
		for i := range queries {
			queries[i] = scaleBenchUnitVector(qrng, fx.dim)
		}
		probe, err := fx.repo.SearchChunks(ctx, biz.KnowledgeSearchQuery{
			CollectionID: fx.collectionID, TopK: 10,
		}, queries[0])
		if err != nil {
			b.Fatalf("dense probe: %v", err)
		}
		if len(probe) == 0 {
			b.Fatal("dense probe returned 0 hits on a populated 10k-chunk collection")
		}
		lat := newScaleBenchLatencyBuckets()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			start := time.Now()
			if _, err := fx.repo.SearchChunks(ctx, biz.KnowledgeSearchQuery{
				CollectionID: fx.collectionID, TopK: 10,
			}, queries[i%len(queries)]); err != nil {
				b.Fatalf("dense search: %v", err)
			}
			lat.add(time.Since(start))
		}
		b.StopTimer()
		lat.report(b)
	})

	b.Run("bm25", func(b *testing.B) {
		ctx := context.Background()
		// 查询词元都在合成语料内（needle-%04d），确定性轮换。
		queries := make([]string, 32)
		for i := range queries {
			queries[i] = fmt.Sprintf("needle-%04d", (i*31)%1000)
		}
		probe, err := fx.repo.SearchChunksBM25(ctx, biz.KnowledgeSearchQuery{
			CollectionID: fx.collectionID, Query: queries[0], TopK: 10,
		})
		if err != nil {
			b.Fatalf("bm25 probe: %v", err)
		}
		if len(probe) == 0 {
			b.Fatalf("bm25 probe query %q returned 0 hits; corpus must contain it", queries[0])
		}
		lat := newScaleBenchLatencyBuckets()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			start := time.Now()
			if _, err := fx.repo.SearchChunksBM25(ctx, biz.KnowledgeSearchQuery{
				CollectionID: fx.collectionID, Query: queries[i%len(queries)], TopK: 10,
			}); err != nil {
				b.Fatalf("bm25 search: %v", err)
			}
			lat.add(time.Since(start))
		}
		b.StopTimer()
		lat.report(b)
	})
}

const (
	scaleBenchDSNEnv    = "KRATOS_TEST_PG_DSN"
	scaleBenchDimEnv    = "KRATOS_TEST_EMBED_DIM"
	scaleBenchChunks    = 10000 // 规模阶梯第一档
	scaleBenchDocs      = 100
	scaleBenchSeedBatch = 500
)

type knowledgeScaleBenchFixture struct {
	repo         *knowledgeRepo
	collectionID string
	dim          int
}

// setupKnowledgeScaleBench 复用 knowledge_retrieval_baseline_test.go /
// knowledge_search_path_test.go 的建库套路：隔离 schema → search_path 钉住
// + 追加 public（pgvector 类型在 public）→ EnsureKnowledgeSchema → 裸 repo。
// 区别：DSN 只认 KRATOS_TEST_PG_DSN（空则 Skip），不接受 testhelper 的默认 DSN。
func setupKnowledgeScaleBench(b *testing.B) *knowledgeScaleBenchFixture {
	b.Helper()
	dsn := strings.TrimSpace(os.Getenv(scaleBenchDSNEnv))
	if dsn == "" {
		b.Skipf("%s 未设置，跳过规模基准。设置示例：\n"+
			"  PowerShell: $env:%s = \"postgres://postgres:123456@127.0.0.1:55432/aranea_test?sslmode=disable\"\n"+
			"  bash:       export %s=\"postgres://postgres:123456@127.0.0.1:55432/aranea_test?sslmode=disable\"",
			scaleBenchDSNEnv, scaleBenchDSNEnv, scaleBenchDSNEnv)
	}
	dim := 1536
	if v := strings.TrimSpace(os.Getenv(scaleBenchDimEnv)); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed <= 0 {
			b.Fatalf("invalid %s %q: want positive int", scaleBenchDimEnv, v)
		}
		dim = parsed
	}
	ctx := context.Background()

	schema := "test_scale_" + scaleBenchRandHex(b, 6)
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		b.Fatalf("open postgres: %v", err)
	}
	defer func() { _ = admin.Close() }()
	if _, err := admin.ExecContext(ctx, `CREATE SCHEMA `+pq.QuoteIdentifier(schema)); err != nil {
		b.Fatalf("create bench schema %s: %v (is Postgres reachable at %s?)", schema, err, scaleBenchDSNEnv)
	}
	b.Cleanup(func() {
		cleanupDB, err := sql.Open("postgres", dsn)
		if err != nil {
			return
		}
		defer func() { _ = cleanupDB.Close() }()
		_, _ = cleanupDB.ExecContext(context.Background(),
			`DROP SCHEMA IF EXISTS `+pq.QuoteIdentifier(schema)+` CASCADE`)
	})

	db, err := sql.Open("postgres", scaleBenchWithSearchPath(dsn, schema))
	if err != nil {
		b.Fatalf("open schema-scoped postgres: %v", err)
	}
	b.Cleanup(func() { _ = db.Close() })
	// 钉单连接后把 public 追加进 search_path（pgvector/pg_trgm 类型与操作符在 public）。
	db.SetMaxOpenConns(1)
	var cur string
	if err := db.QueryRow(`SELECT current_schema()`).Scan(&cur); err != nil {
		b.Fatalf("current_schema: %v", err)
	}
	if _, err := db.Exec(`SET search_path TO ` + cur + `, public`); err != nil {
		b.Fatalf("set search_path: %v", err)
	}
	d := &Data{rawDB: db, readDB: db, pg: db, pgRead: db, rwDB: NewReadWriteDB(db, db), lg: loggateway.NewNoop(), dialect: DialectPostgres}
	if err := EnsureKnowledgeSchema(ctx, db, dim); err != nil {
		b.Fatalf("ensure knowledge schema: %v", err)
	}
	fx := &knowledgeScaleBenchFixture{
		repo:         &knowledgeRepo{data: d, lg: loggateway.NewNoop()},
		collectionID: "scale-bench",
		dim:          dim,
	}
	if _, err := fx.repo.CreateCollection(ctx, biz.KnowledgeCollection{
		ID: fx.collectionID, Name: "scale benchmark", EmbeddingModel: "scale-bench-synth", Dim: dim, Status: "active",
	}); err != nil {
		b.Fatalf("create collection: %v", err)
	}
	seedKnowledgeScaleChunks(b, fx)
	return fx
}

// seedKnowledgeScaleChunks 批量插入 10k 合成 chunks：确定性单位向量 + 含
// needle 词元的可检索内容；100 文档 × 100 chunks，走生产 InsertChunks 路径。
func seedKnowledgeScaleChunks(b *testing.B, fx *knowledgeScaleBenchFixture) {
	b.Helper()
	ctx := context.Background()
	for i := 0; i < scaleBenchDocs; i++ {
		docID := fmt.Sprintf("scale-doc-%04d", i)
		if _, err := fx.repo.CreateDocument(ctx, biz.KnowledgeDocument{
			ID: docID, CollectionID: fx.collectionID, RelPath: fmt.Sprintf("docs/doc-%04d.md", i),
			Source: fmt.Sprintf("doc-%04d.md", i), Status: "indexed",
		}); err != nil {
			b.Fatalf("create doc %s: %v", docID, err)
		}
	}
	seedStart := time.Now()
	rng := rand.New(rand.NewSource(1))
	for base := 0; base < scaleBenchChunks; base += scaleBenchSeedBatch {
		batch := make([]biz.KnowledgeChunk, 0, scaleBenchSeedBatch)
		for i := base; i < base+scaleBenchSeedBatch && i < scaleBenchChunks; i++ {
			batch = append(batch, biz.KnowledgeChunk{
				ID:           fmt.Sprintf("scale-chunk-%05d", i),
				DocID:        fmt.Sprintf("scale-doc-%04d", i%scaleBenchDocs),
				CollectionID: fx.collectionID,
				Content:      scaleBenchChunkContent(i),
				Embedding:    scaleBenchUnitVector(rng, fx.dim),
				ChunkIndex:   i / scaleBenchDocs,
			})
		}
		if err := fx.repo.InsertChunks(ctx, batch); err != nil {
			b.Fatalf("insert chunks [%d,%d): %v", base, base+len(batch), err)
		}
	}
	b.Logf("seeded %d chunks (dim=%d) in %s", scaleBenchChunks, fx.dim, time.Since(seedStart).Round(time.Millisecond))
}

// scaleBenchChunkContent 混合中文常用词与幂等 needle 词元：BM25 凭 needle 命中，
// 高频词填充文档频率以贴近真实分布。
func scaleBenchChunkContent(i int) string {
	return fmt.Sprintf("巡检 变更 应急 安全基线 容量规划 监控告警 备份恢复 网络割接 核心机房 交换机 "+
		"needle-%04d filler-%04d 值班记录 健康度评估 演练总结", i%1000, i%97)
}

// scaleBenchUnitVector 用确定性 RNG 生成单位向量（高维高斯归一化）。
func scaleBenchUnitVector(rng *rand.Rand, dim int) []float32 {
	v := make([]float32, dim)
	var norm float64
	for i := range v {
		x := rng.NormFloat64()
		v[i] = float32(x)
		norm += x * x
	}
	inv := float32(1 / math.Sqrt(norm))
	for i := range v {
		v[i] *= inv
	}
	return v
}

// scaleBenchWithSearchPath 与 internal/data/testhelper.withSearchPath 同策略：
// URL 形式 DSN 追加 search_path 参数；关键字形式直接拼接。
func scaleBenchWithSearchPath(dsn, schema string) string {
	if u, err := url.Parse(dsn); err == nil && u.Scheme != "" {
		q := u.Query()
		q.Set("search_path", schema)
		u.RawQuery = q.Encode()
		return u.String()
	}
	return dsn + " search_path=" + schema
}

func scaleBenchRandHex(tb testing.TB, n int) string {
	tb.Helper()
	raw := make([]byte, n)
	if _, err := crand.Read(raw); err != nil {
		tb.Fatalf("rand: %v", err)
	}
	return hex.EncodeToString(raw)
}

// scaleBenchLatencyBuckets 分位桶：几何上界桶累计计数，按“首个累计 ≥ q·total
// 的桶的中点”近似 p50/p95——避免为海量迭代保存全部样本。
type scaleBenchLatencyBuckets struct {
	bounds []float64 // 上界（ms，升序）
	counts []uint64
	total  uint64
}

func newScaleBenchLatencyBuckets() *scaleBenchLatencyBuckets {
	bounds := []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000}
	return &scaleBenchLatencyBuckets{bounds: bounds, counts: make([]uint64, len(bounds)+1)}
}

func (h *scaleBenchLatencyBuckets) add(d time.Duration) {
	ms := float64(d) / float64(time.Millisecond)
	for i, ub := range h.bounds {
		if ms <= ub {
			h.counts[i]++
			h.total++
			return
		}
	}
	h.counts[len(h.bounds)]++
	h.total++
}

// quantile 近似分位数（ms）：目标桶内取中点；落在溢出桶时返回最大有限上界。
func (h *scaleBenchLatencyBuckets) quantile(q float64) float64 {
	if h.total == 0 {
		return 0
	}
	target := uint64(q*float64(h.total)) + 1
	var cum uint64
	prev := 0.0
	for i, ub := range h.bounds {
		cum += h.counts[i]
		if cum >= target {
			return (prev + ub) / 2
		}
		prev = ub
	}
	return prev
}

func (h *scaleBenchLatencyBuckets) report(b *testing.B) {
	b.Helper()
	p50, p95 := h.quantile(0.50), h.quantile(0.95)
	b.ReportMetric(p50, "p50-ms")
	b.ReportMetric(p95, "p95-ms")
	b.Logf("ops=%d p50=%.2fms p95=%.2fms", h.total, p50, p95)
}
