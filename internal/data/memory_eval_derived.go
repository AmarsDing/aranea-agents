package data

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

const (
	evalIntentRouterRetry     = 5 * time.Minute
	evalDigestKeyLook         = 8
	evalRangeFactListLimit    = 2048
	evalLinkHopIDCap          = 24
	evalRelationalHopIDBudget = 30
)

type evalUserWriteLock struct {
	mu sync.Mutex
	m  map[string]*sync.Mutex
}

// lock takes the user's write lock and returns the unlock func.
func (l *evalUserWriteLock) lock(id string) func() {
	l.mu.Lock()
	if l.m == nil {
		l.m = map[string]*sync.Mutex{}
	}
	lk, ok := l.m[id]
	if !ok {
		lk = &sync.Mutex{}
		l.m[id] = lk
	}
	l.mu.Unlock()
	lk.Lock()
	return lk.Unlock
}

// intentRouter lazily builds the VIR vector intent router (AMC P3) on the
// store's embedder — anchors and statement embeddings share one vector space.
// A nil result means the router is unavailable (no embedder, or an outage
// inside the retry window): callers then degrade to pre-router safe behavior
// (no query modes, no vector-governed supersede; same-slot通道不受影响).
func (s *evalMemoryStore) intentRouter(ctx context.Context) *biz.EvalIntentRouter {
	if s.emb == nil {
		return nil
	}
	s.routerMu.Lock()
	defer s.routerMu.Unlock()
	if s.router != nil {
		return s.router
	}
	if !s.routerErrAt.IsZero() && time.Since(s.routerErrAt) < evalIntentRouterRetry {
		return nil
	}
	r, err := biz.NewEvalIntentRouter(ctx, s.emb)
	if err != nil {
		// 评审 #5：调用方请求被取消/超时不等于嵌入上游故障——不盖退避
		// 时间戳，让下一个请求立即重试，避免一次冷启动取消把路由毒化
		// 整个 evalIntentRouterRetry 窗口。
		if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil
		}
		s.routerErrAt = time.Now()
		s.lg.Warn("eval intent router build failed, intent routing disabled",
			loggateway.StepID("memoryeval.intent_router"), loggateway.Err(err))
		return nil
	}
	s.router = r
	return s.router
}

// embedEvalStatements embeds one message's atoms in BOTH forms within a
// single batch round-trip (评测流一条消息多枚原子 → 2n 文本一次往返；无批
// 量支持时逐条)：
//   - prefixed：带角色前缀的完整语句，与存量事实向量同一文本分布，供
//     0.82 向量改口通道比对；
//   - routed：stripEvalRolePrefix 之后的裸文本，与 VIR 裸锚点同一文本分
//     布，供意图路由（评审 #1：带前缀语句 vs 裸锚点的余弦在真实多语嵌入
//     上系统性收窄阈值余量，写侧 0.72 门会静默失火）。
//
// 两个返回 map 均以 statement 索引为键；nil map（路由器缺席）或缺失条目
// （该文本嵌入失败）安全降级——同槽 supersede 通道不受影响。
func (s *evalMemoryStore) embedEvalStatements(ctx context.Context, stmts []string) (prefixed, routed map[int][]float32) {
	if len(stmts) == 0 || s.emb == nil || s.intentRouter(ctx) == nil {
		return nil, nil
	}
	all := make([]string, 0, 2*len(stmts))
	all = append(all, stmts...)
	for _, st := range stmts {
		all = append(all, stripEvalRolePrefix(st))
	}
	var vecs [][]float32
	if eb, ok := s.emb.(biz.EmbeddingBatchService); ok {
		v, err := eb.EmbedBatch(ctx, all)
		if err != nil {
			s.lg.Warn("eval atom batch embed failed, degrade to per-text",
				loggateway.StepID("memoryeval.atom_embed"), loggateway.Err(err))
		} else if len(v) == len(all) {
			vecs = v
		}
	}
	if vecs == nil {
		vecs = make([][]float32, len(all))
		for i, t := range all {
			v, err := s.emb.Embed(ctx, t)
			if err != nil {
				s.lg.Warn("eval atom embed failed, intent routing skipped for it",
					loggateway.StepID("memoryeval.atom_embed"), loggateway.Err(err))
				continue
			}
			vecs[i] = v
		}
	}
	prefixed = make(map[int][]float32, len(stmts))
	routed = make(map[int][]float32, len(stmts))
	for i := range stmts {
		if v := vecs[i]; v != nil {
			prefixed[i] = v
		}
		if v := vecs[len(stmts)+i]; v != nil {
			routed[i] = v
		}
	}
	return prefixed, routed
}

