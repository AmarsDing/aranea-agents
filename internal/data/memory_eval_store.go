package data

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

const (
	evalMMRLambda       = 0.7
	evalL2SearchLimit   = 20
	evalSessionHopTop   = 5
	evalSessionHopLimit = 12
	evalLinkHopTop      = 8
	evalSupersedeLook   = 40
	evalEpisodeMaxChars = 32768
	// evalRecallMinScore is the deterministic evidence floor for the eval
	// search path (AMC C3-A Q9): biz.AdaptiveRecallMinScore treats a
	// configured value <= 0 as "no floor", so the eval adapter passes a
	// positive base (effective = max(base, top1*0.6)) and filters strictly —
	// an empty result stays empty, no backfill padding.
	evalRecallMinScore = 0.12
	// evalAggregateRecentEpisodes is how many recent L2 episodes the
	// aggregate query mode (总结/总体/overall/…) merges on top of fact hits.
	evalAggregateRecentEpisodes = 5
	// evalRangeFactListLimit bounds the user-scoped fact scan used for
	// date-range filtering.
	evalRangeFactListLimit = 200
	// evalCorrectionCosine is the minimum cosine similarity between a
	// correction statement and an older fact for the vector-correction
	// supersede path (AMC C3-A Q7) to invalidate the old value.
	evalCorrectionCosine = 0.82
	// evalCorrectionTopK bounds the vector candidates scanned per correction.
	// AMC C3 修复 #6：向量索引是全局（未按 scope 分区）索引，topK=5 时
	// 候选可能被其它租户/用户的更近邻向量挤占，真正的同 scope 矛盾旧值
	// 进不了前 5；宽取 50 后按 scope 过滤，成本仍是一次索引访问。
	evalCorrectionTopK = 50
)

// evalMemoryStore implements biz.EvalMemoryStore over L3 facts plus a thin
// L2 episode timeline. Writes skip statement redaction (PII is flagged only)
// so benchmark evidence stays searchable. Embed index sync is scheduled
// after the Add response so long histories cannot time out the contract.
type evalMemoryStore struct {
	facts    *l3FactRepo
	episodes *l2EpisodeRepo
	vec      *biz.MemoryUsecase
	syncer   biz.MemoryFactIndexSyncer
	lg       loggateway.Logger
}

var _ biz.EvalMemoryStore = (*evalMemoryStore)(nil)

// NewEvalMemoryStore assembles the evaluation facade. When emb is nil, writes
// skip vector indexing and recall degrades to keyword/FTS — the Add/Search
// contract stays functional.
func NewEvalMemoryStore(d *Data, emb biz.EmbeddingService, lg loggateway.Logger) biz.EvalMemoryStore {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	s := &evalMemoryStore{
		facts:    newL3FactRepo(d, d.VectorStore()),
		episodes: newL2EpisodeRepo(d, d.VectorStore()),
		lg:       lg.With(loggateway.Domain("memory_eval")),
	}
	if emb != nil {
		s.vec = biz.NewMemoryUsecase(NewMemoryRepo(d), emb)
		s.syncer = NewMemoryFactIndexSync(s.vec, d, lg)
	}
	return s
}

