package biz

import (
	"context"
	"fmt"
	"strings"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	"aranea-agents/pkg/loggateway"
)

const (
	dedupSimilarityThreshold = 0.2
	dedupBodyPreviewLen      = 500
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// SkillDuplicateGroup represents a group of similar skills that may be duplicates.
type SkillDuplicateGroup struct {
	GroupID      string
	Skills       []SkillSummary
	OverlapType  string  // "name_similarity" / "description_similarity" / "invocation_overlap"
	OverlapScore float64 // 0-1, higher = more similar
}

// SkillSummary is a lightweight skill representation for dedup comparison.
type SkillSummary struct {
	ID          string
	Name        string
	Slug        string
	Description string
	BodyPreview string // first 500 chars of skill body
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
	lg     loggateway.Logger
}

// NewSkillDedupUsecase creates a new SkillDedupUsecase.
func NewSkillDedupUsecase(reader SkillDedupReader, writer SkillDedupWriter, lg loggateway.Logger) *SkillDedupUsecase {
	return &SkillDedupUsecase{
		reader: reader,
		writer: writer,
		lg:     lg,
	}
}

// DetectDuplicateGroups finds groups of similar skills.
// Similarity threshold: description similarity >= 0.2
func (uc *SkillDedupUsecase) DetectDuplicateGroups(ctx context.Context) ([]SkillDuplicateGroup, error) {
	summaries, err := uc.reader.ListAllSkillSummaries(ctx)
	if err != nil {
		return nil, err
	}
	if len(summaries) < 2 {
		return nil, nil
	}

	// Pairwise comparison using Jaccard similarity on description word sets.
	type pair struct {
		i, j       int
		similarity float64
		overlap    string
	}

	var pairs []pair
	for i := 0; i < len(summaries); i++ {
		for j := i + 1; j < len(summaries); j++ {
			a, b := summaries[i], summaries[j]

			// Check name similarity first (higher priority).
			nameSim := jaccardSimilarity(a.Name, b.Name)
			if nameSim >= dedupSimilarityThreshold {
				pairs = append(pairs, pair{i: i, j: j, similarity: nameSim, overlap: "name_similarity"})
				continue
			}

			// Fall back to description similarity.
			descSim := jaccardSimilarity(a.Description, b.Description)
			if descSim >= dedupSimilarityThreshold {
				pairs = append(pairs, pair{i: i, j: j, similarity: descSim, overlap: "description_similarity"})
			}
		}
	}

	if len(pairs) == 0 {
		return nil, nil
	}

	// Group connected pairs using union-find.
	parent := make([]int, len(summaries))
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

	// Track best overlap info per group root.
	type groupMeta struct {
		overlapType  string
		bestScore    float64
	}
	groupInfo := make(map[int]groupMeta)

	for _, p := range pairs {
		union(p.i, p.j)
		root := find(p.i)
		meta, ok := groupInfo[root]
		if !ok || p.similarity > meta.bestScore {
			groupInfo[root] = groupMeta{overlapType: p.overlap, bestScore: p.similarity}
		}
	}

	// Collect groups.
	buckets := make(map[int][]int)
	for i := range summaries {
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
		groups = append(groups, SkillDuplicateGroup{
			GroupID:      fmt.Sprintf("dedup-%03d", groupSeq),
			Skills:       skills,
			OverlapType:  meta.overlapType,
			OverlapScore: meta.bestScore,
		})
		groupSeq++
	}

	uc.lg.Info("skill dedup detection completed",
		loggateway.StepID("skill_dedup.detect"),
		loggateway.Str("group_count", fmt.Sprintf("%d", len(groups))),
	)
	return groups, nil
}

// MergeSkills merges source skill into target skill.
// Source skill is deprecated, invocations are transferred.
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
		return dErr
	}

	uc.lg.Info("skill merged",
		loggateway.StepID("skill_dedup.merge"),
		loggateway.Str("source_id", sourceID),
		loggateway.Str("target_id", targetID),
	)
	return nil
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

func wordSet(s string) map[string]bool {
	words := strings.Fields(strings.ToLower(s))
	set := make(map[string]bool, len(words))
	for _, w := range words {
		set[w] = true
	}
	return set
}