// evalFloorExempt reports whether an under-floor item still passes by mode
// (AMC P3): trend mode pins positively-scored series digests with count≥2
// (P3-c; 评审 C4——单观测 digest 复述原子本身，置顶只是噪声); aggregate mode
// pins positively-scored profile digests and slot histories (P3-a). Unscored
// rows never exempt — hop noise stays gated in every mode.
func evalFloorExempt(env evalCollectedItem, qa biz.EvalQueryAnalysis) bool {
	if env.item.Score <= 0 {
		return false
	}
	if qa.Trend && biz.IsEvalSeriesDigestPinworthy(env.item.Content) {
		return true
	}
	if qa.Aggregate &&
		(biz.IsEvalProfileDigestStatement(env.item.Content) ||
			biz.IsEvalSlotHistoryStatement(env.item.Content)) {
		return true
	}
	return false
}

// escapeEvalLike backslash-escapes the PG LIKE wildcards so a series-key
// keyword stays literal (PG LIKE escape char is '\' by default; '[' is only a
// wildcard in T-SQL bracket syntax, so it needs NO treatment here). Applied
// to the digest prefix before ListFactRowsForUser's %kw% contains-match.
func escapeEvalLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// evalDerivedSource keeps synthetic evidence distinct from source occurrences.
// Derived rows never anchor relative dates or enter the session transcript.
const evalDerivedSource = "agent_memory_challenge_derived"

func evalIsDerivedRaw(raw []byte) bool {
	return factJSONString(raw, "source_kind") == evalDerivedSource ||
		factJSONString(raw, "source_session_id") == "" && biz.IsEvalRollupStatement(factJSONString(raw, "statement"))
}

func evalRawSourceTimestamp(raw []byte) string {
	var meta map[string]json.RawMessage
	if json.Unmarshal([]byte(factJSONString(raw, "metadata_json")), &meta) == nil {
		if value, ok := meta["source_timestamp"]; ok {
			var at string
			_ = json.Unmarshal(value, &at)
			return at
		}
	}
	return factJSONString(raw, "valid_from")
}

// upsertNumericDigests rolls only new occurrences, under Add's advisory lock
// and transaction. Unknown dates stay unknown: no wall-clock day is invented.
func (s *evalMemoryStore) upsertNumericDigests(ctx context.Context, userID, sessionID string, msgIdx, atomIdx int, statement, eventAt string, atomDates []string, batchIDs map[string]struct{}, digestWritten *[]evalWrittenFact) error {
	day := evalDayPart(eventAt)
	if len(atomDates) > 0 {
		day = atomDates[0]
	}
	if day == "" {
		return nil
	}
	byKey := map[string][]biz.EvalNumericAtom{}
	for _, atom := range biz.ParseEvalNumericAtoms(statement) {
		if key := atom.SeriesKey(); key != "" {
			byKey[key] = append(byKey[key], atom)
		}
	}
	var keys []string
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		prefix := "[series-digest: " + key + "]"
		rows, err := s.facts.ListFactRowsForUser(ctx, "user", userID, userID, escapeEvalLike(prefix), evalDigestKeyLook, 0)
		if err != nil {
			return err
		}
		var oldIDs []string
		var series biz.EvalNumSeries
		for _, raw := range rows {
			old := decodeEvalFactRow(raw)
			parsed, ok := biz.ParseEvalSeriesDigest(old.statement)
			if !ok || parsed.Topic != key || old.validUntil != "" || !evalIsDerivedRaw(raw) {
				continue
			}
			if len(oldIDs) == 0 {
				series = parsed
			}
			oldIDs = append(oldIDs, old.id)
		}
		if series.LastDay != "" && day < series.LastDay {
			continue
		}
		if series.Topic == "" {
			series.Topic = key
		}
		for _, atom := range byKey[key] {
			series.Add(atom.Value, atom.Unit, day)
		}
		if err := s.writeEvalDerivedRow(ctx, userID, sessionID, series.RenderEvalSeriesDigest(), 0.7, eventAt, oldIDs, batchIDs, digestWritten); err != nil {
			return err
		}
	}
	return nil
}

