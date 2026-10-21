package data

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"aranea-agents/pkg/loggateway"

	"github.com/lib/pq"
)

// l3TrgmScopePrefilterLimit caps the scope-first candidate window the trigram
// operator runs on. The scope predicates (scope_type/scope_id/user_id +
// active/undeleted/currently-valid) already isolate one tenant's working
// memory; 2000 rows comfortably covers 单用户评测规模（百级事实）乃至长记忆
// 生产单 scope 的工作集——超出时取 created_at 最新 2000 行，再由外层
// word_similarity 重排，最坏代价与表总量脱钩。
const l3TrgmScopePrefilterLimit = 2000

// searchL3Trigram returns trigram-ranked fact IDs (best first) for CJK and
// short queries. FTS 'simple' treats a Chinese run as one token, so
// word_similarity (query trigrams vs a contiguous span of statement) is the
// third recall channel — same operator knowledge / agent-case already use.
//
// 2026-10-20 生产事故治本（AMC C3-B）：旧 SQL 把 statement %> / word_similarity
// 直接挂在全表 WHERE/ORDER BY 上，GIN trgm 索引在 38 万行/8.2GB memory_facts
// 上走全量候选扫描再排序，32 并发单条 10-16 分钟拖死全站。现改为 scope 先行：
// CTE 先用 status/deleted_at/valid_until + (scope_type, scope_id) 复合索引
// 等值谓词 + user_id 过滤把候选收敛到 created_at 最新 2000 行，%> 与
// word_similarity 只在物化子集上求值（MATERIALIZED 阻止 CTE 内联——否则
// 规划器可能把 %> 下推回全表 GIN 路径，复现事故）。
// 占位符按文本顺序编号（Dialect().RenumberPlaceholders 逐 '?' 递增）。
func (r *l3FactRepo) searchL3Trigram(ctx context.Context, scopeType, scopeID, userID, query string, limit int) ([]string, error) {
	query = strings.TrimSpace(query)
	if r == nil || r.data == nil {
		return nil, nil
	}
	if utf8.RuneCountInString(query) < 2 {
		return nil, nil
	}
	if limit <= 0 {
		limit = l3FTSCandidateLimit
	}
	q := `WITH scoped AS MATERIALIZED (
  SELECT id, statement, valid_from, created_at FROM memory_facts
  WHERE status = 'active' AND deleted_at = '' AND valid_until = ''
    AND (? = '' OR scope_type = ?)
    AND (? = '' OR scope_id = ?)
    AND (? = '' OR user_id = ?)
  ORDER BY created_at DESC
  LIMIT ?
)
SELECT id FROM scoped
WHERE statement %> ?
ORDER BY word_similarity(?, statement) DESC, COALESCE(NULLIF(valid_from, ''), created_at) DESC
LIMIT ?`
	args := []any{scopeType, scopeType, scopeID, scopeID, userID, userID,
		l3TrgmScopePrefilterLimit, query, query, limit}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		if isMissingTrigramOp(err) {
			return nil, nil
		}
		return nil, entErrToBizErr(err, "MEMORY_L3")
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if scanErr := rows.Scan(&id); scanErr != nil {
			return nil, entErrToBizErr(scanErr, "MEMORY_L3")
		}
		ids = append(ids, id)
	}
	return ids, entErrToBizErr(rows.Err(), "MEMORY_L3")
}

func isMissingTrigramOp(err error) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		switch string(pqErr.Code) {
		case "42883", "42704", "42809": // undefined_function / undefined_object / wrong_object_type
			return true
		}
	}
	return false
}

// trgmCandidateIDs is the best-effort wrapper around searchL3Trigram.
func (r *l3FactRepo) trgmCandidateIDs(ctx context.Context, scopeType, scopeID, userID, query string, limit int) []string {
	ids, err := r.searchL3Trigram(ctx, scopeType, scopeID, userID, query, limit)
	if err != nil {
		r.data.lg.Warn("L3 trigram candidate search failed, degrading to non-trgm recall",
			loggateway.StepID("memory.l3_trgm_search"),
			loggateway.Err(err))
		return nil
	}
	return ids
}

func (r *l3FactRepo) trgmExtraCandidates(ctx context.Context, scopeType, scopeID, userID, query string, queryEmbedding []float32, present map[string]struct{}, minScore float64, now time.Time) ([]scoredFact, error) {
	return r.trgmExtraCandidatesAt(ctx, scopeType, scopeID, userID, query, queryEmbedding, present, minScore, now, l3FTSCandidateLimit)
}

func (r *l3FactRepo) trgmExtraCandidatesAt(ctx context.Context, scopeType, scopeID, userID, query string, queryEmbedding []float32, present map[string]struct{}, minScore float64, now time.Time, limit int) ([]scoredFact, error) {
	if limit <= 0 {
		limit = l3FTSCandidateLimit
	}
	ids := r.trgmCandidateIDs(ctx, scopeType, scopeID, userID, query, limit)
	if len(ids) == 0 {
		return nil, nil
	}
	missing := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, ok := present[id]; !ok {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		return nil, nil
	}
	rows, err := r.queryFactRowsByIDs(ctx, missing, scopeType, scopeID, userID, len(queryEmbedding) > 0)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L3")
	}
	defer rows.Close()
	scored := scoreFactRows(rows, tokenizeQuery(query), queryEmbedding, nil, minScore, now)
	if err := rows.Err(); err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L3")
	}
	return scored, nil
}
