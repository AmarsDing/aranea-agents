package data

import (
	"sort"
	"strings"

	"aranea-agents/internal/biz"
)

// evalEvidenceUnits joins only atoms from the same stored source message.
// Rules, exceptions and demonstrations remain an ordered evidence unit even
// at top_k=1. Historical states retain separate validity labels.
func evalEvidenceUnits(items []evalCollectedItem, qa biz.EvalQueryAnalysis) []evalCollectedItem {
	if !qa.Procedural && !qa.Aggregate || qa.Chronological || qa.Relational {
		return items
	}
	type source struct{ session, message, timestamp string }
	groups := map[source][]evalCollectedItem{}
	var keys []source
	var out []evalCollectedItem
	for _, env := range items {
		message := factJSONString(env.raw, "source_message_id")
		if message == "" || env.invalidUntil != "" || env.historical {
			out = append(out, env)
			continue
		}
		key := source{env.sess, message, env.sourceTime}
		if len(groups[key]) == 0 {
			keys = append(keys, key)
		}
		groups[key] = append(groups[key], env)
	}
	for _, key := range keys {
		members := groups[key]
		sort.SliceStable(members, func(i, j int) bool { return members[i].atomIndex < members[j].atomIndex })
		unit := members[0]
		var lines []string
		for _, member := range members {
			lines = append(lines, member.item.Content)
			if member.item.Score > unit.item.Score {
				unit.item.Score = member.item.Score
			}
			if member.relevance > unit.relevance {
				unit.relevance = member.relevance
			}
		}
		unit.item.Content = strings.Join(lines, "\n")
		unit.evidenceUnit = unit.evidenceUnit || len(members) > 1
		out = append(out, unit)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].item.Score != out[j].item.Score {
			return out[i].item.Score > out[j].item.Score
		}
		return out[i].item.ID < out[j].item.ID
	})
	return out
}

// A full L2 transcript that repeats already retrieved L3 atoms consumes
// context without adding evidence. Occurrences in L3 remain source-specific.
func evalRemoveCoveredEpisodes(items []evalCollectedItem) []evalCollectedItem {
	covered := map[string]map[string]bool{}
	for _, env := range items {
		if strings.HasPrefix(env.item.ID, "l2:") {
			continue
		}
		if covered[env.sess] == nil {
			covered[env.sess] = map[string]bool{}
		}
		for _, line := range strings.Split(env.item.Content, "\n") {
			covered[env.sess][strings.TrimSpace(line)] = true
		}
	}
	out := items[:0]
	for _, env := range items {
		if strings.HasPrefix(env.item.ID, "l2:") {
			all := true
			for _, line := range strings.Split(env.item.Content, "\n") {
				if line = strings.TrimSpace(line); line != "" && !covered[env.sess][line] {
					all = false
					break
				}
			}
			if all {
				continue
			}
		}
		out = append(out, env)
	}
	return out
}

// A global summary gets at least one relevant unit per source before a dense
// source gets another. If there are more sources than slots, broad summaries
// sample the available history evenly, retaining both ends of that history.
func evalCoverageCapItems(items []evalCollectedItem, topK int32, broad bool) []evalCollectedItem {
	lim := int(topK)
	if lim <= 0 {
		lim = 100
	}
	var first, rest []evalCollectedItem
	seen := map[string]bool{}
	for _, env := range items {
		key := env.sess
		if key == "" {
			key = env.item.ID
		}
		if seen[key] {
			rest = append(rest, env)
		} else {
			first = append(first, env)
			seen[key] = true
		}
	}
	if len(first) > lim && broad {
		sort.SliceStable(first, func(i, j int) bool {
			a, b := first[i].sourceTime, first[j].sourceTime
			if a == "" {
				a = first[i].item.CreatedAt
			}
			if b == "" {
				b = first[j].item.CreatedAt
			}
			if a != b {
				return a < b
			}
			return first[i].item.ID < first[j].item.ID
		})
		out := make([]evalCollectedItem, 0, lim)
		for i := 0; i < lim; i++ {
			index := 0
			if lim > 1 {
				index = i * (len(first) - 1) / (lim - 1)
			}
			out = append(out, first[index])
		}
		return out
	}
	if len(first) >= lim {
		return first[:lim]
	}
	// Cover distinct evidence types before another near-duplicate fact from
	// a dense source consumes the remaining summary slots.
	classes := map[string]bool{}
	for _, env := range first {
		classes[evalEvidenceClass(env)] = true
	}
	var remainingRows []evalCollectedItem
	for _, env := range rest {
		class := evalEvidenceClass(env)
		if len(first) < lim && !classes[class] {
			first = append(first, env)
			classes[class] = true
		} else {
			remainingRows = append(remainingRows, env)
		}
	}
	rest = remainingRows
	remaining := lim - len(first)
	if remaining <= 0 {
		return first
	}
	if len(rest) > remaining {
		rest = evalMMRCapItems(rest, int32(remaining))
	}
	return append(first, rest...)
}
