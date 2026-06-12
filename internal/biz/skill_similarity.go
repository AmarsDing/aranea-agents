package biz

import (
	"context"
	"strings"

	"aranea-agents/pkg/loggateway"
)

// SkillSimilarityComparer compares two skills and returns a similarity result.
type SkillSimilarityComparer interface {
	Compare(ctx context.Context, a, b SkillDedupCandidate) (*SimilarityResult, error)
}

// SimilarityDimension 相似度维度
type SimilarityDimension string

const (
	DimName        SimilarityDimension = "name"
	DimDescription SimilarityDimension = "description"
	DimBody        SimilarityDimension = "body"
	DimTag         SimilarityDimension = "tag"
)

// SimilarityWeights 可配置权重
type SimilarityWeights map[SimilarityDimension]float64

// DefaultDedupWeights 平台级去重默认权重
func DefaultDedupWeights() SimilarityWeights {
	return SimilarityWeights{
		DimName:        0.30,
		DimDescription: 0.25,
		DimBody:        0.30,
		DimTag:         0.15,
	}
}

// DimensionScore 单维度评分
type DimensionScore struct {
	Dimension SimilarityDimension
	Score     float64
	Method    string // "jaccard" / "jaccard+embedding"
}

// SimilarityResult 完整相似度结果
type SimilarityResult struct {
	SkillAID       string
	SkillBID       string
	TotalScore     float64
	Dimensions     []DimensionScore
	ConflictRisk   string // "low" / "medium" / "high"
	Recommendation string // "keep_separate" / "suggest_refine" / "block_duplicate"
}

// SkillDedupCandidate 扩展的去重候选项
type SkillDedupCandidate struct {
	ID          string
	Name        string
	Slug        string
	Description string
	BodyPreview string
	Tags        []string
}

// DedupEmbedder 可选的 Embedding 语义相似度接口
type DedupEmbedder interface {
	CosineSimilarity(ctx context.Context, textA, textB string) (float64, error)
}

// Similarity classification thresholds
const (
	similarityHighThreshold     = 0.8  // total score >= this → high risk
	similarityNameHighThreshold = 0.9  // name score >= this → high risk
	similarityMediumThreshold   = 0.5  // total score >= this → medium risk
	embeddingBlendWeight        = 0.5  // Jaccard/Embedding blend ratio for Body dimension
)

// SkillSimilarityEngine 统一相似度引擎
type SkillSimilarityEngine struct {
	embedder DedupEmbedder // 可选
	weights  SimilarityWeights
	lg       loggateway.Logger
}

// NewSkillSimilarityEngine creates a new SkillSimilarityEngine.
func NewSkillSimilarityEngine(embedder DedupEmbedder, weights SimilarityWeights, lg loggateway.Logger) *SkillSimilarityEngine {
	return &SkillSimilarityEngine{embedder: embedder, weights: weights, lg: lg}
}

// Compare 计算两个 Skill 的完整相似度
func (e *SkillSimilarityEngine) Compare(ctx context.Context, a, b SkillDedupCandidate) (*SimilarityResult, error) {
	dims := []DimensionScore{
		{Dimension: DimName, Score: jaccardSimilarity(a.Name, b.Name), Method: "jaccard"},
		{Dimension: DimDescription, Score: jaccardSimilarity(a.Description, b.Description), Method: "jaccard"},
		{Dimension: DimBody, Score: jaccardSimilarity(a.BodyPreview, b.BodyPreview), Method: "jaccard"},
		{Dimension: DimTag, Score: setJaccard(a.Tags, b.Tags), Method: "jaccard"},
	}

	// 如果有 Embedder，用语义相似度增强 Body 维度
	if e.embedder != nil {
		if embScore, err := e.embedder.CosineSimilarity(ctx, a.BodyPreview, b.BodyPreview); err == nil {
			for i := range dims {
				if dims[i].Dimension == DimBody {
					dims[i].Score = (1-embeddingBlendWeight)*dims[i].Score + embeddingBlendWeight*embScore
					dims[i].Method = "jaccard+embedding"
				}
			}
		} else {
			e.lg.Warn("similarity engine: CosineSimilarity failed, using jaccard only",
				loggateway.StepID("skill_similarity.compare"),
				loggateway.Err(err))
		}
	}

	// 加权总分
	totalScore := 0.0
	totalWeight := 0.0
	for _, d := range dims {
		w := e.weights[d.Dimension]
		totalScore += d.Score * w
		totalWeight += w
	}
	if totalWeight > 0 {
		totalScore /= totalWeight
	}

	risk, recommendation := classifySimilarity(totalScore, dims)

	return &SimilarityResult{
		SkillAID:       a.ID,
		SkillBID:       b.ID,
		TotalScore:     totalScore,
		Dimensions:     dims,
		ConflictRisk:   risk,
		Recommendation: recommendation,
	}, nil
}

func classifySimilarity(total float64, dims []DimensionScore) (risk, recommendation string) {
	nameScore := 0.0
	for _, d := range dims {
		if d.Dimension == DimName {
			nameScore = d.Score
			break
		}
	}
	switch {
	case total >= similarityHighThreshold || nameScore >= similarityNameHighThreshold:
		return "high", "block_duplicate"
	case total >= similarityMediumThreshold:
		return "medium", "suggest_refine"
	default:
		return "low", "keep_separate"
	}
}

// setJaccard 计算 string 切片的 Jaccard 相似度
func setJaccard(a, b []string) float64 {
	setA := make(map[string]bool, len(a))
	for _, s := range a {
		setA[strings.ToLower(s)] = true
	}
	setB := make(map[string]bool, len(b))
	for _, s := range b {
		setB[strings.ToLower(s)] = true
	}
	if len(setA) == 0 && len(setB) == 0 {
		return 1.0
	}
	if len(setA) == 0 || len(setB) == 0 {
		return 0.0
	}
	intersection := 0
	for k := range setA {
		if setB[k] {
			intersection++
		}
	}
	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}