func (s *evalMemoryStore) AddMessages(ctx context.Context, userID, sessionID string, msgs []biz.EvalMessage) (int, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		sessionID = "eval-default"
	}
	stored := 0
	var written []evalWrittenFact
	batchIDs := map[string]struct{}{}
	for i, m := range msgs {
		eventAt := normalizeEvalTimestamp(m.Timestamp)
		anchor := time.Now().UTC()
		if eventAt != "" {
			if t, err := time.Parse(time.RFC3339Nano, eventAt); err == nil {
				anchor = t
			}
		}
		// A1/Q4 atomization: split each message into atomic statements, each
		// atom keeps the role prefix and is classified / superseded / stored
		// independently; stored still counts messages, not atoms.
		atoms := biz.SplitEvalStatements(m.Content, biz.EvalMaxStatementAtoms)
		for j, atom := range atoms {
			// AMC C3 修复 #3：相对时间按原子解析（此前只用 atoms[0] 的结果
			// 复用到全部原子，「…。前天…。」第二枚原子丢失/错配 event_dates，
			// Q6 区间/时序题与 Q9 前缀全部错位）。
			atomDates := biz.ResolveEvalRelativeTimes(stripEvalRolePrefix(atom), anchor)
			statement := atom
			if m.Role != "" {
				statement = m.Role + ": " + atom
			}
			kind := biz.ClassifyEvalFactKind(statement)
			imp := 0.5
			if kind != "event" {
				imp = 0.7
			}
			prevs, err := s.supersedeContradictions(ctx, userID, kind, statement, eventAt, batchIDs)
			if err != nil {
				s.lg.Warn("eval supersede skipped",
					loggateway.StepID("memoryeval.supersede"), loggateway.Err(err))
			}
			raw, err := s.facts.UpsertFactRow(ctx, biz.FactUpsert{
				ScopeType:       "user",
				ScopeID:         userID,
				UserID:          userID,
				AgentID:         userID,
				Statement:       statement,
				FactKind:        kind,
				Confidence:      1.0,
				Importance:      imp,
				Status:          "active",
				Version:         1,
				SourceKind:      "agent_memory_challenge",
				SourceSessionID: sessionID,
				SourceMessageID: m.MessageID,
				CreatedAt:       eventAt,
				ValidFrom:       eventAt,
				SkipPIIRedact:   true,
				MetadataJSON:    evalMetadataJSON(sessionID, i, j, atomDates, prevs),
			})
			if err != nil {
				return stored, err
			}
			id := factJSONString(raw, "id")
			if id != "" {
				batchIDs[id] = struct{}{}
			}
			written = append(written, evalWrittenFact{id: id, raw: raw})
		}
		stored++
	}
	if err := s.linkEvalSiblings(ctx, written); err != nil {
		s.lg.Warn("eval sibling links skipped",
			loggateway.StepID("memoryeval.links"), loggateway.Err(err))
	}
	if err := s.upsertEvalEpisode(ctx, userID, sessionID, msgs); err != nil {
		s.lg.Warn("eval L2 episode skipped",
			loggateway.StepID("memoryeval.l2"), loggateway.Err(err))
	}
	s.scheduleEmbed(written)
	return stored, nil
}

// evalCollectedItem is one search hit plus its evidence-dating metadata
// (metadata.event_dates[0] else valid_from else created_at) used by the
// range filter, chronological ordering and the Q9 evidence prefix.
type evalCollectedItem struct {
	item biz.EvalMemoryItem
	day  string
	sess string
}

func (c evalCollectedItem) prefix() string { return evalItemPrefix(c.day, c.sess) }