// rollupEvalDerivedRows rebuilds profile and changed slot histories from
// surviving source rows. Lifecycle control rows and derived rows are excluded.
// Database errors propagate so raw evidence and its projections commit together.
func (s *evalMemoryStore) rollupEvalDerivedRows(ctx context.Context, userID, sessionID string, touchedSlots map[string]struct{}, batchIDs map[string]struct{}, digestWritten *[]evalWrittenFact) error {
	rows, err := s.facts.ListFactRowsForUserAll(ctx, "user", userID, userID, "", evalRangeFactListLimit, 0)
	if err != nil {
		return err
	}
	var decoded []evalFactRow
	var activeViews []biz.EvalRollupFact
	var latest string
	for _, raw := range rows {
		row := decodeEvalFactRow(raw)
		if row.id == "" || row.statement == "" {
			continue
		}
		if evalIsDerivedRaw(raw) {
			decoded = append(decoded, row)
			continue
		}
		if factJSONString(raw, "source_kind") == evalControlSource {
			continue
		}
		row.validFrom = evalRawSourceTimestamp(raw)
		decoded = append(decoded, row)
		if evalStrictlyAfter(row.validFrom, latest) || latest == "" {
			latest = row.validFrom
		}
		if slot := biz.EvalFactSlot(row.statement); slot != "" {
			touchedSlots[slot] = struct{}{}
		}
		if row.validUntil != "" || biz.IsEvalHistoricalStatement(row.statement) || biz.IsEvalPlannedStatement(stripEvalRolePrefix(row.statement)) {
			continue
		}
		body := biz.StripEvalHypotheticalClauses(stripEvalRolePrefix(row.statement))
		if body == "" {
			continue
		}
		activeViews = append(activeViews, biz.EvalRollupFact{Statement: body, Kind: row.kind, ValidFrom: row.validFrom})
	}
	digest := biz.AssembleEvalProfileDigest(activeViews)
	if err := s.rotateEvalDerivedRow(ctx, userID, sessionID, decoded, biz.EvalProfileDigestStatementPrefix(), digest, 1, latest, batchIDs, digestWritten); err != nil {
		return err
	}
	var slots []string
	for slot := range touchedSlots {
		slots = append(slots, slot)
	}
	sort.Strings(slots)
	for _, slot := range slots {
		var entries []biz.EvalRollupFact
		for _, row := range decoded {
			if biz.IsEvalRollupStatement(row.statement) || biz.IsEvalPlannedStatement(stripEvalRolePrefix(row.statement)) {
				continue
			}
			body := biz.StripEvalHypotheticalClauses(stripEvalRolePrefix(row.statement))
			if body == "" || biz.EvalFactSlot(row.statement) != slot {
				continue
			}
			entries = append(entries, biz.EvalRollupFact{Statement: body, Kind: row.kind, ValidFrom: row.validFrom, ValidUntil: row.validUntil})
		}
		if err := s.rotateEvalDerivedRow(ctx, userID, sessionID, decoded, biz.EvalSlotHistoryStatementPrefix(slot), biz.AssembleEvalSlotHistory(slot, entries), 1, latest, batchIDs, digestWritten); err != nil {
			return err
		}
	}
	return nil
}

func (s *evalMemoryStore) rotateEvalDerivedRow(ctx context.Context, userID, sessionID string, decoded []evalFactRow, prefix, statement string, importance float64, sourceAt string, batchIDs map[string]struct{}, digestWritten *[]evalWrittenFact) error {
	var oldIDs []string
	unchanged := false
	for _, row := range decoded {
		if row.validUntil != "" || row.sourceSessionID != "" || !strings.HasPrefix(row.statement, prefix) || !biz.IsEvalRollupStatement(row.statement) {
			continue
		}
		oldIDs = append(oldIDs, row.id)
		unchanged = unchanged || row.statement == statement
	}
	if unchanged && len(oldIDs) == 1 {
		return nil
	}
	return s.writeEvalDerivedRow(ctx, userID, sessionID, statement, importance, sourceAt, oldIDs, batchIDs, digestWritten)
}

