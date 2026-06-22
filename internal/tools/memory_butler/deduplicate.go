package memory_butler

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/jsonutil"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

type deduplicateMemoriesInput struct {
	AgentID      string  `json:"agent_id" jsonschema:"description=Agent ID,required"`
	SimThreshold float64 `json:"sim_threshold" jsonschema:"description=相似度阈值,minimum=0,maximum=1,default=0.8"`
}

type deduplicateMemoriesOutput struct {
	MergedCount int `json:"merged_count"`
}

func newDeduplicateMemoriesTool(deps Deps) trpctool.Tool {
	execute := func(ctx context.Context, input deduplicateMemoriesInput) (deduplicateMemoriesOutput, error) {
		if input.AgentID == "" {
			return deduplicateMemoriesOutput{}, ErrAgentIDRequired
		}
		threshold := input.SimThreshold
		if threshold <= 0 {
			threshold = defaultSimilarityThreshold
		}

		rows, _, _, _, err := deps.MemoryAdmin.ListFactRows(ctx, biz.ListFactRowsParams{
			ScopeType: "agent",
			ScopeID:   input.AgentID,
			Limit:     defaultFactListLimit,
			Offset:    0,
		})
		if err != nil {
			return deduplicateMemoriesOutput{}, err
		}

		type factEntry struct {
			ID        string
			Statement string
			UpdatedAt string
		}
		var facts []factEntry
		for _, raw := range rows {
			m, _ := jsonutil.ParseMap(raw)
			if m == nil {
				continue
			}
			id := jsonutil.IfaceStr(m, "id")
			stmt := jsonutil.IfaceStr(m, "statement")
			updated := jsonutil.IfaceStr(m, "updated_at")
			if id == "" || stmt == "" {
				continue
			}
			facts = append(facts, factEntry{ID: id, Statement: stmt, UpdatedAt: updated})
		}

		// Identify duplicates using simplified string-based similarity.
		// TODO(P1): Replace with embedding-based cosine similarity.
		var toDelete []string
		seen := make(map[string]bool)
		for i := 0; i < len(facts); i++ {
			if seen[facts[i].ID] {
				continue
			}
			for j := i + 1; j < len(facts); j++ {
				if seen[facts[j].ID] {
					continue
				}
				if stringSimilarity(facts[i].Statement, facts[j].Statement) >= threshold {
					// Keep the newer one, delete the older one.
					if facts[i].UpdatedAt >= facts[j].UpdatedAt {
						toDelete = append(toDelete, facts[j].ID)
						seen[facts[j].ID] = true
					} else {
						toDelete = append(toDelete, facts[i].ID)
						seen[facts[i].ID] = true
						break
					}
				}
			}
		}

		if len(toDelete) == 0 {
			return deduplicateMemoriesOutput{MergedCount: 0}, nil
		}

		deleted, err := deps.MemoryAdmin.DeleteFactRowsByIDs(ctx, toDelete)
		if err != nil {
			return deduplicateMemoriesOutput{}, err
		}
		return deduplicateMemoriesOutput{MergedCount: deleted}, nil
	}

	return function.NewFunctionTool(
		execute,
		function.WithName("memory_butler_deduplicate_memories"),
		function.WithDescription("去重记忆：识别并合并语义高度相似的记忆条目，保留较新的版本，删除较旧的重复项。"),
	)
}

// stringSimilarity computes a simplified overlap ratio between two strings.
// It returns the ratio of shared trigrams to the total unique trigrams.
func stringSimilarity(a, b string) float64 {
	aLower := strings.ToLower(a)
	bLower := strings.ToLower(b)
	if aLower == bLower {
		return 1.0
	}
	if len(aLower) < 3 || len(bLower) < 3 {
		// For very short strings, use simple containment check.
		if strings.Contains(aLower, bLower) || strings.Contains(bLower, aLower) {
			minLen := float64(min(len(aLower), len(bLower)))
			maxLen := float64(max(len(aLower), len(bLower)))
			return minLen / maxLen
		}
		return 0
	}

	aTrigrams := trigramSet(aLower)
	bTrigrams := trigramSet(bLower)

	var shared int
	for k := range aTrigrams {
		if bTrigrams[k] {
			shared++
		}
	}
	total := len(aTrigrams) + len(bTrigrams) - shared
	if total == 0 {
		return 0
	}
	return float64(shared) / float64(total)
}

func trigramSet(s string) map[string]bool {
	set := make(map[string]bool)
	runes := []rune(s)
	for i := 0; i+3 <= len(runes); i++ {
		set[string(runes[i:i+3])] = true
	}
	return set
}