func (s *evalMemoryStore) SearchMemories(ctx context.Context, userID, query string, topK int32) ([]biz.EvalMemoryItem, error) {
	now := time.Now().UTC()
	qa := biz.AnalyzeEvalQuery(query, now)
	var embedding []float32
	if s.vec != nil {
		if v, err := s.vec.EmbedText(ctx, query); err == nil {
			embedding = v
		} else {
			s.lg.Warn("eval query embed failed, degrade to keyword recall",
				loggateway.StepID("memoryeval.search_degrade"), loggateway.Err(err))
		}
	}
	rows, err := s.facts.RecallL3Facts(ctx, "user", userID, userID, query, embedding, topK, 0)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	var items []evalCollectedItem
	appendRow := func(raw []byte, fallback float64) {
		env, ok := evalFactEnvelope(raw, fallback)
		if !ok || env.item.ID == "" {
			return
		}
		if _, dup := seen[env.item.ID]; dup {
			return
		}
		seen[env.item.ID] = struct{}{}
		items = append(items, env)
	}
	for _, raw := range rows {
		appendRow(raw, 0)
	}
	s.appendSessionHop(ctx, userID, rows, appendRow)
	s.appendLinkHop(ctx, userID, rows, appendRow)
	if s.episodes != nil {
		l2lim := evalL2SearchLimit
		if int(topK) > 0 && int(topK) < l2lim {
			l2lim = int(topK)
		}
		episodes, epErr := s.episodes.RecallL2Episodes(ctx, userID, "", query, embedding, int32(l2lim))
		if epErr != nil {
			s.lg.Warn("eval L2 search skipped",
				loggateway.StepID("memoryeval.l2_search"), loggateway.Err(epErr))
		} else {
			for _, raw := range episodes {
				env, ok := evalEpisodeEnvelope(raw, 0)
				if !ok || env.item.ID == "" {
					continue
				}
				if _, dup := seen[env.item.ID]; dup {
					continue
				}
				seen[env.item.ID] = struct{}{}
				items = append(items, env)
			}
		}
	}
	// A3/Q6 range mode: widen the candidate pool with the full user fact list
	// (deterministic keyword scoring), then keep only evidence whose
	// COALESCE(event_dates[0], valid_from, created_at) falls inside the range.
	if qa.RangeStart != nil || qa.RangeEnd != nil {
		ranged, rerr := s.facts.ListFactRowsForUser(ctx, "user", userID, userID, "", evalRangeFactListLimit, 0)
		if rerr != nil {
			s.lg.Warn("eval range widen skipped",
				loggateway.StepID("memoryeval.range_widen"), loggateway.Err(rerr))
		} else {
			tokens := tokenizeQuery(query)
			for _, raw := range ranged {
				var row map[string]any
				if json.Unmarshal(raw, &row) != nil {
					continue
				}
				bd := scoreFactRow(row, tokens, embedding, nil, now)
				appendRow(annotateFactScores(raw, bd), bd.Total)
			}
		}
		items = filterEvalItemsByRange(items, qa)
	}
	// A6/Q9 evidence gating: strict adaptive floor — empty stays empty.
	items = filterEvalItemsByFloor(items, evalRecallMinScore)
	if len(items) == 0 {
		return []biz.EvalMemoryItem{}, nil
	}
	// A3/A5 aggregate mode: additionally merge the user's most recent L2
	// episodes (deduped) as summary evidence. These are deliberately
	// mode-attached and therefore appended after the fact-evidence floor.
	// The dedup set is rebuilt from the surviving items: the default L2 path
	// marks episode ids seen before the floor, and episodes that did not pass
	// the floor must still be attachable here.
	if qa.Aggregate && s.episodes != nil {
		seen = make(map[string]struct{}, len(items))
		for _, env := range items {
			seen[env.item.ID] = struct{}{}
		}
		s.appendAggregateEpisodes(ctx, userID, query, now, seen, &items)
	}
	// A3/Q6 chronological mode: order by the evidence day ascending and skip
	// the MMR cap (topK still bounds the result count).
	if qa.Chronological {
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].day != items[j].day {
				return items[i].day < items[j].day
			}
			return items[i].item.Score > items[j].item.Score
		})
		if lim := int(topK); lim > 0 && len(items) > lim {
			items = items[:lim]
		}
	} else {
		items = evalMMRCapItems(items, topK)
	}
	out := make([]biz.EvalMemoryItem, 0, len(items))
	for _, env := range items {
		it := env.item
		// Q9 evidence gating: identifier masking, then the evidence prefix
		// (which goes before the role prefix inside the statement).
		it.Content = env.prefix() + MaskEvalEvidence(it.Content, query)
		out = append(out, it)
	}
	return out, nil
}

// filterEvalItemsByRange keeps items whose evidence day falls inside the
// inclusive [RangeStart, RangeEnd] bounds (open sides pass).
func filterEvalItemsByRange(items []evalCollectedItem, qa biz.EvalQueryAnalysis) []evalCollectedItem {
	if qa.RangeStart == nil && qa.RangeEnd == nil {
		return items
	}
	startDay, endDay := "", ""
	if qa.RangeStart != nil {
		startDay = qa.RangeStart.Format("2006-01-02")
	}
	if qa.RangeEnd != nil {
		endDay = qa.RangeEnd.Format("2006-01-02")
	}
	out := items[:0]
	for _, env := range items {
		if env.day == "" {
			continue
		}
		if startDay != "" && env.day < startDay {
			continue
		}
		if endDay != "" && env.day > endDay {
			continue
		}
		out = append(out, env)
	}
	return out
}

