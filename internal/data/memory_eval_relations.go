package data

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

type evalRelationSeed struct {
	entity string
	parent evalCollectedItem
}

func evalRelationSeeds(items []evalCollectedItem) []evalRelationSeed {
	var out []evalRelationSeed
	seen := map[string]bool{}
	count := 0
	for _, env := range items {
		if strings.HasPrefix(env.item.ID, "l2:") || evalIsDerivedRaw(env.raw) {
			continue
		}
		if count >= 8 {
			break
		}
		count++
		for _, entity := range biz.EvalEvidenceEntities(env.item.Content) {
			key := strings.ToLower(entity)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, evalRelationSeed{entity, env})
			if len(out) >= 12 {
				return out
			}
		}
	}
	return out
}

func evalRelationPattern(entity string) string {
	return `(^|[^[:alnum:]_])` + regexp.QuoteMeta(strings.ToLower(entity)) + `([^[:alnum:]_]|$)`
}

func (s *evalMemoryStore) evalRelationRows(ctx context.Context, userID string, seeds []evalRelationSeed, qa biz.EvalQueryAnalysis) ([][]byte, error) {
	args := []any{userID}
	var terms []string
	for _, seed := range seeds {
		if len(seed.entity) == 0 {
			continue
		}
		if seed.entity[0] >= 128 {
			args = append(args, evalLikePattern(seed.entity))
			terms = append(terms, fmt.Sprintf("lower(statement) LIKE $%d", len(args)))
		} else {
			args = append(args, evalRelationPattern(seed.entity))
			terms = append(terms, fmt.Sprintf("lower(statement) ~ $%d", len(args)))
		}
	}
	if len(terms) == 0 {
		return nil, nil
	}
	where := ` WHERE scope_type='user' AND scope_id=$1 AND user_id=$1 AND status='active' AND deleted_at='' AND source_kind NOT IN ('agent_memory_challenge_control','agent_memory_challenge_derived')`
	if !qa.Historical {
		where += ` AND valid_until=''`
	}
	where += ` AND (` + strings.Join(terms, " OR ") + `)`
	q := sqlFactSelectSQL(false) + where + ` ORDER BY CASE WHEN fact_kind IN ('preference','user_preference') THEN 1 ELSE 0 END,created_at DESC,id ASC LIMIT 64`
	rows, err := s.facts.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		raw, err := scanFactRowJSON(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, raw)
	}
	return out, rows.Err()
}

// appendEvalRelations expands literal entity links from gated L3 seeds only.
// Two hops and 64 rows per hop bound work; original text supplies relation
// direction and validity, so a shared entity never becomes an invented edge.
func (s *evalMemoryStore) appendEvalRelations(ctx context.Context, userID, query string, items []evalCollectedItem, qa biz.EvalQueryAnalysis) []evalCollectedItem {
	seen := map[string]bool{}
	for _, env := range items {
		seen[env.item.ID] = true
	}
	frontier := items
	for hop := 0; hop < 2; hop++ {
		seeds := evalRelationSeeds(frontier)
		if len(seeds) == 0 {
			break
		}
		rows, err := s.evalRelationRows(ctx, userID, seeds, qa)
		if err != nil {
			s.lg.Warn("eval entity support skipped", loggateway.StepID("memoryeval.relations"), loggateway.Err(err))
			break
		}
		var next []evalCollectedItem
		for _, raw := range rows {
			env, ok := evalFactEnvelope(raw, 0)
			if !ok || evalIsDerivedRaw(raw) || seen[env.item.ID] || env.historical && !qa.Historical || !biz.EvalEvidenceApplicable(env.item.Content, query) {
				continue
			}
			parent, matched := evalRelationParent(env, seeds)
			if !matched {
				continue
			}
			if qa.RangeEnd != nil && env.day != "" && env.day > qa.RangeEnd.Format("2006-01-02") {
				continue
			}
			env.item.Score = parent.item.Score * 0.85
			env.relevance = parent.relevance * 0.85
			env.support = parent.item.ID
			seen[env.item.ID] = true
			next = append(next, env)
		}
		items = append(items, next...)
		frontier = next
	}
	return items
}

func evalRelationParent(env evalCollectedItem, seeds []evalRelationSeed) (evalCollectedItem, bool) {
	for _, seed := range seeds {
		if seed.entity[0] >= 128 && strings.Contains(env.item.Content, seed.entity) || seed.entity[0] < 128 && regexp.MustCompile(evalRelationPattern(seed.entity)).MatchString(strings.ToLower(env.item.Content)) {
			return seed.parent, true
		}
	}
	return evalCollectedItem{}, false
}

// A linked row never occupies a result slot without the stored rows that led
// to it. If the requested top_k cannot hold a whole chain, omit that branch.
func evalRelationalCapItems(items []evalCollectedItem, topK int32) []evalCollectedItem {
	limit := int(topK)
	if limit <= 0 {
		limit = 100
	}
	byID := make(map[string]evalCollectedItem, len(items))
	for _, item := range items {
		byID[item.item.ID] = item
	}
	selected := map[string]bool{}
	var out []evalCollectedItem
	for _, candidate := range evalMMRCapItems(items, topK) {
		var chain []evalCollectedItem
		path := map[string]bool{}
		valid := true
		for item := candidate; !selected[item.item.ID]; {
			if path[item.item.ID] {
				valid = false
				break
			}
			path[item.item.ID] = true
			chain = append(chain, item)
			if item.support == "" {
				break
			}
			parent, exists := byID[item.support]
			if !exists {
				valid = false
				break
			}
			item = parent
		}
		if !valid || len(out)+len(chain) > limit {
			continue
		}
		for i := len(chain) - 1; i >= 0; i-- {
			selected[chain[i].item.ID] = true
			out = append(out, chain[i])
		}
	}
	// Among complete chains, emit available roots/children in event order.
	// Parent evidence always precedes its support row, even with odd source dates.
	var ordered []evalCollectedItem
	emitted := map[string]bool{}
	for len(out) > 0 {
		best := -1
		for i, env := range out {
			if env.support != "" && !emitted[env.support] {
				continue
			}
			if best < 0 || env.day != "" && (out[best].day == "" || env.day < out[best].day) {
				best = i
			}
		}
		if best < 0 {
			break
		}
		ordered = append(ordered, out[best])
		emitted[out[best].item.ID] = true
		out = append(out[:best], out[best+1:]...)
	}
	return ordered
}
