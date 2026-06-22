package biz

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"aranea-agents/pkg/loggateway"
)

const (
	dedupSimilarityThreshold = 0.5 // raised from 0.3: Jaccard on short text is naturally low, 0.3 caused too many false positives
	dedupBodyPreviewLen      = 500
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// SkillDuplicateGroup represents a group of similar skills that may be duplicates.
type SkillDuplicateGroup struct {
	GroupID        string
	Skills         []SkillSummary
	OverlapType    string           // "name_similarity" / "description_similarity" / "invocation_overlap"
	OverlapScore   float64          // 0-1, higher = more similar
	Dimensions     []DimensionScore // 维度明细
	ConflictRisk   string           // "low" / "medium" / "high"
	Recommendation string           // "keep_separate" / "suggest_refine" / "block_duplicate"
}

// SkillSummary is a lightweight skill representation for dedup comparison.
type SkillSummary struct {
	ID          string
	Name        string
	Slug        string
	Description string
	BodyPreview string // first 500 chars of skill body
	Tags        []string
}

// ---------------------------------------------------------------------------
// Repo interfaces
// ---------------------------------------------------------------------------

// SkillDedupReader reads skill data for dedup analysis.
type SkillDedupReader interface {
	ListAllSkillSummaries(ctx context.Context) ([]SkillSummary, error)
}

// ---------------------------------------------------------------------------
// Usecase
// ---------------------------------------------------------------------------

// SkillDedupUsecase detects duplicate skills.
type SkillDedupUsecase struct {
	reader SkillDedupReader
	engine SkillSimilarityComparer
	lg     loggateway.Logger

	cacheMu      sync.RWMutex
	cachedGroups []SkillDuplicateGroup
	cachedAt     time.Time
	cacheTTL     time.Duration
}

// NewSkillDedupUsecase creates a new SkillDedupUsecase.
func NewSkillDedupUsecase(reader SkillDedupReader, engine SkillSimilarityComparer, lg loggateway.Logger) *SkillDedupUsecase {
	return &SkillDedupUsecase{
		reader:   reader,
		engine:   engine,
		lg:       lg,
		cacheTTL: 10 * time.Minute,
	}
}

// DetectDuplicateGroups finds groups of similar skills using the unified similarity engine.
func (uc *SkillDedupUsecase) DetectDuplicateGroups(ctx context.Context) ([]SkillDuplicateGroup, error) {
	// Default 5-minute timeout to prevent runaway comparisons.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	// Check cache first.
	uc.cacheMu.RLock()
	if !uc.cachedAt.IsZero() && time.Since(uc.cachedAt) < uc.cacheTTL && uc.cachedGroups != nil {
		cached := uc.cachedGroups
		uc.cacheMu.RUnlock()
		return cached, nil
	}
	uc.cacheMu.RUnlock()

	summaries, err := uc.reader.ListAllSkillSummaries(ctx)
	if err != nil {
		return nil, err
	}
	if len(summaries) < 2 {
		return nil, nil
	}

	// Convert SkillSummary to SkillDedupCandidate for the engine.
	candidates := make([]SkillDedupCandidate, len(summaries))
	for i, s := range summaries {
		candidates[i] = SkillDedupCandidate{
			ID:          s.ID,
			Name:        s.Name,
			Slug:        s.Slug,
			Description: s.Description,
			BodyPreview: s.BodyPreview,
			Tags:        s.Tags,
		}
	}

	// Pairwise comparison using the unified similarity engine with batch processing.
	type pair struct {
		i, j   int
		result *SimilarityResult
	}

	const batchSize = 200
	var pairs []pair
	for batchStart := 0; batchStart < len(candidates); batchStart += batchSize {
		// Check context cancellation between batches.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		batchEnd := batchStart + batchSize
		if batchEnd > len(candidates) {
			batchEnd = len(candidates)
		}
		for i := batchStart; i < batchEnd; i++ {
			for j := i + 1; j < len(candidates); j++ {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				default:
				}
				res, err := uc.engine.Compare(ctx, candidates[i], candidates[j])
				if err != nil {
					// If context was cancelled, return immediately rather than continuing.
					if ctx.Err() != nil {
						return nil, ctx.Err()
					}
					uc.lg.Warn("skill_dedup: Compare failed",
						loggateway.StepID("skill_dedup.compare"),
						loggateway.Err(err))
					continue
				}
				if res.TotalScore >= dedupSimilarityThreshold {
					pairs = append(pairs, pair{i: i, j: j, result: res})
				}
			}
		}
	}

	if len(pairs) == 0 {
		return nil, nil
	}

	// Group connected pairs using union-find.
	uf := newUnionFind(len(candidates))

	// Track overlap info per group root — aggregate all overlap types.
	type groupMeta struct {
		overlapTypes map[string]bool
		bestScore    float64
		bestDims     []DimensionScore
		bestRisk     string
		bestRec      string
	}
	groupInfo := make(map[int]*groupMeta)

	for _, p := range pairs {
		uf.Union(p.i, p.j)
		root := uf.Find(p.i)
		meta := groupInfo[root]
		if meta == nil {
			meta = &groupMeta{overlapTypes: map[string]bool{}, bestScore: 0}
			groupInfo[root] = meta
		}
		// Determine overlap type from dimensions.
		for _, d := range p.result.Dimensions {
			if d.Score >= dedupSimilarityThreshold {
				meta.overlapTypes[string(d.Dimension)+"_similarity"] = true
			}
		}
		if p.result.TotalScore > meta.bestScore {
			meta.bestScore = p.result.TotalScore
			meta.bestDims = p.result.Dimensions
			meta.bestRisk = p.result.ConflictRisk
			meta.bestRec = p.result.Recommendation
		}
	}

	// Collect groups.
	buckets := make(map[int][]int)
	for i := range candidates {
		root := uf.Find(i)
		buckets[root] = append(buckets[root], i)
	}

	var groups []SkillDuplicateGroup
	groupSeq := 0
	for root, indices := range buckets {
		if len(indices) < 2 {
			continue
		}
		meta := groupInfo[root]
		var skills []SkillSummary
		for _, idx := range indices {
			skills = append(skills, summaries[idx])
		}
		// Join all overlap types for the group.
		overlapTypes := make([]string, 0, len(meta.overlapTypes))
		for t := range meta.overlapTypes {
			overlapTypes = append(overlapTypes, t)
		}
		groups = append(groups, SkillDuplicateGroup{
			GroupID:        fmt.Sprintf("dedup-%03d", groupSeq),
			Skills:         skills,
			OverlapType:    strings.Join(overlapTypes, "+"),
			OverlapScore:   meta.bestScore,
			Dimensions:     meta.bestDims,
			ConflictRisk:   meta.bestRisk,
			Recommendation: meta.bestRec,
		})
		groupSeq++
	}

	uc.lg.Info("skill dedup detection completed",
		loggateway.StepID("skill_dedup.detect"),
		loggateway.Str("group_count", fmt.Sprintf("%d", len(groups))),
	)

	// Update cache.
	uc.cacheMu.Lock()
	uc.cachedGroups = groups
	uc.cachedAt = time.Now()
	uc.cacheMu.Unlock()

	return groups, nil
}