// filterEvalItemsByFloor applies biz.AdaptiveRecallMinScore with a positive
// base so effective = max(base, top1*ratio). Zero-score noise (e.g. unscored
// hop rows) never passes when a floor exists; empty stays empty.
func filterEvalItemsByFloor(items []evalCollectedItem, base float64) []evalCollectedItem {
	if len(items) == 0 {
		return items
	}
	top1 := 0.0
	for _, env := range items {
		if env.item.Score > top1 {
			top1 = env.item.Score
		}
	}
	floor := biz.AdaptiveRecallMinScore(base, top1)
	if floor <= 0 {
		return items
	}
	out := items[:0]
	for _, env := range items {
		if env.item.Score >= floor {
			out = append(out, env)
		}
	}
	return out
}

// appendAggregateEpisodes merges the user's most recent L2 episodes for
// summary-style queries (AMC C3-A Q8a). Episodes carry their deterministic
// keyword/importance score so the downstream cap ranks them consistently.
func (s *evalMemoryStore) appendAggregateEpisodes(ctx context.Context, userID, query string, now time.Time, seen map[string]struct{}, items *[]evalCollectedItem) {
	recent, err := s.episodes.ListEpisodeRowsForRecall(ctx, userID, "", evalAggregateRecentEpisodes)
	if err != nil {
		s.lg.Warn("eval aggregate episodes skipped",
			loggateway.StepID("memoryeval.aggregate_l2"), loggateway.Err(err))
		return
	}
	tokens := tokenizeQuery(query)
	for _, raw := range recent {
		env, ok := evalEpisodeEnvelope(raw, 0)
		if !ok || env.item.ID == "" {
			continue
		}
		if _, dup := seen[env.item.ID]; dup {
			continue
		}
		var row map[string]any
		if json.Unmarshal(raw, &row) == nil {
			title, _ := row["title"].(string)
			summary, _ := row["outcome_summary"].(string)
			imp := float64(anyFloat(row, "importance"))
			endedAt, _ := row["ended_at"].(string)
			kw := keywordOverlapScore(tokens, title+" "+summary)
			env.item.Score = l2ScoreWeightKeyword*kw + l2ScoreWeightImport*imp*decayFactor(endedAt, now)
		}
		seen[env.item.ID] = struct{}{}
		*items = append(*items, env)
	}
}

// HasEvalCorrectionCue reports an explicit correction/改口 marker (AMC C3-A
// Q7). AMC C3 修复 #8：原裸子串「改」误伤 改变/改善/修改/改天（每枚原子多付
// 一次 EmbedText + 一次向量近邻搜索，且 0.82 余弦下存在误杀相邻事实风险），
// 收紧为明确改口句式（改成/改为/改到/改回/改去/改口/改主意/调换/换成/换为/换了/
// 转到/变成/变为/不再是/不喜欢了/没感觉），保留 其实/不对/错了 与英文
// actually/correction/no longer/in fact。英文匹配大小写不敏感。
func HasEvalCorrectionCue(statement string) bool {
	s := strings.TrimSpace(statement)
	if s == "" {
		return false
	}
	for _, c := range []string{
		"改成", "改为", "改到", "改回", "改去", "改口", "改主意",
		"调换", "换成", "换为", "换了", "转到", "变成", "变为",
		"不再是", "不喜欢了", "没感觉",
		"其实", "不对", "错了",
	} {
		if strings.Contains(s, c) {
			return true
		}
	}
	if strings.Contains(s, "不是") && strings.Contains(s, "而是") {
		return true
	}
	low := strings.ToLower(s)
	for _, c := range []string{"actually", "correction", "no longer", "in fact"} {
		if strings.Contains(low, c) {
			return true
		}
	}
	return false
}