func (s *evalMemoryStore) writeEvalDerivedRow(ctx context.Context, userID, sessionID, statement string, importance float64, sourceAt string, oldIDs []string, batchIDs map[string]struct{}, written *[]evalWrittenFact) error {
	var raw []byte
	var err error
	if statement != "" {
		// Empty source_timestamp explicitly prevents legacy valid_from fallback.
		meta, _ := json.Marshal(map[string]any{"eval_session": sessionID, "derived": true, "source_timestamp": ""})
		raw, err = s.facts.UpsertFactRow(ctx, biz.FactUpsert{
			ScopeType: "user", ScopeID: userID, UserID: userID, AgentID: userID,
			Fingerprint: evalStableHash(evalDerivedSource, userID, statement, time.Now().UTC().Format(time.RFC3339Nano)),
			Statement:   statement, FactKind: "domain_knowledge", Confidence: 1, Importance: importance, Status: "active", Version: 1,
			SourceKind: evalDerivedSource, CreatedAt: sourceAt, ValidFrom: sourceAt, MetadataJSON: string(meta), SkipPIIRedact: true,
		})
		if err != nil {
			return err
		}
	}
	newID := factJSONString(raw, "id")
	for _, oldID := range oldIDs {
		if oldID == newID {
			continue
		}
		if err := s.invalidateEvalOccurrence(ctx, userID, oldID, sourceAt); err != nil {
			return err
		}
	}
	if newID != "" {
		batchIDs[newID] = struct{}{}
		*written = append(*written, evalWrittenFact{id: newID, raw: raw})
	}
	return nil
}

// Rebuild numeric projections after deletion, without recycling any stale
// counters. Raw rows remain the authority; repeated source observations dedup.
func (s *evalMemoryStore) rebuildEvalNumericDigests(ctx context.Context, userID string) error {
	rows, err := s.facts.ListFactRowsForUserAll(ctx, "user", userID, userID, "", evalRangeFactListLimit, 0)
	if err != nil {
		return err
	}
	sort.SliceStable(rows, func(i, j int) bool { return evalRawSourceTimestamp(rows[i]) < evalRawSourceTimestamp(rows[j]) })
	seen := map[string]bool{}
	batch := map[string]struct{}{}
	var written []evalWrittenFact
	for _, raw := range rows {
		if evalIsDerivedRaw(raw) || factJSONString(raw, "source_kind") == evalControlSource || factJSONString(raw, "valid_until") != "" {
			continue
		}
		statement, sourceAt := factJSONString(raw, "statement"), evalRawSourceTimestamp(raw)
		key := evalStableHash(sourceAt, statement)
		if seen[key] {
			continue
		}
		seen[key] = true
		var meta struct {
			Dates []string `json:"event_dates"`
		}
		_ = json.Unmarshal([]byte(factJSONString(raw, "metadata_json")), &meta)
		if err := s.upsertNumericDigests(ctx, userID, factJSONString(raw, "source_session_id"), 0, 0, statement, sourceAt, meta.Dates, batch, &written); err != nil {
			return err
		}
	}
	return nil
}

// applyEvalQueryModes assigns the query-side modes via the VIR router over
// the already-computed query embedding (AMC P3). When the router is
// unavailable deterministic modes stay intact.
func (s *evalMemoryStore) applyEvalQueryModes(ctx context.Context, qa *biz.EvalQueryAnalysis, embedding []float32) {
	if len(embedding) == 0 {
		return
	}
	r := s.intentRouter(ctx)
	if r == nil {
		return
	}
	qa.Chronological = qa.Chronological || r.Match(embedding, biz.EvalIntentChronological)
	qa.Aggregate = qa.Aggregate || r.Match(embedding, biz.EvalIntentAggregate)
	qa.Enumerative = qa.Enumerative || r.Match(embedding, biz.EvalIntentEnumerative)
	qa.Relational = qa.Relational || r.Match(embedding, biz.EvalIntentRelational)
	qa.Trend = qa.Trend || r.Match(embedding, biz.EvalIntentTrend)
	qa.Historical = qa.Historical || qa.Chronological
}

// evalRelationalHop2Budget returns the remaining link-id budget for the
// relational second hop (AMC P3-d: 限深 2、总量上界 30，确定性).
func evalRelationalHop2Budget(used int) int {
	if rem := evalRelationalHopIDBudget - used; rem > 0 {
		return rem
	}
	return 0
}

// evalEnumerativeCapItems caps an enumerative-mode result set (AMC P3 / A3):
// plain score-descending order, no MMR diversity penalty. Set-completeness
// questions ("有哪些/分别/all of") need every same-topic member fact; MMR
// would demote them as redundant and the judge sees a partial set. Ties keep
// the incoming (recency-first) order via stable sort.
func evalEnumerativeCapItems(items []evalCollectedItem, topK int32) []evalCollectedItem {
	if len(items) == 0 {
		return items
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].item.Score > items[j].item.Score
	})
	if lim := int(topK); lim > 0 && len(items) > lim {
		items = items[:lim]
	}
	return items
}

