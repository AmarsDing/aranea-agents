package biz

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ═══════════════════════════════════════════════════════════════════════════
// A. 相似度引擎（SkillSimilarityEngine）
// 需求来源：20-skill.design.md —— 四维加权（name/description/body/tag）+ 可选 embedding 增强
// ═══════════════════════════════════════════════════════════════════════════

type stubEmbedder struct {
	score float64
	err   error
	calls int
}

func (s *stubEmbedder) CosineSimilarity(_ context.Context, _, _ string) (float64, error) {
	s.calls++
	return s.score, s.err
}

func TestSimilarity_IdenticalSkills_HighRisk(t *testing.T) {
	eng := NewSkillSimilarityEngine(nil, DefaultDedupWeights(), loggateway.NewNoop())
	a := SkillDedupCandidate{ID: "a", Name: "数据库查询工具", Description: "用于执行数据库查询", BodyPreview: "## 用法\n查询数据库", Tags: []string{"db", "sql"}}
	b := a
	b.ID = "b"
	res, err := eng.Compare(context.Background(), a, b)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if res.TotalScore < 0.99 {
		t.Errorf("identical skills: TotalScore = %v, want ~1.0", res.TotalScore)
	}
	if res.ConflictRisk != "high" || res.Recommendation != "block_duplicate" {
		t.Errorf("identical skills: risk=%q rec=%q, want high/block_duplicate", res.ConflictRisk, res.Recommendation)
	}
}

func TestSimilarity_DistinctSkills_LowRisk(t *testing.T) {
	eng := NewSkillSimilarityEngine(nil, DefaultDedupWeights(), loggateway.NewNoop())
	a := SkillDedupCandidate{ID: "a", Name: "alpha beta gamma", Description: "delta epsilon zeta", BodyPreview: "theta iota kappa", Tags: []string{"a1", "b1"}}
	b := SkillDedupCandidate{ID: "b", Name: "lambda mu nu", Description: "xi omicron pi", BodyPreview: "rho sigma tau", Tags: []string{"c1", "d1"}}
	res, err := eng.Compare(context.Background(), a, b)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if res.TotalScore != 0 {
		t.Errorf("distinct skills: TotalScore = %v, want 0", res.TotalScore)
	}
	if res.ConflictRisk != "low" || res.Recommendation != "keep_separate" {
		t.Errorf("distinct skills: risk=%q rec=%q, want low/keep_separate", res.ConflictRisk, res.Recommendation)
	}
}

// 名称高度相似（≥0.9）即使总分低也应判 high/block_duplicate —— 防止换皮重复注册。
func TestSimilarity_NameHighScore_AloneTriggersHighRisk(t *testing.T) {
	eng := NewSkillSimilarityEngine(nil, DefaultDedupWeights(), loggateway.NewNoop())
	a := SkillDedupCandidate{ID: "a", Name: "数据库查询工具", Description: "delta epsilon zeta", BodyPreview: "theta iota kappa", Tags: []string{"a1"}}
	b := SkillDedupCandidate{ID: "b", Name: "数据库查询工具", Description: "xi omicron pi", BodyPreview: "rho sigma tau", Tags: []string{"c1"}}
	res, err := eng.Compare(context.Background(), a, b)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if res.ConflictRisk != "high" || res.Recommendation != "block_duplicate" {
		t.Errorf("name-identical: risk=%q rec=%q, want high/block_duplicate (total=%v)", res.ConflictRisk, res.Recommendation, res.TotalScore)
	}
}

// 总分落在 [0.5, 0.8) 且名称不高度相似 → medium/suggest_refine。
func TestSimilarity_MediumScore_SuggestRefine(t *testing.T) {
	eng := NewSkillSimilarityEngine(nil, DefaultDedupWeights(), loggateway.NewNoop())
	// description + body 完全相同（权重 0.25+0.30=0.55），name/tags 完全不同。
	a := SkillDedupCandidate{ID: "a", Name: "alpha beta", Description: "执行数据库查询操作", BodyPreview: "## 用法\n查询", Tags: []string{"a1"}}
	b := SkillDedupCandidate{ID: "b", Name: "gamma delta", Description: "执行数据库查询操作", BodyPreview: "## 用法\n查询", Tags: []string{"c1"}}
	res, err := eng.Compare(context.Background(), a, b)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if res.TotalScore < 0.5 || res.TotalScore >= 0.8 {
		t.Fatalf("TotalScore = %v, want [0.5, 0.8)", res.TotalScore)
	}
	if res.ConflictRisk != "medium" || res.Recommendation != "suggest_refine" {
		t.Errorf("risk=%q rec=%q, want medium/suggest_refine", res.ConflictRisk, res.Recommendation)
	}
}

// Embedding 增强：body 维度按 50/50 混合 jaccard 与语义分，方法标记为 jaccard+embedding。
func TestSimilarity_EmbeddingBlend_BoostsBody(t *testing.T) {
	emb := &stubEmbedder{score: 0.8}
	eng := NewSkillSimilarityEngine(emb, DefaultDedupWeights(), loggateway.NewNoop())
	a := SkillDedupCandidate{ID: "a", Name: "alpha", Description: "x", BodyPreview: "theta iota", Tags: nil}
	b := SkillDedupCandidate{ID: "b", Name: "beta", Description: "y", BodyPreview: "rho sigma", Tags: nil}
	res, err := eng.Compare(context.Background(), a, b)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if emb.calls != 1 {
		t.Errorf("embedder calls = %d, want 1", emb.calls)
	}
	for _, d := range res.Dimensions {
		if d.Dimension == DimBody {
			want := 0.5 * 0.8 // jaccard=0 → 0.5*0 + 0.5*0.8
			if d.Score != want {
				t.Errorf("body score = %v, want %v (embedding blend)", d.Score, want)
			}
			if d.Method != "jaccard+embedding" {
				t.Errorf("body method = %q, want jaccard+embedding", d.Method)
			}
		}
	}
}