// supersedeContradictions runs the same-slot invalidation loop and — when an
// embedder is available and the new statement carries a correction cue — the
// vector-correction pass (AMC C3-A Q7). It returns the audit trail of
// superseded statements for the new fact's metadata.previous_values.
//
// AMC C3 修复 #1：batchIDs（本次 AddMessages 已写入的事实）只在向量通道
// 排除——向量近邻可能把同批 sibling 误当矛盾旧值；同槽通道必须允许失效同批
// 行，否则同一请求内的 [旧值, 改口] 消息对互不覆盖（对基线的功能回退）。
//
// AMC C3 修复 #0/#4（下）：事件时序守卫——平台重试会重放旧请求，重放里的
// 旧语句绝不能失效事件时间（valid_from）严格更晚的事实；两通道统一执行。
// 墓碑行（valid_until 非空）直接跳过，避免重放时无效翻动审计时戳。
// 向量通道在 embedding 失败或未接线时整体跳过（warn-only）。
func (s *evalMemoryStore) supersedeContradictions(ctx context.Context, userID, newKind, newStmt, newEventAt string, batchIDs map[string]struct{}) ([]evalPrevValue, error) {
	if !evalGovernableKindForStore(newKind) {
		return nil, nil
	}
	rows, err := s.facts.ListFactRowsForUser(ctx, "user", userID, userID, "", evalSupersedeLook, 0)
	if err != nil {
		return nil, err
	}
	var prevs []evalPrevValue
	invalidated := map[string]struct{}{}
	invalidate := func(old evalFactRow) {
		if old.id == "" || old.statement == "" {
			return
		}
		if old.validUntil != "" {
			return
		}
		if _, dup := invalidated[old.id]; dup {
			return
		}
		if _, invErr := s.facts.InvalidateFact(ctx, old.id); invErr != nil {
			s.lg.Warn("eval invalidate failed",
				loggateway.StepID("memoryeval.invalidate"), loggateway.Err(invErr),
				loggateway.Str("fact_id", old.id))
			return
		}
		invalidated[old.id] = struct{}{}
		prevs = append(prevs, evalPrevValue{
			Statement:  old.statement,
			ValidUntil: time.Now().UTC().Format(time.RFC3339Nano),
		})
		if old.sourceSessionID != "" {
			// The merged eval episode keeps every appended turn, so a
			// superseded statement must also leave the L2 transcript,
			// otherwise recall keeps surfacing the stale value.
			if stripErr := s.episodes.StripEpisodeStatement(ctx, userID, old.sourceSessionID, old.statement); stripErr != nil {
				s.lg.Warn("eval episode strip failed",
					loggateway.StepID("memoryeval.episode_strip"), loggateway.Err(stripErr))
			}
		}
	}
	for _, raw := range rows {
		old := decodeEvalFactRow(raw)
		if old.id == "" || old.statement == "" {
			continue
		}
		if !biz.ShouldSupersedeEvalFact(old.kind, old.statement, newKind, newStmt) {
			continue
		}
		if !evalTimeAllowsInvalidate(old.validFrom, newEventAt) {
			continue
		}
		invalidate(old)
	}
	// Vector-correction pass (Q7): a correction cue plus a semantically near
	// older fact (cosine >= 0.82, normalization-inequivalent) invalidates the
	// stale value even when it occupies a different slot.
	if s.vec == nil || !HasEvalCorrectionCue(newStmt) || s.facts.vectorStore == nil {
		return prevs, nil
	}
	emb, embErr := s.vec.EmbedText(ctx, newStmt)
	if embErr != nil || len(emb) == 0 {
		if embErr != nil {
			s.lg.Warn("eval correction embed failed, vector supersede skipped",
				loggateway.StepID("memoryeval.correction_embed"), loggateway.Err(embErr))
		}
		return prevs, nil
	}
	hits, searchErr := s.facts.vectorStore.Search(ctx, float32To64(emb), evalCorrectionTopK, 0)
	if searchErr != nil {
		s.lg.Warn("eval correction vector search failed, vector supersede skipped",
			loggateway.StepID("memoryeval.correction_search"), loggateway.Err(searchErr))
		return prevs, nil
	}
	var ids []string
	for _, h := range hits {
		if h.Score < evalCorrectionCosine {
			continue
		}
		if _, sameBatch := batchIDs[h.ID]; sameBatch {
			continue
		}
		ids = append(ids, h.ID)
	}
	if len(ids) == 0 {
		return prevs, nil
	}
	candRows, qErr := s.facts.queryFactRowsByIDs(ctx, ids, "user", userID, userID, false)
	if qErr != nil {
		return prevs, qErr
	}
	defer candRows.Close()
	for candRows.Next() {
		raw, scanErr := scanFactRowJSON(candRows)
		if scanErr != nil {
			continue
		}
		old := decodeEvalFactRow(raw)
		if old.id == "" || old.statement == "" {
			continue
		}
		if biz.NormalizeForDedup(old.statement) == biz.NormalizeForDedup(newStmt) {
			continue
		}
		if !evalTimeAllowsInvalidate(old.validFrom, newEventAt) {
			continue
		}
		invalidate(old)
	}
	return prevs, candRows.Err()
}

