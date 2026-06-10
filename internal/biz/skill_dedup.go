package biz

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	"aranea-agents/pkg/loggateway"
)

const (
	dedupSimilarityThreshold = 0.3 // raised from 0.2 to reduce false positives
	dedupBodyPreviewLen      = 500
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// SkillDuplicateGroup represents a group of similar skills that may be duplicates.
type SkillDuplicateGroup struct {
	GroupID        string
	Skills         []SkillSummary
	OverlapType    string            // "name_similarity" / "description_similarity" / "invocation_overlap"
	OverlapScore   float64           // 0-1, higher = more similar
	Dimensions     []DimensionScore  // 维度明细
	ConflictRisk   string            // "low" / "medium" / "high"
	Recommendation string            // "keep_separate" / "suggest_refine" / "block_duplicate"
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

// SkillDedupWriter writes dedup results.
type SkillDedupWriter interface {
	DeprecateSkill(ctx context.Context, skillID string, reason string) error
	TransferInvocations(ctx context.Context, fromSkillID string, toSkillID string) error
}

// ---------------------------------------------------------------------------
// Usecase
// ---------------------------------------------------------------------------

// SkillDedupUsecase detects and merges duplicate skills.
type SkillDedupUsecase struct {
	reader SkillDedupReader
	writer SkillDedupWriter
	engine *SkillSimilarityEngine
	lg     loggateway.Logger

	cacheMu      sync.RWMutex
	cachedGroups []SkillDuplicateGroup
	cachedAt     time.Time
	cacheTTL     time.Duration
}

// NewSkillDedupUsecase creates a new SkillDedupUsecase.
func NewSkillDedupUsecase(reader SkillDedupReader, writer SkillDedupWriter, engine *SkillSimilarityEngine, lg loggateway.Logger) *SkillDedupUsecase {
	return &SkillDedupUsecase{
		reader:   reader,
		writer:   writer,
		engine:   engine,
		lg:       lg,
		cacheTTL: 10 * time.Minute,
	}
}

// DetectDuplicateGroups finds groups of similar skills using the unified similarity engine.
func (uc *SkillDedupUsecase) DetectDuplicateGroups(ctx context.Context) ([]SkillDuplicateGroup, error) {
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

	// Pairwise comparison using the unified similarity engine.
	type pair struct {
		i, j       int
		result     *SimilarityResult
	}

	var pairs []pair
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			res, err := uc.engine.Compare(ctx, candidates[i], candidates[j])
			if err != nil {
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

	if len(pairs) == 0 {
		return nil, nil
	}

	// Group connected pairs using union-find.
	parent := make([]int, len(candidates))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(x int) int {
		if parent[x] != x {
			parent[x] = find(parent[x])
		}
		return parent[x]
	}
	union := func(a, b int) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[ra] = rb
		}
	}

	// Track overlap info per group root — aggregate all overlap types.
	type groupMeta struct {
		overlapTypes  map[string]bool
		bestScore     float64
		bestDims      []DimensionScore
		bestRisk      string
		bestRec       string
	}
	groupInfo := make(map[int]*groupMeta)

	for _, p := range pairs {
		union(p.i, p.j)
		root := find(p.i)
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
		root := find(i)
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

// MergeSkills merges source skill into target skill.
// Source skill is deprecated, invocations are transferred.
// If deprecation fails after transfer, the transfer is NOT rolled back
// (invocations remain on target) but an error is returned so the caller
// knows the source is still active.
//
// Deprecated: Use SkillMergeUsecase.Merge instead. The new method provides
// transactional ApplyMerge with content fusion and Gate validation.
func (uc *SkillDedupUsecase) MergeSkills(ctx context.Context, sourceID string, targetID string) error {
	sourceID, err := requireNonEmpty(sourceID, "SKILL_DEDUP", "source_id")
	if err != nil {
		return err
	}
	targetID, err = requireNonEmpty(targetID, "SKILL_DEDUP", "target_id")
	if err != nil {
		return err
	}
	if sourceID == targetID {
		return kerrors.BadRequest("SKILL_DEDUP", "source and target must be different skills")
	}

	// Transfer invocations from source to target.
	if tErr := uc.writer.TransferInvocations(ctx, sourceID, targetID); tErr != nil {
		return tErr
	}

	// Deprecate the source skill.
	reason := fmt.Sprintf("merged into skill %s", targetID)
	if dErr := uc.writer.DeprecateSkill(ctx, sourceID, reason); dErr != nil {
		uc.lg.Warn("MergeSkills: TransferInvocations succeeded but DeprecateSkill failed; invocations remain on target",
			loggateway.StepID("skill_dedup.merge"),
			loggateway.Str("source_id", sourceID),
			loggateway.Str("target_id", targetID),
			loggateway.Err(dErr))
		return dErr
	}

	uc.lg.Info("skill merged",
		loggateway.StepID("skill_dedup.merge"),
		loggateway.Str("source_id", sourceID),
		loggateway.Str("target_id", targetID),
	)
	return nil
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
		return 1.0
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