// Embedding 失败时降级为纯 jaccard，不影响比较结果返回。
func TestSimilarity_EmbeddingError_JaccardFallback(t *testing.T) {
	emb := &stubEmbedder{err: errors.New("embedding service down")}
	eng := NewSkillSimilarityEngine(emb, DefaultDedupWeights(), loggateway.NewNoop())
	a := SkillDedupCandidate{ID: "a", Name: "alpha", Description: "x", BodyPreview: "theta iota", Tags: nil}
	b := SkillDedupCandidate{ID: "b", Name: "beta", Description: "y", BodyPreview: "theta iota", Tags: nil}
	res, err := eng.Compare(context.Background(), a, b)
	if err != nil {
		t.Fatalf("Compare must not fail on embedder error: %v", err)
	}
	for _, d := range res.Dimensions {
		if d.Dimension == DimBody {
			if d.Score != 1.0 {
				t.Errorf("body score = %v, want 1.0 (pure jaccard fallback)", d.Score)
			}
			if d.Method != "jaccard" {
				t.Errorf("body method = %q, want jaccard", d.Method)
			}
		}
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// B. Jaccard 工具函数（中文 bigram 分词是召回关键）
// ═══════════════════════════════════════════════════════════════════════════

func TestJaccard_EmptyInputs(t *testing.T) {
	if got := jaccardSimilarity("", ""); got != 0 {
		t.Errorf("jaccard('','') = %v, want 0", got)
	}
	if got := jaccardSimilarity("hello world", ""); got != 0 {
		t.Errorf("jaccard(x,'') = %v, want 0", got)
	}
	if got := jaccardSimilarity("hello world", "hello world"); got != 1.0 {
		t.Errorf("jaccard(x,x) = %v, want 1.0", got)
	}
}

// 中文必须按 bigram 分词，否则整串视为单 token 导致相似度全 0。
func TestWordSet_ChineseBigrams(t *testing.T) {
	set := wordSet("数据库查询")
	for _, want := range []string{"数据", "据库", "库查", "查询"} {
		if !set[want] {
			t.Errorf("wordSet(数据库查询) missing bigram %q; got %v", want, set)
		}
	}
	if len(set) != 4 {
		t.Errorf("wordSet(数据库查询) size = %d, want 4; got %v", len(set), set)
	}
	// 单字符英文 token 应被丢弃（避免噪音）
	if wordSet("a b c")["a"] {
		t.Error("single-char english token must be dropped")
	}
}

func TestSetJaccard_CaseInsensitive(t *testing.T) {
	if got := setJaccard([]string{"Python", "GO"}, []string{"python", "go"}); got != 1.0 {
		t.Errorf("setJaccard case-insensitive = %v, want 1.0", got)
	}
	if got := setJaccard(nil, nil); got != 0 {
		t.Errorf("setJaccard(nil,nil) = %v, want 0", got)
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// C. 去重检测（SkillDedupUsecase）
// 需求来源：20-skill.design.md —— 相似 Skill 分组、缓存、阈值
// ═══════════════════════════════════════════════════════════════════════════

type dedupStubReader struct {
	summaries []SkillSummary
	err       error
	calls     int
}

func (r *dedupStubReader) ListAllSkillSummaries(_ context.Context) ([]SkillSummary, error) {
	r.calls++
	return r.summaries, r.err
}

// stubComparer 按预置分数表返回相似度，key 为排序后的 "idA|idB"。
type stubComparer struct {
	scores map[string]*SimilarityResult
	err    error
}

func pairKey(a, b string) string {
	if a > b {
		a, b = b, a
	}
	return a + "|" + b
}

func (c *stubComparer) Compare(_ context.Context, a, b SkillDedupCandidate) (*SimilarityResult, error) {
	if c.err != nil {
		return nil, c.err
	}
	if res, ok := c.scores[pairKey(a.ID, b.ID)]; ok {
		return res, nil
	}
	return &SimilarityResult{SkillAID: a.ID, SkillBID: b.ID}, nil
}

func scorePair(score float64, dim SimilarityDimension) *SimilarityResult {
	return &SimilarityResult{
		TotalScore: score,
		Dimensions: []DimensionScore{{Dimension: dim, Score: score, Method: "jaccard"}},
	}
}

func TestDedup_LessThanTwoSkills_NoGroups(t *testing.T) {
	reader := &dedupStubReader{summaries: []SkillSummary{{ID: "only-one"}}}
	uc := NewSkillDedupUsecase(reader, &stubComparer{}, loggateway.NewNoop())
	groups, err := uc.DetectDuplicateGroups(context.Background())
	if err != nil {
		t.Fatalf("DetectDuplicateGroups: %v", err)
	}
	if len(groups) != 0 {
		t.Errorf("groups = %d, want 0 for single skill", len(groups))
	}
}

func TestDedup_IdenticalSkills_SingleGroup(t *testing.T) {
	s := SkillSummary{ID: "x", Name: "数据库查询工具", Description: "执行数据库查询", BodyPreview: "## 用法\n查询", Tags: []string{"db"}}
	a, b := s, s
	a.ID, b.ID = "a", "b"
	reader := &dedupStubReader{summaries: []SkillSummary{a, b}}
	eng := NewSkillSimilarityEngine(nil, DefaultDedupWeights(), loggateway.NewNoop())
	uc := NewSkillDedupUsecase(reader, eng, loggateway.NewNoop())

	groups, err := uc.DetectDuplicateGroups(context.Background())
	if err != nil {
		t.Fatalf("DetectDuplicateGroups: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(groups))
	}
	g := groups[0]
	if len(g.Skills) != 2 {
		t.Errorf("group size = %d, want 2", len(g.Skills))
	}
	if g.OverlapScore < 0.99 {
		t.Errorf("OverlapScore = %v, want ~1.0", g.OverlapScore)
	}
	if g.ConflictRisk != "high" || g.Recommendation != "block_duplicate" {
		t.Errorf("risk=%q rec=%q, want high/block_duplicate", g.ConflictRisk, g.Recommendation)
	}
	if !strings.Contains(g.OverlapType, "name_similarity") {
		t.Errorf("OverlapType = %q, want contains name_similarity", g.OverlapType)
	}
}

// 传递分组：A~B、B~C 相似但 A~C 不相似 → union-find 应归入同一组。
func TestDedup_TransitiveGrouping_UnionFind(t *testing.T) {
	reader := &dedupStubReader{summaries: []SkillSummary{{ID: "a"}, {ID: "b"}, {ID: "c"}}}
	cmp := &stubComparer{scores: map[string]*SimilarityResult{
		pairKey("a", "b"): scorePair(0.6, DimName),
		pairKey("b", "c"): scorePair(0.6, DimName),
	}}
	uc := NewSkillDedupUsecase(reader, cmp, loggateway.NewNoop())
	groups, err := uc.DetectDuplicateGroups(context.Background())
	if err != nil {
		t.Fatalf("DetectDuplicateGroups: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1 (transitive)", len(groups))
	}
	if len(groups[0].Skills) != 3 {
		t.Errorf("group size = %d, want 3", len(groups[0].Skills))
	}
}

func TestDedup_BelowThreshold_NoGroups(t *testing.T) {
	reader := &dedupStubReader{summaries: []SkillSummary{{ID: "a"}, {ID: "b"}}}
	cmp := &stubComparer{scores: map[string]*SimilarityResult{
		pairKey("a", "b"): scorePair(0.49, DimName), // 低于 0.5 阈值
	}}
	uc := NewSkillDedupUsecase(reader, cmp, loggateway.NewNoop())
	groups, err := uc.DetectDuplicateGroups(context.Background())
	if err != nil {
		t.Fatalf("DetectDuplicateGroups: %v", err)
	}
	if len(groups) != 0 {
		t.Errorf("groups = %d, want 0 below threshold", len(groups))
	}
}

// 单对比较失败不应中断整体扫描（降级跳过该对）。
func TestDedup_CompareError_SkipsPair(t *testing.T) {
	reader := &dedupStubReader{summaries: []SkillSummary{{ID: "a"}, {ID: "b"}}}
	cmp := &stubComparer{err: errors.New("compare boom")}
	uc := NewSkillDedupUsecase(reader, cmp, loggateway.NewNoop())
	groups, err := uc.DetectDuplicateGroups(context.Background())
	if err != nil {
		t.Fatalf("Compare errors must not fail the whole scan: %v", err)
	}
	if len(groups) != 0 {
		t.Errorf("groups = %d, want 0", len(groups))
	}
}

func TestDedup_CacheHit_SkipsReader(t *testing.T) {
	reader := &dedupStubReader{summaries: []SkillSummary{{ID: "a"}, {ID: "b"}}}
	cmp := &stubComparer{scores: map[string]*SimilarityResult{pairKey("a", "b"): scorePair(0.9, DimName)}}
	uc := NewSkillDedupUsecase(reader, cmp, loggateway.NewNoop())

	if _, err := uc.DetectDuplicateGroups(context.Background()); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if reader.calls != 1 {
		t.Fatalf("reader calls = %d, want 1", reader.calls)
	}
	// 第二次调用命中 10 分钟 TTL 缓存，不再访问 reader。
	if _, err := uc.DetectDuplicateGroups(context.Background()); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if reader.calls != 1 {
		t.Errorf("cached call hit reader: calls = %d, want 1", reader.calls)
	}
}

// InvalidateDedupCache 本身功能正确，但生产代码无任何调用方（审计发现 D-4：
// skill 增删改/合并后缓存最长 10 分钟过期，结果陈旧）。此测试验证方法本身行为。
func TestDedup_InvalidateCache_ForcesRescan(t *testing.T) {
	reader := &dedupStubReader{summaries: []SkillSummary{{ID: "a"}, {ID: "b"}}}
	cmp := &stubComparer{scores: map[string]*SimilarityResult{pairKey("a", "b"): scorePair(0.9, DimName)}}
	uc := NewSkillDedupUsecase(reader, cmp, loggateway.NewNoop())

	if _, err := uc.DetectDuplicateGroups(context.Background()); err != nil {
		t.Fatalf("first call: %v", err)
	}
	uc.InvalidateDedupCache()
	if _, err := uc.DetectDuplicateGroups(context.Background()); err != nil {
		t.Fatalf("after invalidate: %v", err)
	}
	if reader.calls != 2 {
		t.Errorf("reader calls = %d, want 2 after invalidation", reader.calls)
	}
}

// 分组元数据必须取组内最高分 pair 的信息。
// 构造：comp{0,4} 内部分数 0.9（高），comp{1,2} 内部分数 0.6（低），
// 合并 pair (2,4)=0.55。union-by-rank 等秩合并时 Find(2)=1 的 root 存活，
// meta{0}（含 0.9 高分与 body 维度）被静默丢弃 → 组 OverlapScore 报 0.6。
func TestDedup_GroupMetadata_UsesBestPair(t *testing.T) {
	reader := &dedupStubReader{summaries: []SkillSummary{{ID: "s0"}, {ID: "s1"}, {ID: "s2"}, {ID: "s3"}, {ID: "s4"}}}
	cmp := &stubComparer{scores: map[string]*SimilarityResult{
		pairKey("s0", "s4"): scorePair(0.9, DimBody),
		pairKey("s1", "s2"): scorePair(0.6, DimName),
		pairKey("s2", "s4"): scorePair(0.55, DimTag),
	}}
	uc := NewSkillDedupUsecase(reader, cmp, loggateway.NewNoop())
	groups, err := uc.DetectDuplicateGroups(context.Background())
	if err != nil {
		t.Fatalf("DetectDuplicateGroups: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(groups))
	}
	g := groups[0]
	if len(g.Skills) != 4 {
		t.Fatalf("group size = %d, want 4", len(g.Skills))
	}
	if g.OverlapScore < 0.89 {
		t.Errorf("OverlapScore = %v, want 0.9 (best pair in group); metadata from absorbed union-find root was lost", g.OverlapScore)
	}
	if !strings.Contains(g.OverlapType, "body_similarity") {
		t.Errorf("OverlapType = %q, want contains body_similarity (from best pair); absorbed root overlap types were lost", g.OverlapType)
	}
}

func TestDedup_PreCancelledContext_ReturnsError(t *testing.T) {
	reader := &dedupStubReader{summaries: []SkillSummary{{ID: "a"}, {ID: "b"}}}
	uc := NewSkillDedupUsecase(reader, &stubComparer{}, loggateway.NewNoop())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := uc.DetectDuplicateGroups(ctx)
	if err == nil {
		t.Error("cancelled context must return error, got nil")
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// D. 合并（SkillMergeUsecase 三阶段：融合 → Gate → 事务应用）
// ═══════════════════════════════════════════════════════════════════════════

type mergeStubReader struct {
	skills map[string]*SkillMergeSource
	err    error
}

func (r *mergeStubReader) GetFullSkillForMerge(_ context.Context, skillID string) (*SkillMergeSource, error) {
	if r.err != nil {
		return nil, r.err
	}
	s, ok := r.skills[skillID]
	if !ok {
		return nil, apierror.NotFound("SKILL", "skill not found")
	}
	return s, nil
}

type mergeStubWriter struct {
	result     *SkillMergeResult
	err        error
	applyCalls int
	lastParams SkillMergeApplyParams
}

func (w *mergeStubWriter) ApplyMerge(_ context.Context, params SkillMergeApplyParams) (*SkillMergeResult, error) {
	w.applyCalls++
	w.lastParams = params
	if w.err != nil {
		return nil, w.err
	}
	if w.result != nil {
		return w.result, nil
	}
	return &SkillMergeResult{TargetSkillID: params.TargetID, NewVersionID: "v-new", FusedBody: params.FusedBody, FusedTags: params.FusedTags}, nil
}

type stubContentFuser struct {
	fused *FusedContent
	err   error
	calls int
}

func (f *stubContentFuser) Fuse(_ context.Context, _, _ SkillMergeSource) (*FusedContent, error) {
	f.calls++
	return f.fused, f.err
}

func newMergeFixture() (*mergeStubReader, *mergeStubWriter) {
	reader := &mergeStubReader{skills: map[string]*SkillMergeSource{
		"src": {ID: "src", Name: "源技能", Body: "## 用法\n源用法\n", Tags: []string{"python", "db"}, Status: "active"},
		"tgt": {ID: "tgt", Name: "目标技能", Body: "## 用法\n目标用法\n## 高级\n目标高级\n", Tags: []string{"Python", "web"}, Status: "active"},
	}}
	return reader, &mergeStubWriter{}
}

func TestMerge_SameSourceAndTarget_Rejected(t *testing.T) {
	reader, writer := newMergeFixture()
	uc := NewSkillMergeUsecase(reader, writer, nil, nil, loggateway.NewNoop())
	_, err := uc.Merge(context.Background(), SkillMergeRequest{SourceID: "x", TargetID: "x", Strategy: MergeStrategyAppend})
	if !isAPIErrorCode(err, apierror.CodeBadRequest) {
		t.Errorf("err = %v, want BadRequest", err)
	}
}

func TestMerge_DeletedSource_Rejected(t *testing.T) {
	reader, writer := newMergeFixture()
	reader.skills["src"].Status = "deleted"
	uc := NewSkillMergeUsecase(reader, writer, nil, nil, loggateway.NewNoop())
	_, err := uc.Merge(context.Background(), SkillMergeRequest{SourceID: "src", TargetID: "tgt", Strategy: MergeStrategyAppend})
	if !isAPIErrorCode(err, apierror.CodeBadRequest) {
		t.Errorf("err = %v, want BadRequest for deleted source", err)
	}
}

func TestMerge_ArchivedTarget_Rejected(t *testing.T) {
	reader, writer := newMergeFixture()
	reader.skills["tgt"].Status = "archived"
	uc := NewSkillMergeUsecase(reader, writer, nil, nil, loggateway.NewNoop())
	_, err := uc.Merge(context.Background(), SkillMergeRequest{SourceID: "src", TargetID: "tgt", Strategy: MergeStrategyAppend})
	if !isAPIErrorCode(err, apierror.CodeBadRequest) {
		t.Errorf("err = %v, want BadRequest for archived target", err)
	}
}

// ai_fuse 已废弃，必须明确报错而非静默走其他路径。
func TestMerge_AIFuseStrategy_RejectedAsDeprecated(t *testing.T) {
	reader, writer := newMergeFixture()
	uc := NewSkillMergeUsecase(reader, writer, &stubContentFuser{fused: &FusedContent{Body: "x"}}, nil, loggateway.NewNoop())
	_, err := uc.Merge(context.Background(), SkillMergeRequest{SourceID: "src", TargetID: "tgt", Strategy: MergeStrategyAIFuse})
	if !isAPIErrorCode(err, apierror.CodeBadRequest) {
		t.Errorf("err = %v, want BadRequest for deprecated ai_fuse", err)
	}
	if writer.applyCalls != 0 {
		t.Error("ApplyMerge must not be called for deprecated strategy")
	}
}

// A5: appendWithDedup must produce deterministic output regardless of Go map
// iteration order — merged sections appear in sorted heading order.
func TestMerge_Append_DeterministicOutput(t *testing.T) {
	source := &SkillMergeSource{ID: "src", Name: "源", Body: "## Zebra\nz\n## Alpha\na\n## Middle\nm\n", Status: "active"}
	target := &SkillMergeSource{ID: "tgt", Name: "目标", Body: "## 用法\n目标用法\n", Status: "active"}
	var first string
	for i := 0; i < 20; i++ {
		got := appendWithDedup(target.Body, source.Body, source.Name, loggateway.NewNoop())
		if i == 0 {
			first = got
			continue
		}
		if got != first {
			t.Fatalf("appendWithDedup non-deterministic:\nfirst:\n%s\n\ngot:\n%s", first, got)
		}
	}
	// Sorted order: Alpha < Middle < Zebra.
	ia := strings.Index(first, "## Alpha")
	im := strings.Index(first, "## Middle")
	iz := strings.Index(first, "## Zebra")
	if ia < 0 || im < 0 || iz < 0 || !(ia < im && im < iz) {
		t.Errorf("sections not in sorted order (alpha=%d middle=%d zebra=%d):\n%s", ia, im, iz, first)
	}
}

// A3: successful merge must invalidate the dedup result cache.
func TestMerge_Success_InvalidatesDedupCache(t *testing.T) {
	reader, writer := newMergeFixture()
	inv := &fakeDedupInvalidator{}
	uc := NewSkillMergeUsecase(reader, writer, nil, nil, loggateway.NewNoop())
	uc.SetDedupCacheInvalidator(inv)
	_, err := uc.Merge(context.Background(), SkillMergeRequest{SourceID: "src", TargetID: "tgt", Strategy: MergeStrategyAppend})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if inv.calls == 0 {
		t.Error("expected dedup cache invalidation after successful merge")
	}
}

// A3: failed merge must NOT invalidate the cache (no mutation happened).
func TestMerge_Failure_KeepsDedupCache(t *testing.T) {
	reader, writer := newMergeFixture()
	writer.err = errors.New("db down")
	inv := &fakeDedupInvalidator{}
	uc := NewSkillMergeUsecase(reader, writer, nil, nil, loggateway.NewNoop())
	uc.SetDedupCacheInvalidator(inv)
	_, err := uc.Merge(context.Background(), SkillMergeRequest{SourceID: "src", TargetID: "tgt", Strategy: MergeStrategyAppend})
	if err == nil {
		t.Fatal("expected merge error")
	}
	if inv.calls != 0 {
		t.Errorf("failed merge must not invalidate dedup cache, got %d calls", inv.calls)
	}
}

type fakeDedupInvalidator struct{ calls int }

func (f *fakeDedupInvalidator) InvalidateDedupCache() { f.calls++ }

func TestMerge_RuleFuse_WithoutFuser_Rejected(t *testing.T) {
	reader, writer := newMergeFixture()
	uc := NewSkillMergeUsecase(reader, writer, nil, nil, loggateway.NewNoop())
	_, err := uc.Merge(context.Background(), SkillMergeRequest{SourceID: "src", TargetID: "tgt", Strategy: MergeStrategyRuleFuse})
	if !isAPIErrorCode(err, apierror.CodeBadRequest) {
		t.Errorf("err = %v, want BadRequest when fuser not configured", err)
	}
}

func TestMerge_RuleFuse_Success(t *testing.T) {
	reader, writer := newMergeFixture()
	fuser := &stubContentFuser{fused: &FusedContent{Body: "fused-body", Tags: []string{"merged"}}}
	uc := NewSkillMergeUsecase(reader, writer, fuser, nil, loggateway.NewNoop())
	res, err := uc.Merge(context.Background(), SkillMergeRequest{SourceID: "src", TargetID: "tgt", Strategy: MergeStrategyRuleFuse})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if fuser.calls != 1 {
		t.Errorf("fuser calls = %d, want 1", fuser.calls)
	}
	if writer.lastParams.FusedBody != "fused-body" {
		t.Errorf("FusedBody = %q, want fused-body", writer.lastParams.FusedBody)
	}
	if !strings.Contains(writer.lastParams.MergeReason, "src") {
		t.Errorf("MergeReason = %q, want reference to source id", writer.lastParams.MergeReason)
	}
	if res.TargetSkillID != "tgt" {
		t.Errorf("TargetSkillID = %q, want tgt", res.TargetSkillID)
	}
}

func TestMerge_ManualPick_EmptyBody_Rejected(t *testing.T) {
	reader, writer := newMergeFixture()
	uc := NewSkillMergeUsecase(reader, writer, nil, nil, loggateway.NewNoop())
	_, err := uc.Merge(context.Background(), SkillMergeRequest{SourceID: "src", TargetID: "tgt", Strategy: MergeStrategyManualPick, ManualBody: "   "})
	if !isAPIErrorCode(err, apierror.CodeBadRequest) {
		t.Errorf("err = %v, want BadRequest for empty manual_body", err)
	}
}

// 标签并集大小写不敏感去重，保留 target 侧的原始大小写。
func TestMerge_ManualPick_MergesTagsCaseInsensitive(t *testing.T) {
	reader, writer := newMergeFixture()
	uc := NewSkillMergeUsecase(reader, writer, nil, nil, loggateway.NewNoop())
	res, err := uc.Merge(context.Background(), SkillMergeRequest{SourceID: "src", TargetID: "tgt", Strategy: MergeStrategyManualPick, ManualBody: "人工选定内容"})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	// target ["Python","web"] + source ["python","db"] → ["Python","web","db"]
	want := []string{"Python", "web", "db"}
	if len(res.FusedTags) != len(want) {
		t.Fatalf("FusedTags = %v, want %v", res.FusedTags, want)
	}
	for i, tag := range want {
		if res.FusedTags[i] != tag {
			t.Errorf("FusedTags[%d] = %q, want %q (full: %v)", i, res.FusedTags[i], tag, res.FusedTags)
		}
	}
}

// append 策略：源中与目标同名的 ## 段落必须跳过，新段落追加。
func TestMerge_Append_SkipsDuplicateSections(t *testing.T) {
	reader, writer := newMergeFixture()
	uc := NewSkillMergeUsecase(reader, writer, nil, nil, loggateway.NewNoop())
	res, err := uc.Merge(context.Background(), SkillMergeRequest{SourceID: "src", TargetID: "tgt", Strategy: MergeStrategyAppend})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	body := res.FusedBody
	if !strings.Contains(body, "# Merged from: 源技能") {
		t.Errorf("merged body missing source attribution header:\n%s", body)
	}
	if strings.Count(body, "## 用法") != 1 {
		t.Errorf("duplicate section '## 用法' not skipped, count = %d:\n%s", strings.Count(body, "## 用法"), body)
	}
	if !strings.Contains(body, "## 高级") {
		t.Errorf("target-only section '## 高级' must be preserved:\n%s", body)
	}
}

// Gate 不通过必须阻断事务应用。
func TestMerge_GateFailure_BlocksApply(t *testing.T) {
	reader, writer := newMergeFixture()
	gate := &mockSkillGateVerifier{result: &GateVerificationResult{
		Passed: false,
		Checks: []GateCheckResult{{Name: "security", Passed: false, Reason: "detected api_key"}},
	}}
	uc := NewSkillMergeUsecase(reader, writer, nil, gate, loggateway.NewNoop())
	_, err := uc.Merge(context.Background(), SkillMergeRequest{SourceID: "src", TargetID: "tgt", Strategy: MergeStrategyAppend})
	if !isAPIErrorCode(err, apierror.CodeBadRequest) {
		t.Errorf("err = %v, want BadRequest on gate failure", err)
	}
	if !strings.Contains(err.Error(), "security") {
		t.Errorf("error should surface gate failure detail, got: %v", err)
	}
	if writer.applyCalls != 0 {
		t.Error("ApplyMerge must not be called when gate fails")
	}
}

func TestMerge_UnknownStrategy_Rejected(t *testing.T) {
	reader, writer := newMergeFixture()
	uc := NewSkillMergeUsecase(reader, writer, nil, nil, loggateway.NewNoop())
	_, err := uc.Merge(context.Background(), SkillMergeRequest{SourceID: "src", TargetID: "tgt", Strategy: "teleport"})
	if !isAPIErrorCode(err, apierror.CodeBadRequest) {
		t.Errorf("err = %v, want BadRequest for unknown strategy", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// E. 统一进化编排器（SkillEvolutionOrchestrator）
// 需求来源：7-agent-evolution.design.md —— 触发器扫描、冷却期、审批状态机
// ═══════════════════════════════════════════════════════════════════════════

type orchStubCheckReader struct {
	hasPending     bool
	pendingErr     error
	latestByAction map[string]*UnifiedEvolutionSuggestion
	latestErr      error
}

func (r *orchStubCheckReader) HasPendingForTarget(_ context.Context, _, _ string) (bool, error) {
	return r.hasPending, r.pendingErr
}
func (r *orchStubCheckReader) GetLatestByTarget(_ context.Context, _, _ string) (*UnifiedEvolutionSuggestion, error) {
	return nil, nil
}
func (r *orchStubCheckReader) GetLatestByTargetAndAction(_ context.Context, _, _, actionType string) (*UnifiedEvolutionSuggestion, error) {
	if r.latestErr != nil {
		return nil, r.latestErr
	}
	return r.latestByAction[actionType], nil
}

type orchStubQueryReader struct {
	byID map[string]*UnifiedEvolutionSuggestion
}

func (r *orchStubQueryReader) GetByID(_ context.Context, id string) (*UnifiedEvolutionSuggestion, error) {
	s, ok := r.byID[id]
	if !ok {
		return nil, apierror.NotFound("EVO", "suggestion not found")
	}
	return s, nil
}
func (r *orchStubQueryReader) ListByTarget(_ context.Context, _, _, _ string, _, _ int) ([]UnifiedEvolutionSuggestion, error) {
	return nil, nil
}
func (r *orchStubQueryReader) CountByTarget(_ context.Context, _, _, _ string) (int, error) {
	return 0, nil
}
func (r *orchStubQueryReader) ListByTargetAndAction(_ context.Context, _, _, _, _ string, _, _ int) ([]UnifiedEvolutionSuggestion, error) {
	return nil, nil
}
func (r *orchStubQueryReader) CountByTargetAndAction(_ context.Context, _, _, _, _ string) (int, error) {
	return 0, nil
}

type orchStubWriter struct {
	created      []UnifiedEvolutionSuggestion
	createErr    error
	statusCalls  []string
	expireCalls  int
	expireResult int
}

func (w *orchStubWriter) Create(_ context.Context, s UnifiedEvolutionSuggestion) error {
	if w.createErr != nil {
		return w.createErr
	}
	w.created = append(w.created, s)
	return nil
}
func (w *orchStubWriter) UpdateStatus(_ context.Context, id string, status string, _ string, _ string) error {
	w.statusCalls = append(w.statusCalls, id+":"+status)
	return nil
}
func (w *orchStubWriter) UpdateStatusCAS(_ context.Context, id string, _ []string, to string, _ string, _ string) (bool, error) {
	w.statusCalls = append(w.statusCalls, id+":"+to)
	return true, nil
}
func (w *orchStubWriter) UpdateDraftBody(_ context.Context, _ string, _ string) error { return nil }
func (w *orchStubWriter) UpdateLifecycleStatus(_ context.Context, _ string, _ string) error {
	return nil
}
func (w *orchStubWriter) UpdateSandboxResult(_ context.Context, _ string, _ bool, _ json.RawMessage) error {
	return nil
}
func (w *orchStubWriter) UpdateMetadataKey(_ context.Context, _ string, _ string, _ string) error {
	return nil
}
func (w *orchStubWriter) ExpireOlderThan(_ context.Context, _ time.Time) (int, error) {
	w.expireCalls++
	return w.expireResult, nil
}

type stubTrigger struct {
	targetType   EvolutionTargetType
	actionType   EvolutionActionType
	source       string
	suggestions  []UnifiedEvolutionSuggestion
	err          error
	checkCalls   int
	checkedForID string
}

func (tr *stubTrigger) TargetType() EvolutionTargetType { return tr.targetType }
func (tr *stubTrigger) ActionType() EvolutionActionType { return tr.actionType }
func (tr *stubTrigger) TriggerSource() string           { return tr.source }
func (tr *stubTrigger) Check(_ context.Context, targetID string) ([]UnifiedEvolutionSuggestion, error) {
	tr.checkCalls++
	tr.checkedForID = targetID
	return tr.suggestions, tr.err
}

func newTriggerSuggestion(actionType EvolutionActionType) UnifiedEvolutionSuggestion {
	return UnifiedEvolutionSuggestion{
		ID:         "sug-1",
		TargetType: EvolutionTargetSkill,
		TargetID:   "skill-1",
		ActionType: actionType,
		Status:     "pending",
		CreatedAt:  time.Now().UTC(),
	}
}

// 已有 pending 建议时短路，触发器不应再执行（防止建议堆积）。
func TestOrchestrator_CheckAndCreate_PendingExists_SkipsAll(t *testing.T) {
	check := &orchStubCheckReader{hasPending: true}
	writer := &orchStubWriter{}
	orch := NewSkillEvolutionOrchestrator(check, &orchStubQueryReader{}, writer, loggateway.NewNoop())
	tr := &stubTrigger{targetType: EvolutionTargetSkill, suggestions: []UnifiedEvolutionSuggestion{newTriggerSuggestion(EvolutionActionImprove)}}
	orch.RegisterTrigger(tr)

	created, err := orch.CheckAndCreate(context.Background(), EvolutionTargetSkill, "skill-1")
	if err != nil {
		t.Fatalf("CheckAndCreate: %v", err)
	}
	if len(created) != 0 {
		t.Errorf("created = %d, want 0 when pending exists", len(created))
	}
	if tr.checkCalls != 0 {
		t.Errorf("trigger must not run when pending exists, checkCalls = %d", tr.checkCalls)
	}
}

func TestOrchestrator_CheckAndCreate_CreatesFromMatchingTrigger(t *testing.T) {
	check := &orchStubCheckReader{}
	writer := &orchStubWriter{}
	orch := NewSkillEvolutionOrchestrator(check, &orchStubQueryReader{}, writer, loggateway.NewNoop())
	sug := newTriggerSuggestion(EvolutionActionImprove)
	tr := &stubTrigger{targetType: EvolutionTargetSkill, suggestions: []UnifiedEvolutionSuggestion{sug}}
	orch.RegisterTrigger(tr)

	created, err := orch.CheckAndCreate(context.Background(), EvolutionTargetSkill, "skill-1")
	if err != nil {
		t.Fatalf("CheckAndCreate: %v", err)
	}
	if len(created) != 1 || len(writer.created) != 1 {
		t.Errorf("created = %d, writer = %d, want 1/1", len(created), len(writer.created))
	}
	if tr.checkedForID != "skill-1" {
		t.Errorf("trigger checked target %q, want skill-1", tr.checkedForID)
	}
}

// 目标类型不匹配的触发器必须跳过（skill 扫描不触发 agent 触发器）。
func TestOrchestrator_CheckAndCreate_SkipsMismatchedTargetType(t *testing.T) {
	check := &orchStubCheckReader{}
	writer := &orchStubWriter{}
	orch := NewSkillEvolutionOrchestrator(check, &orchStubQueryReader{}, writer, loggateway.NewNoop())
	tr := &stubTrigger{targetType: EvolutionTargetAgent, suggestions: []UnifiedEvolutionSuggestion{newTriggerSuggestion(EvolutionActionEvolve)}}
	orch.RegisterTrigger(tr)

	created, err := orch.CheckAndCreate(context.Background(), EvolutionTargetSkill, "skill-1")
	if err != nil {
		t.Fatalf("CheckAndCreate: %v", err)
	}
	if len(created) != 0 || tr.checkCalls != 0 {
		t.Errorf("mismatched trigger must be skipped: created=%d checkCalls=%d", len(created), tr.checkCalls)
	}
}

// 单个触发器失败不应阻断其他触发器（故障隔离）。
func TestOrchestrator_CheckAndCreate_TriggerError_ContinuesWithOthers(t *testing.T) {
	check := &orchStubCheckReader{}
	writer := &orchStubWriter{}
	orch := NewSkillEvolutionOrchestrator(check, &orchStubQueryReader{}, writer, loggateway.NewNoop())
	bad := &stubTrigger{targetType: EvolutionTargetSkill, err: errors.New("trigger boom")}
	good := &stubTrigger{targetType: EvolutionTargetSkill, suggestions: []UnifiedEvolutionSuggestion{newTriggerSuggestion(EvolutionActionImprove)}}
	orch.RegisterTrigger(bad)
	orch.RegisterTrigger(good)

	created, err := orch.CheckAndCreate(context.Background(), EvolutionTargetSkill, "skill-1")
	if err != nil {
		t.Fatalf("trigger error must not fail the whole scan: %v", err)
	}
	if len(created) != 1 {
		t.Errorf("created = %d, want 1 from healthy trigger", len(created))
	}
}

// 冷却期内（同 actionType 168h）不再创建建议。
func TestOrchestrator_CheckAndCreate_CooldownActive_Skips(t *testing.T) {
	recent := &UnifiedEvolutionSuggestion{Status: "pending", CreatedAt: time.Now().UTC().Add(-1 * time.Hour)}
	check := &orchStubCheckReader{latestByAction: map[string]*UnifiedEvolutionSuggestion{
		string(EvolutionActionImprove): recent,
	}}
	writer := &orchStubWriter{}
	orch := NewSkillEvolutionOrchestrator(check, &orchStubQueryReader{}, writer, loggateway.NewNoop())
	tr := &stubTrigger{targetType: EvolutionTargetSkill, suggestions: []UnifiedEvolutionSuggestion{newTriggerSuggestion(EvolutionActionImprove)}}
	orch.RegisterTrigger(tr)

	created, err := orch.CheckAndCreate(context.Background(), EvolutionTargetSkill, "skill-1")
	if err != nil {
		t.Fatalf("CheckAndCreate: %v", err)
	}
	if len(created) != 0 || len(writer.created) != 0 {
		t.Errorf("cooldown active: created = %d, want 0", len(created))
	}
}

// 冷却期过后（>168h）允许再次创建。
func TestOrchestrator_CheckAndCreate_CooldownExpired_Creates(t *testing.T) {
	old := &UnifiedEvolutionSuggestion{Status: "pending", CreatedAt: time.Now().UTC().Add(-(EvoTriggerCooldownHours + 1) * time.Hour)}
	check := &orchStubCheckReader{latestByAction: map[string]*UnifiedEvolutionSuggestion{
		string(EvolutionActionImprove): old,
	}}
	writer := &orchStubWriter{}
	orch := NewSkillEvolutionOrchestrator(check, &orchStubQueryReader{}, writer, loggateway.NewNoop())
	tr := &stubTrigger{targetType: EvolutionTargetSkill, suggestions: []UnifiedEvolutionSuggestion{newTriggerSuggestion(EvolutionActionImprove)}}
	orch.RegisterTrigger(tr)

	created, err := orch.CheckAndCreate(context.Background(), EvolutionTargetSkill, "skill-1")
	if err != nil {
		t.Fatalf("CheckAndCreate: %v", err)
	}
	if len(created) != 1 {
		t.Errorf("cooldown expired: created = %d, want 1", len(created))
	}
}

// 不同 actionType 的冷却期相互独立。
func TestOrchestrator_CheckAndCreate_CooldownPerActionType(t *testing.T) {
	recent := &UnifiedEvolutionSuggestion{Status: "pending", CreatedAt: time.Now().UTC()}
	check := &orchStubCheckReader{latestByAction: map[string]*UnifiedEvolutionSuggestion{
		string(EvolutionActionImprove): recent, // improve 在冷却期
	}}
	writer := &orchStubWriter{}
	orch := NewSkillEvolutionOrchestrator(check, &orchStubQueryReader{}, writer, loggateway.NewNoop())
	tr := &stubTrigger{targetType: EvolutionTargetSkill, suggestions: []UnifiedEvolutionSuggestion{newTriggerSuggestion(EvolutionActionCreate)}} // create 不在冷却期
	orch.RegisterTrigger(tr)

	created, err := orch.CheckAndCreate(context.Background(), EvolutionTargetSkill, "skill-1")
	if err != nil {
		t.Fatalf("CheckAndCreate: %v", err)
	}
	if len(created) != 1 {
		t.Errorf("different actionType must not be blocked by cooldown: created = %d, want 1", len(created))
	}
}

// DB 唯一约束冲突（并发创建）视为正常，跳过不报错。
func TestOrchestrator_CheckAndCreate_DuplicateKey_Tolerated(t *testing.T) {
	check := &orchStubCheckReader{}
	writer := &orchStubWriter{createErr: errors.New("UNIQUE constraint failed: idx_ues_pending_target")}
	orch := NewSkillEvolutionOrchestrator(check, &orchStubQueryReader{}, writer, loggateway.NewNoop())
	tr := &stubTrigger{targetType: EvolutionTargetSkill, suggestions: []UnifiedEvolutionSuggestion{newTriggerSuggestion(EvolutionActionImprove)}}
	orch.RegisterTrigger(tr)

	created, err := orch.CheckAndCreate(context.Background(), EvolutionTargetSkill, "skill-1")
	if err != nil {
		t.Fatalf("duplicate key must be tolerated: %v", err)
	}
	if len(created) != 0 {
		t.Errorf("created = %d, want 0 for duplicate", len(created))
	}
}

func TestOrchestrator_CheckAndCreate_WriterError_Propagates(t *testing.T) {
	check := &orchStubCheckReader{}
	writer := &orchStubWriter{createErr: errors.New("disk io error")}
	orch := NewSkillEvolutionOrchestrator(check, &orchStubQueryReader{}, writer, loggateway.NewNoop())
	tr := &stubTrigger{targetType: EvolutionTargetSkill, suggestions: []UnifiedEvolutionSuggestion{newTriggerSuggestion(EvolutionActionImprove)}}
	orch.RegisterTrigger(tr)

	_, err := orch.CheckAndCreate(context.Background(), EvolutionTargetSkill, "skill-1")
	if err == nil {
		t.Error("non-duplicate writer error must propagate")
	}
}

func TestOrchestrator_Approve_FromPending(t *testing.T) {
	query := &orchStubQueryReader{byID: map[string]*UnifiedEvolutionSuggestion{
		"sug-1": {ID: "sug-1", Status: "pending"},
	}}
	writer := &orchStubWriter{}
	orch := NewSkillEvolutionOrchestrator(&orchStubCheckReader{}, query, writer, loggateway.NewNoop())
	if err := orch.Approve(context.Background(), "sug-1", "user:1"); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if len(writer.statusCalls) != 1 || writer.statusCalls[0] != "sug-1:approved" {
		t.Errorf("statusCalls = %v, want [sug-1:approved]", writer.statusCalls)
	}
}

// 状态机守卫：非 pending 状态（如已 approved）不得再次 approve。
func TestOrchestrator_Approve_FromApproved_Rejected(t *testing.T) {
	query := &orchStubQueryReader{byID: map[string]*UnifiedEvolutionSuggestion{
		"sug-1": {ID: "sug-1", Status: "approved"},
	}}
	writer := &orchStubWriter{}
	orch := NewSkillEvolutionOrchestrator(&orchStubCheckReader{}, query, writer, loggateway.NewNoop())
	err := orch.Approve(context.Background(), "sug-1", "user:1")
	if !isAPIErrorCode(err, apierror.CodeBadRequest) {
		t.Errorf("err = %v, want BadRequest for non-pending approve", err)
	}
	if len(writer.statusCalls) != 0 {
		t.Error("UpdateStatus must not be called on illegal transition")
	}
}

func TestOrchestrator_Reject_FromPending(t *testing.T) {
	query := &orchStubQueryReader{byID: map[string]*UnifiedEvolutionSuggestion{
		"sug-1": {ID: "sug-1", Status: "pending"},
	}}
	writer := &orchStubWriter{}
	orch := NewSkillEvolutionOrchestrator(&orchStubCheckReader{}, query, writer, loggateway.NewNoop())
	if err := orch.Reject(context.Background(), "sug-1", "user:1", "不适用"); err != nil {
		t.Fatalf("Reject: %v", err)
	}
	if len(writer.statusCalls) != 1 || writer.statusCalls[0] != "sug-1:rejected" {
		t.Errorf("statusCalls = %v, want [sug-1:rejected]", writer.statusCalls)
	}
}

func TestOrchestrator_Reject_FromExpired_Rejected(t *testing.T) {
	query := &orchStubQueryReader{byID: map[string]*UnifiedEvolutionSuggestion{
		"sug-1": {ID: "sug-1", Status: "expired"},
	}}
	orch := NewSkillEvolutionOrchestrator(&orchStubCheckReader{}, query, &orchStubWriter{}, loggateway.NewNoop())
	err := orch.Reject(context.Background(), "sug-1", "user:1", "x")
	if !isAPIErrorCode(err, apierror.CodeBadRequest) {
		t.Errorf("err = %v, want BadRequest for expired reject", err)
	}
}

// CreateSuggestion 对 DB 唯一约束冲突静默成功（并发安全兜底）。
func TestOrchestrator_CreateSuggestion_DuplicateKey_Swallowed(t *testing.T) {
	writer := &orchStubWriter{createErr: errors.New("duplicate entry")}
	orch := NewSkillEvolutionOrchestrator(&orchStubCheckReader{}, &orchStubQueryReader{}, writer, loggateway.NewNoop())
	if err := orch.CreateSuggestion(context.Background(), newTriggerSuggestion(EvolutionActionImprove)); err != nil {
		t.Errorf("duplicate key must return nil, got %v", err)
	}
}

func TestOrchestrator_CreateSuggestion_OtherError_Propagates(t *testing.T) {
	writer := &orchStubWriter{createErr: errors.New("connection reset")}
	orch := NewSkillEvolutionOrchestrator(&orchStubCheckReader{}, &orchStubQueryReader{}, writer, loggateway.NewNoop())
	if err := orch.CreateSuggestion(context.Background(), newTriggerSuggestion(EvolutionActionImprove)); err == nil {
		t.Error("non-duplicate error must propagate")
	}
}

func TestOrchestrator_ExpirePending_Delegates(t *testing.T) {
	writer := &orchStubWriter{expireResult: 3}
	orch := NewSkillEvolutionOrchestrator(&orchStubCheckReader{}, &orchStubQueryReader{}, writer, loggateway.NewNoop())
	n, err := orch.ExpirePending(context.Background())
	if err != nil {
		t.Fatalf("ExpirePending: %v", err)
	}
	if n != 3 || writer.expireCalls != 1 {
		t.Errorf("expired = %d calls = %d, want 3/1", n, writer.expireCalls)
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// F. 学习闭环（LearningLoopUsecase：观察 → 模式 → 提案 → 验证 → 注册）
// 需求来源：7-agent-evolution.design.md —— chat 中自动总结的学习闭环
// ═══════════════════════════════════════════════════════════════════════════

type obsStubRW struct {
	list  []Observation
	err   error
	calls int
}

func (o *obsStubRW) ListByAgent(_ context.Context, _ string, _ time.Time) ([]Observation, error) {
	o.calls++
	return o.list, o.err
}
func (o *obsStubRW) CountByAgent(_ context.Context, _ string, _ time.Time) (int64, error) {
	return int64(len(o.list)), nil
}
func (o *obsStubRW) Create(_ context.Context, obs Observation) (Observation, error) { return obs, nil }
func (o *obsStubRW) BatchCreate(_ context.Context, _ []Observation) error           { return nil }

type patternStubRW struct {
	created   []Pattern
	existing  []Pattern
	createErr error
}

func (p *patternStubRW) ListByAgent(_ context.Context, agentID string, status string) ([]Pattern, error) {
	var out []Pattern
	for _, pt := range append(append([]Pattern{}, p.existing...), p.created...) {
		if pt.AgentID == agentID && string(pt.Status) == status {
			out = append(out, pt)
		}
	}
	return out, nil
}
func (p *patternStubRW) GetByID(_ context.Context, id string) (Pattern, error) {
	for _, pt := range p.created {
		if pt.ID == id {
			return pt, nil
		}
	}
	return Pattern{}, apierror.NotFound("LEARNING", "pattern not found")
}
func (p *patternStubRW) Create(_ context.Context, pt Pattern) (Pattern, error) {
	if p.createErr != nil {
		return Pattern{}, p.createErr
	}
	p.created = append(p.created, pt)
	return pt, nil
}
func (p *patternStubRW) UpdateStatus(_ context.Context, _ string, status PatternStatus) (Pattern, error) {
	return Pattern{Status: status}, nil
}

// learningStubAgents 嵌入完整 AgentRepository，仅覆盖 RunLoop 需要的方法。
type learningStubAgents struct {
	AgentRepository
	settings AgentRuntimeSettings
	err      error
}

func (a *learningStubAgents) GetAgentRuntimeSettings(_ context.Context, _ string) (AgentRuntimeSettings, error) {
	return a.settings, a.err
}

func toolCallObs(id, toolName string) Observation {
	return Observation{
		ID:         id,
		AgentID:    "agent-1",
		Kind:       ObservationKindToolCall,
		Content:    "call " + toolName,
		Metadata:   `{"tool_name":"` + toolName + `"}`,
		ObservedAt: time.Now().UTC(),
	}
}

// 观察数 <3 时不产生模式（样本不足防噪）。
func TestDetectPatterns_InsufficientObservations_NoPattern(t *testing.T) {
	obs := &obsStubRW{list: []Observation{toolCallObs("o1", "search"), toolCallObs("o2", "search")}}
	pats := &patternStubRW{}
	uc := &LearningLoopUsecase{obs: obs, patterns: pats, lg: loggateway.NewNoop()}
	patterns, err := uc.DetectPatterns(context.Background(), "agent-1")
	if err != nil {
		t.Fatalf("DetectPatterns: %v", err)
	}
	if len(patterns) != 0 || len(pats.created) != 0 {
		t.Errorf("patterns = %d created = %d, want 0/0", len(patterns), len(pats.created))
	}
}

// 总数够但分散在不同 bucket（每 bucket <3）→ 不产生模式。
func TestDetectPatterns_ScatteredBuckets_NoPattern(t *testing.T) {
	obs := &obsStubRW{list: []Observation{
		toolCallObs("o1", "search"),
		{ID: "o2", AgentID: "agent-1", Kind: ObservationKindFeedback, Content: "good"},
		{ID: "o3", AgentID: "agent-1", Kind: ObservationKindMemoryHit, Content: "hit"},
	}}
	pats := &patternStubRW{}
	uc := &LearningLoopUsecase{obs: obs, patterns: pats, lg: loggateway.NewNoop()}
	patterns, err := uc.DetectPatterns(context.Background(), "agent-1")
	if err != nil {
		t.Fatalf("DetectPatterns: %v", err)
	}
	if len(patterns) != 0 {
		t.Errorf("patterns = %d, want 0 for scattered buckets", len(patterns))
	}
}

// 同 bucket ≥3 → 生成模式，含证据 ID 与置信度。
func TestDetectPatterns_BucketOfThree_CreatesPattern(t *testing.T) {
	obs := &obsStubRW{list: []Observation{toolCallObs("o1", "search"), toolCallObs("o2", "search"), toolCallObs("o3", "search")}}
	pats := &patternStubRW{}
	uc := &LearningLoopUsecase{obs: obs, patterns: pats, lg: loggateway.NewNoop()}
	patterns, err := uc.DetectPatterns(context.Background(), "agent-1")
	if err != nil {
		t.Fatalf("DetectPatterns: %v", err)
	}
	if len(patterns) != 1 {
		t.Fatalf("patterns = %d, want 1", len(patterns))
	}
	p := patterns[0]
	if p.Kind != string(ObservationKindToolCall) {
		t.Errorf("Kind = %q, want tool_call", p.Kind)
	}
	if p.Frequency != 3 || p.Confidence != 1.0 {
		t.Errorf("Frequency = %d Confidence = %v, want 3/1.0", p.Frequency, p.Confidence)
	}
	var evidence []string
	if err := json.Unmarshal([]byte(p.Evidence), &evidence); err != nil || len(evidence) != 3 {
		t.Errorf("Evidence = %q, want 3 observation IDs", p.Evidence)
	}
	if !strings.Contains(p.Description, "search(3)") {
		t.Errorf("Description = %q, want contains search(3)", p.Description)
	}
}

// 低置信度 bucket（占比 <10%）被过滤。
func TestDetectPatterns_LowConfidenceBucket_Skipped(t *testing.T) {
	list := []Observation{toolCallObs("t1", "search"), toolCallObs("t2", "search"), toolCallObs("t3", "search")}
	for i := 0; i < 28; i++ {
		list = append(list, Observation{ID: "f" + strings.Repeat("x", i%7) + string(rune('a'+i%26)) + string(rune('0'+i%10)), AgentID: "agent-1", Kind: ObservationKindFeedback, Content: "ok"})
	}
	obs := &obsStubRW{list: list}
	pats := &patternStubRW{}
	uc := &LearningLoopUsecase{obs: obs, patterns: pats, lg: loggateway.NewNoop()}
	patterns, err := uc.DetectPatterns(context.Background(), "agent-1")
	if err != nil {
		t.Fatalf("DetectPatterns: %v", err)
	}
	// tool_call bucket 置信度 3/31 ≈ 0.097 < 0.1 → 跳过；feedback bucket 28/31 ≈ 0.903 → 创建。
	if len(patterns) != 1 || patterns[0].Kind != string(ObservationKindFeedback) {
		t.Errorf("patterns = %+v, want exactly 1 feedback pattern", patterns)
	}
}

// 已存在相同 kind+description 的 detected 模式 → 不重复创建。
func TestDetectPatterns_ExistingDuplicate_NotRecreated(t *testing.T) {
	obs := &obsStubRW{list: []Observation{toolCallObs("o1", "search"), toolCallObs("o2", "search"), toolCallObs("o3", "search")}}
	existing := Pattern{
		ID:          "p-existing",
		AgentID:     "agent-1",
		Kind:        string(ObservationKindToolCall),
		Description: "高频工具调用模式: search(3)",
		Status:      PatternStatusDetected,
	}
	pats := &patternStubRW{existing: []Pattern{existing}}
	uc := &LearningLoopUsecase{obs: obs, patterns: pats, lg: loggateway.NewNoop()}
	patterns, err := uc.DetectPatterns(context.Background(), "agent-1")
	if err != nil {
		t.Fatalf("DetectPatterns: %v", err)
	}
	if len(patterns) != 0 || len(pats.created) != 0 {
		t.Errorf("duplicate pattern must not be recreated: patterns = %d created = %d", len(patterns), len(pats.created))
	}
}

// 置信度 <0.15 的模式不生成提案。
func TestGenerateProposals_BelowConfidenceThreshold_Skipped(t *testing.T) {
	propRW := newMockProposalRW()
	uc := &LearningLoopUsecase{proposals: propRW, lg: loggateway.NewNoop()}
	patterns := []Pattern{
		{ID: "p1", AgentID: "agent-1", Kind: string(ObservationKindToolCall), Confidence: 0.14, Frequency: 3},
	}
	proposals, err := uc.GenerateProposals(context.Background(), "agent-1", patterns)
	if err != nil {
		t.Fatalf("GenerateProposals: %v", err)
	}
	if len(proposals) != 0 {
		t.Errorf("proposals = %d, want 0 for low-confidence pattern", len(proposals))
	}
}

// 模式 kind → 提案 kind 映射：tool_call→prompt, feedback→persona, memory_*→skill。
func TestGenerateProposals_KindMapping(t *testing.T) {
	propRW := newMockProposalRW()
	uc := &LearningLoopUsecase{proposals: propRW, lg: loggateway.NewNoop()}
	patterns := []Pattern{
		{ID: "p1", AgentID: "agent-1", Kind: string(ObservationKindToolCall), Confidence: 0.9, Frequency: 5, Description: "d1"},
		{ID: "p2", AgentID: "agent-1", Kind: string(ObservationKindFeedback), Confidence: 0.9, Frequency: 5, Description: "d2"},
		{ID: "p3", AgentID: "agent-1", Kind: string(ObservationKindMemoryMiss), Confidence: 0.9, Frequency: 5, Description: "d3"},
	}
	proposals, err := uc.GenerateProposals(context.Background(), "agent-1", patterns)
	if err != nil {
		t.Fatalf("GenerateProposals: %v", err)
	}
	if len(proposals) != 3 {
		t.Fatalf("proposals = %d, want 3", len(proposals))
	}
	wantKinds := map[string]string{"p1": "prompt", "p2": "persona", "p3": "skill"}
	for _, prop := range proposals {
		if want := wantKinds[prop.PatternID]; prop.Kind != want {
			t.Errorf("proposal for pattern %s: Kind = %q, want %q", prop.PatternID, prop.Kind, want)
		}
		if prop.Status != ProposalStatusDraft {
			t.Errorf("proposal %s: Status = %q, want draft", prop.ID, prop.Status)
		}
	}
}

// RunLoop 无模式时静默结束，不触碰提案/agents。
func TestRunLoop_NoPatterns_NoOp(t *testing.T) {
	obs := &obsStubRW{list: []Observation{toolCallObs("o1", "search")}}
	uc := &LearningLoopUsecase{obs: obs, patterns: &patternStubRW{}, lg: loggateway.NewNoop()}
	if err := uc.RunLoop(context.Background(), "agent-1"); err != nil {
		t.Fatalf("RunLoop: %v", err)
	}
}

// 端到端：3 条观察 → 模式 → 提案 → 验证 → EvoAutoApply=true → 自动注册为 applied。
func TestRunLoop_AutoApplyOn_ProposalApplied(t *testing.T) {
	obs := &obsStubRW{list: []Observation{toolCallObs("o1", "search"), toolCallObs("o2", "search"), toolCallObs("o3", "search")}}
	pats := &patternStubRW{}
	propRW := newMockProposalRW()
	agents := &learningStubAgents{settings: AgentRuntimeSettings{EvoEnabled: true, EvoAutoApply: true}}
	uc := &LearningLoopUsecase{obs: obs, patterns: pats, proposals: propRW, agents: agents, lg: loggateway.NewNoop()}

	if err := uc.RunLoop(context.Background(), "agent-1"); err != nil {
		t.Fatalf("RunLoop: %v", err)
	}
	if len(pats.created) != 1 {
		t.Fatalf("patterns created = %d, want 1", len(pats.created))
	}
	if len(propRW.byID) != 1 {
		t.Fatalf("proposals created = %d, want 1", len(propRW.byID))
	}
	for _, p := range propRW.byID {
		if p.Status != ProposalStatusApplied {
			t.Errorf("proposal status = %q, want applied (EvoAutoApply=true)", p.Status)
		}
	}
}

// EvoAutoApply=false 时提案停在 validated，等待人工审批。
func TestRunLoop_AutoApplyOff_ProposalStaysValidated(t *testing.T) {
	obs := &obsStubRW{list: []Observation{toolCallObs("o1", "search"), toolCallObs("o2", "search"), toolCallObs("o3", "search")}}
	pats := &patternStubRW{}
	propRW := newMockProposalRW()
	agents := &learningStubAgents{settings: AgentRuntimeSettings{EvoEnabled: true, EvoAutoApply: false}}
	uc := &LearningLoopUsecase{obs: obs, patterns: pats, proposals: propRW, agents: agents, lg: loggateway.NewNoop()}

	if err := uc.RunLoop(context.Background(), "agent-1"); err != nil {
		t.Fatalf("RunLoop: %v", err)
	}
	if len(propRW.byID) != 1 {
		t.Fatalf("proposals created = %d, want 1", len(propRW.byID))
	}
	for _, p := range propRW.byID {
		if p.Status != ProposalStatusValidated {
			t.Errorf("proposal status = %q, want validated (EvoAutoApply=false)", p.Status)
		}
	}
}