// evalTimeAllowsInvalidate is the replay-safety guard (AMC C3 修复 #0/#4):
// an existing fact may be invalidated only when its event time (valid_from)
// is not strictly later than the new statement's event time. Platform
// retries re-POST older requests after newer ones have landed; without this
// guard the replayed old statement would re-invalidate newer facts via the
// same-slot or vector pass. Unparseable/empty times fail open (allow).
func evalTimeAllowsInvalidate(oldValidFrom, newEventAt string) bool {
	oldValidFrom = strings.TrimSpace(oldValidFrom)
	newEventAt = strings.TrimSpace(newEventAt)
	if oldValidFrom == "" || newEventAt == "" {
		return true
	}
	of, err1 := time.Parse(time.RFC3339Nano, oldValidFrom)
	nv, err2 := time.Parse(time.RFC3339Nano, newEventAt)
	if err1 != nil || err2 != nil {
		return true
	}
	return !of.After(nv)
}

func evalGovernableKindForStore(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "user_preference", "preference", "user_identity", "constraint", "agent_instruction":
		return true
	default:
		return false
	}
}

func (s *evalMemoryStore) linkEvalSiblings(ctx context.Context, written []evalWrittenFact) error {
	if len(written) < 2 {
		return nil
	}
	ids := make([]string, 0, len(written))
	for _, w := range written {
		if w.id != "" {
			ids = append(ids, w.id)
		}
	}
	if len(ids) < 2 {
		return nil
	}
	payload, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	links := string(payload)
	for _, id := range ids {
		if err := s.facts.setFactLinks(ctx, id, links); err != nil {
			return err
		}
	}
	return nil
}

func (s *evalMemoryStore) upsertEvalEpisode(ctx context.Context, userID, sessionID string, msgs []biz.EvalMessage) error {
	if s.episodes == nil || len(msgs) == 0 {
		return nil
	}
	var b strings.Builder
	goal := ""
	for _, m := range msgs {
		line := m.Content
		if m.Role != "" {
			line = m.Role + ": " + m.Content
		}
		if goal == "" && (m.Role == "" || strings.EqualFold(m.Role, "user")) {
			goal = truncateRunes(m.Content, 200)
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line)
		if b.Len() >= evalEpisodeMaxChars {
			break
		}
	}
	summary := truncateRunes(b.String(), evalEpisodeMaxChars)
	title := "eval session " + sessionID
	return s.episodes.InsertL1ArchiveEpisode(ctx, biz.L1ArchiveEpisodeInsert{
		SessionID:      sessionID,
		AgentID:        userID,
		TaskID:         "eval:" + sessionID,
		TaskTitle:      title,
		Status:         "completed",
		Goal:           goal,
		Outcome:        summary,
		OutcomeSummary: summary,
		EpisodeKind:    "eval_session",
		Importance:     0.6,
		Confidence:     0.8,
		MergeOutcome:   true,
		MaxMergeChars:  evalEpisodeMaxChars,
	})
}