// evalTrendCapItems orders a trend-mode result set (AMC P3-c / G4): numeric
// series digest rows with count≥2 come FIRST (they are the designed evidence
// for 模拟/规律/预测 queries; 评审 C4——单观测 digest 复述原子本身，置顶是纯
// 噪声), then the remaining facts, each partition in plain score-descending
// order with stable ties. MMR is skipped — a digest's value is precisely its
// same-topic aggregation, which diversity capping would demote.
func evalTrendCapItems(items []evalCollectedItem, topK int32) []evalCollectedItem {
	if len(items) == 0 {
		return items
	}
	digests := make([]evalCollectedItem, 0, 4)
	others := make([]evalCollectedItem, 0, len(items))
	for _, env := range items {
		if biz.IsEvalSeriesDigestPinworthy(env.item.Content) {
			digests = append(digests, env)
		} else {
			others = append(others, env)
		}
	}
	sort.SliceStable(digests, func(i, j int) bool {
		return digests[i].item.Score > digests[j].item.Score
	})
	sort.SliceStable(others, func(i, j int) bool {
		return others[i].item.Score > others[j].item.Score
	})
	out := append(digests, others...)
	if lim := int(topK); lim > 0 && len(out) > lim {
		out = out[:lim]
	}
	return out
}

// evalAggregateCapItems orders an aggregate-mode result set (AMC P3-a / F1):
// rollup rows (profile digest + slot histories) come FIRST — they are the
// synthesized cross-session evidence for 总体/profile/人生故事类查询 — then
// the remaining facts/episodes go through ordinary MMR against the REMAINING
// topK budget, so tail diversity behavior matches the default path. topK<=0
// keeps MMR's own unbounded semantics.
func evalAggregateCapItems(items []evalCollectedItem, topK int32) []evalCollectedItem {
	if len(items) == 0 {
		return items
	}
	rollup := make([]evalCollectedItem, 0, 2)
	rest := make([]evalCollectedItem, 0, len(items))
	for _, env := range items {
		if biz.IsEvalProfileDigestStatement(env.item.Content) ||
			biz.IsEvalSlotHistoryStatement(env.item.Content) {
			rollup = append(rollup, env)
		} else {
			rest = append(rest, env)
		}
	}
	// Rollup rows keep plain score order among themselves.
	sort.SliceStable(rollup, func(i, j int) bool {
		return rollup[i].item.Score > rollup[j].item.Score
	})
	out := rollup
	lim := int(topK)
	switch {
	case lim <= 0:
		out = append(out, evalMMRCapItems(rest, 0)...)
	case len(rollup) < lim:
		out = append(out, evalMMRCapItems(rest, int32(lim-len(rollup)))...)
	}
	if lim > 0 && len(out) > lim {
		out = out[:lim]
	}
	return out
}

// Reserve source coverage before allocating spare summary slots to rollups.
// A synthetic profile cannot displace the only evidence for a sparse session.
func evalAggregateCoverageCapItems(items []evalCollectedItem, topK int32, broad bool) []evalCollectedItem {
	var rollup, sources []evalCollectedItem
	sessions := map[string]bool{}
	for _, env := range items {
		if biz.IsEvalProfileDigestStatement(env.item.Content) || biz.IsEvalSlotHistoryStatement(env.item.Content) {
			rollup = append(rollup, env)
		} else {
			sources = append(sources, env)
			if env.sess != "" {
				sessions[env.sess] = true
			}
		}
	}
	limit := int(topK)
	if limit <= 0 {
		limit = 100
	}
	spare := limit - len(sessions)
	if spare < 0 {
		spare = 0
	}
	if len(rollup) > spare {
		rollup = rollup[:spare]
	}
	remaining := limit - len(rollup)
	if remaining > 0 {
		rollup = append(rollup, evalCoverageCapItems(sources, int32(remaining), broad)...)
	}
	return rollup
}

func (s *evalMemoryStore) appendEvalDerivedCandidates(ctx context.Context, userID string, qa biz.EvalQueryAnalysis, appendRow func([]byte, float64)) error {
	if qa.RangeStart != nil || qa.RangeEnd != nil || (!qa.Aggregate && !qa.Historical && !qa.Trend) {
		return nil
	}
	rows, err := s.facts.data.RWDB().ReadDB(ctx).QueryContext(ctx, sqlFactSelectSQL(false)+
		` WHERE scope_type='user' AND scope_id=$1 AND user_id=$1 AND status='active' AND deleted_at='' AND valid_until='' AND source_kind=$2 ORDER BY updated_at DESC,id ASC LIMIT 64`, userID, evalDerivedSource)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		raw, err := scanFactRowJSON(rows)
		if err != nil {
			return err
		}
		appendRow(raw, 0)
	}
	return rows.Err()
}