// InvalidateDedupCache clears the cached duplicate groups so the next call
// to DetectDuplicateGroups performs a fresh scan. Call this after skill
// mutations (create, update, delete, merge) that may change dedup results.
func (uc *SkillDedupUsecase) InvalidateDedupCache() {
	uc.cacheMu.Lock()
	uc.cachedGroups = nil
	uc.cachedAt = time.Time{}
	uc.cacheMu.Unlock()
}

// ---------------------------------------------------------------------------
// Similarity helpers
// ---------------------------------------------------------------------------

func jaccardSimilarity(a, b string) float64 {
	setA := wordSet(a)
	setB := wordSet(b)
	if len(setA) == 0 && len(setB) == 0 {
		return 0.0 // cannot determine similarity from empty fields
	}
	if len(setA) == 0 || len(setB) == 0 {
		return 0.0
	}
	intersection := 0
	for w := range setA {
		if setB[w] {
			intersection++
		}
	}
	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

// wordSet extracts a set of tokens from s for Jaccard similarity comparison.
// For English text, it splits on whitespace. For Chinese text (CJK Unified
// Ideographs), it uses bigram (consecutive character pair) tokenization so
// that "数据库查询" produces {"数据", "据库", "库查", "查询"} instead of
// treating the entire string as a single token.
func wordSet(s string) map[string]bool {
	set := make(map[string]bool)
	lower := strings.ToLower(s)

	// Extract English words (alphanumeric + underscore/dash).
	var engBuf strings.Builder
	for _, r := range lower {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			engBuf.WriteRune(r)
		} else {
			if engBuf.Len() > 1 {
				set[engBuf.String()] = true
			}
			engBuf.Reset()
		}
	}
	if engBuf.Len() > 1 {
		set[engBuf.String()] = true
	}

	// Extract Chinese bigrams (CJK Unified Ideographs range).
	var prev rune
	for _, r := range lower {
		if r >= 0x4e00 && r <= 0x9fff {
			if prev >= 0x4e00 && prev <= 0x9fff {
				set[string([]rune{prev, r})] = true
			}
			prev = r
		} else {
			prev = 0
		}
	}
	return set
}

// unionFind implements a disjoint-set data structure with path compression
// and union by rank for grouping similar skills.
type unionFind struct {
	parent []int
	rank   []int
}

func newUnionFind(n int) *unionFind {
	uf := &unionFind{
		parent: make([]int, n),
		rank:   make([]int, n),
	}
	for i := range uf.parent {
		uf.parent[i] = i
	}
	return uf
}

func (uf *unionFind) Find(x int) int {
	root := x
	for uf.parent[root] != root {
		root = uf.parent[root]
	}
	// Path compression.
	for x != root {
		next := uf.parent[x]
		uf.parent[x] = root
		x = next
	}
	return root
}

func (uf *unionFind) Union(a, b int) {
	ra, rb := uf.Find(a), uf.Find(b)
	if ra == rb {
		return
	}
	// Union by rank.
	if uf.rank[ra] < uf.rank[rb] {
		ra, rb = rb, ra
	}
	uf.parent[rb] = ra
	if uf.rank[ra] == uf.rank[rb] {
		uf.rank[ra]++
	}
}