func (s *evalMemoryStore) scheduleEmbed(written []evalWrittenFact) {
	if s.syncer == nil || len(written) == 0 {
		return
	}
	copied := make([][]byte, 0, len(written))
	for _, w := range written {
		if len(w.raw) > 0 {
			copied = append(copied, append([]byte(nil), w.raw...))
		}
	}
	if len(copied) == 0 {
		return
	}
	syncer := s.syncer
	lg := s.lg
	safego.GoBackground("memoryeval.embed", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		for _, raw := range copied {
			if err := syncer.SyncFactIndexFromRow(ctx, raw); err != nil {
				lg.Warn("eval fact index sync failed, keyword recall still available",
					loggateway.StepID("memoryeval.index_sync"), loggateway.Err(err))
			}
		}
	})
}

func (s *evalMemoryStore) appendSessionHop(ctx context.Context, userID string, hits [][]byte, appendRow func([]byte, float64)) {
	sessions := uniqueNonEmpty(evalJSONStrings(hits, "source_session_id"), evalSessionHopTop)
	for _, sess := range sessions {
		extra, err := s.facts.listFactsBySourceSession(ctx, "user", userID, userID, sess, evalSessionHopLimit)
		if err != nil {
			s.lg.Warn("eval session hop skipped",
				loggateway.StepID("memoryeval.session_hop"), loggateway.Err(err))
			return
		}
		for _, raw := range extra {
			appendRow(raw, 0)
		}
	}
}

func (s *evalMemoryStore) appendLinkHop(ctx context.Context, userID string, hits [][]byte, appendRow func([]byte, float64)) {
	var ids []string
	for i, raw := range hits {
		if i >= evalLinkHopTop {
			break
		}
		ids = append(ids, parseJSONStringSlice(factJSONString(raw, "links"))...)
	}
	ids = uniqueNonEmpty(ids, 24)
	if len(ids) == 0 {
		return
	}
	rows, err := s.facts.queryFactRowsByIDs(ctx, ids, "user", userID, userID, false)
	if err != nil {
		s.lg.Warn("eval link hop skipped",
			loggateway.StepID("memoryeval.link_hop"), loggateway.Err(err))
		return
	}
	defer rows.Close()
	scored := scoreFactRows(rows, nil, nil, nil, 0, time.Now().UTC())
	for _, sf := range scored {
		appendRow(sf.raw, sf.score)
	}
}

func evalMMRCapItems(items []evalCollectedItem, topK int32) []evalCollectedItem {
	if len(items) == 0 {
		return items
	}
	lim := int(topK)
	if lim <= 0 {
		lim = 100
	}
	texts := make([]string, len(items))
	scores := make([]float64, len(items))
	for i, env := range items {
		texts[i] = env.item.Content
		scores[i] = env.item.Score
	}
	order := biz.MMRRerankTexts(texts, scores, lim, evalMMRLambda)
	out := make([]evalCollectedItem, 0, len(order))
	for _, idx := range order {
		out = append(out, items[idx])
	}
	return out
}

type evalWrittenFact struct {
	id  string
	raw []byte
}

type evalFactRow struct {
	id              string
	statement       string
	kind            string
	sourceSessionID string
	validFrom       string
	validUntil      string
}

func decodeEvalFactRow(raw []byte) evalFactRow {
	var row struct {
		ID              string `json:"id"`
		Statement       string `json:"statement"`
		FactKind        string `json:"fact_kind"`
		SourceSessionID string `json:"source_session_id"`
		ValidFrom       string `json:"valid_from"`
		ValidUntil      string `json:"valid_until"`
	}
	if json.Unmarshal(raw, &row) != nil {
		return evalFactRow{}
	}
	return evalFactRow{
		id:              row.ID,
		statement:       row.Statement,
		kind:            row.FactKind,
		sourceSessionID: row.SourceSessionID,
		validFrom:       row.ValidFrom,
		validUntil:      row.ValidUntil,
	}
}

func evalItemFromFactJSON(raw []byte, fallback float64) (biz.EvalMemoryItem, bool) {
	var row struct {
		ID        string `json:"id"`
		Statement string `json:"statement"`
		CreatedAt string `json:"created_at"`
		ValidFrom string `json:"valid_from"`
		Scores    struct {
			Total float64 `json:"total"`
		} `json:"scores"`
	}
	if json.Unmarshal(raw, &row) != nil || row.ID == "" {
		return biz.EvalMemoryItem{}, false
	}
	score := row.Scores.Total
	if score == 0 {
		score = fallback
	}
	ts := row.ValidFrom
	if ts == "" {
		ts = row.CreatedAt
	}
	if row.CreatedAt == "" {
		row.CreatedAt = ts
	}
	return biz.EvalMemoryItem{ID: row.ID, Content: row.Statement, Score: score, Timestamp: ts, CreatedAt: row.CreatedAt}, true
}

// evalFactEnvelope wraps evalItemFromFactJSON with the evidence-day metadata
// (metadata.event_dates[0] else valid_from else created_at) and the source
// session for the Q9 evidence prefix.
func evalFactEnvelope(raw []byte, fallback float64) (evalCollectedItem, bool) {
	item, ok := evalItemFromFactJSON(raw, fallback)
	if !ok {
		return evalCollectedItem{}, false
	}
	meta := factJSONString(raw, "metadata_json")
	sess := factJSONString(raw, "source_session_id")
	day := EvalEvidenceDate(meta, item.Timestamp, item.CreatedAt)
	return evalCollectedItem{item: item, day: day, sess: sess}, true
}

func evalItemFromEpisodeJSON(raw []byte) (biz.EvalMemoryItem, bool) {
	var row struct {
		ID             string `json:"id"`
		Title          string `json:"title"`
		OutcomeSummary string `json:"outcome_summary"`
		CreatedAt      string `json:"created_at"`
		Scores         struct {
			Total float64 `json:"total"`
		} `json:"scores"`
	}
	if json.Unmarshal(raw, &row) != nil || row.ID == "" {
		return biz.EvalMemoryItem{}, false
	}
	content := strings.TrimSpace(row.OutcomeSummary)
	if content == "" {
		content = row.Title
	}
	return biz.EvalMemoryItem{ID: "l2:" + row.ID, Content: content, Score: row.Scores.Total, Timestamp: row.CreatedAt, CreatedAt: row.CreatedAt}, true
}

// evalEpisodeEnvelope wraps evalItemFromEpisodeJSON with the same evidence
// metadata used for fact rows.
func evalEpisodeEnvelope(raw []byte, fallback float64) (evalCollectedItem, bool) {
	item, ok := evalItemFromEpisodeJSON(raw)
	if !ok {
		return evalCollectedItem{}, false
	}
	if item.Score == 0 {
		item.Score = fallback
	}
	meta := factJSONString(raw, "metadata_json")
	sess := factJSONString(raw, "session_id")
	day := EvalEvidenceDate(meta, "", item.CreatedAt)
	return evalCollectedItem{item: item, day: day, sess: sess}, true
}

func factJSONString(raw []byte, key string) string {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	v, _ := m[key].(string)
	return v
}

func evalJSONStrings(rows [][]byte, key string) []string {
	out := make([]string, 0, len(rows))
	for _, raw := range rows {
		if v := factJSONString(raw, key); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func uniqueNonEmpty(in []string, capN int) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
		if capN > 0 && len(out) >= capN {
			break
		}
	}
	return out
}

func parseJSONStringSlice(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return nil
	}
	var out []string
	if json.Unmarshal([]byte(raw), &out) != nil {
		return nil
	}
	return out
}
