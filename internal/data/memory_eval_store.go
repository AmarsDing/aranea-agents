package data

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

const (
	evalMMRLambda       = 0.7
	evalL2SearchLimit   = 20
	evalSessionHopTop   = 5
	evalSessionHopLimit = 12
	evalLinkHopTop      = 8
	evalSupersedeLook   = 2048
	evalEpisodeMaxChars = 32768
	// evalRecallMinScore is the deterministic evidence floor for the eval
	// search path (AMC C3-A Q9): biz.AdaptiveRecallMinScore treats a
	// configured value <= 0 as "no floor", so the eval adapter passes a
	// positive base (effective = max(base, top1*0.6)) and filters strictly —
	// an empty result stays empty, no backfill padding.
	evalRecallMinScore = 0.12
	// evalAggregateRecentEpisodes is how many recent L2 episodes the
	// aggregate query mode (总结/总体/overall/…) merges on top of fact hits.
	evalAggregateRecentEpisodes = 64
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
	facts       *l3FactRepo
	episodes    *l2EpisodeRepo
	vec         *biz.MemoryUsecase
	syncer      biz.MemoryFactIndexSyncer
	lg          loggateway.Logger
	emb         biz.EmbeddingService
	routerMu    sync.Mutex
	router      *biz.EvalIntentRouter
	routerErrAt time.Time
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
		emb:      emb,
	}
	if emb != nil {
		s.vec = biz.NewMemoryUsecase(NewMemoryRepo(d), emb)
		s.syncer = NewMemoryFactIndexSync(s.vec, d, lg)
	}
	return s
}

func (s *evalMemoryStore) addMessages(ctx context.Context, userID, sessionID string, msgs []biz.EvalMessage, indexed *[]evalWrittenFact) (int, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		sessionID = "eval-default"
	}
	stored := 0
	var written []evalWrittenFact
	var derivedWritten []evalWrittenFact
	touchedSlots := map[string]struct{}{}
	changed := false
	batchIDs := map[string]struct{}{}
	policies, err := s.evalMemoryPolicies(ctx, userID)
	if err != nil {
		return 0, err
	}
	for i, m := range msgs {
		eventAt := normalizeEvalTimestamp(m.Timestamp)
		anchor := time.Now().UTC()
		if eventAt != "" {
			if t, err := time.Parse(time.RFC3339Nano, eventAt); err == nil {
				anchor = t
			}
			if local, err := time.Parse(time.RFC3339Nano, m.Timestamp); err == nil {
				anchor = local // Calendar words use the source's own offset.
			}
		}
		// A1/Q4 atomization: split each message into atomic statements, each
		// atom keeps the role prefix and is classified / superseded / stored
		// independently; stored still counts messages, not atoms.
		atoms := biz.SplitEvalStatements(m.Content, biz.EvalMaxStatementAtoms)
		_, directControl := biz.ParseEvalMemoryDirective(m.Role, m.Content)
		controlMessage := len(atoms) == 1 && directControl
		var atomEmbeddings, routeEmbeddings map[int][]float32
		if !controlMessage && s.emb != nil {
			statements := make([]string, len(atoms))
			for j, atom := range atoms {
				statements[j] = atom
				if m.Role != "" {
					statements[j] = m.Role + ": " + atom
				}
			}
			atomEmbeddings, routeEmbeddings = s.embedEvalStatements(ctx, statements)
		}
		var headingSpans []biz.EvalDateSpan
		for j, atom := range atoms {
			if controlMessage {
				if directive, ok := biz.ParseEvalMemoryDirective(m.Role, atom); ok {
					policy, created, err := s.applyEvalMemoryDirective(ctx, userID, sessionID, m, j, directive)
					if err != nil {
						return stored, err
					}
					if created {
						policies = append(policies, policy)
						changed = true
					}
					continue
				}
			}
			// AMC C3 修复 #3：相对时间按原子解析（此前只用 atoms[0] 的结果
			// 复用到全部原子，「…。前天…。」第二枚原子丢失/错配 event_dates，
			// Q6 区间/时序题与 Q9 前缀全部错位）。
			atomDates := biz.ResolveEvalRelativeTimes(stripEvalRolePrefix(atom), anchor)
			statement := atom
			if m.Role != "" {
				statement = m.Role + ": " + atom
			}
			if evalPolicyBlocks(policies, statement, eventAt) {
				continue
			}
			spans := biz.ResolveEvalDateSpans(atom, anchor)
			if eventAt == "" {
				spans = evalAbsoluteSpans(atom)
				atomDates = nil
			}
			if heading := biz.EvalDateHeadingSpans(atom, anchor); len(heading) > 0 {
				if eventAt == "" {
					heading = evalAbsoluteSpans(atom)
				}
				headingSpans = heading
			} else if len(spans) == 0 && len(headingSpans) > 0 {
				spans = append([]biz.EvalDateSpan(nil), headingSpans...)
			}
			fingerprint := evalOccurrenceFingerprint(userID, sessionID, m, j, statement)
			if exists, err := s.evalOccurrenceExists(ctx, userID, fingerprint); err != nil {
				return stored, err
			} else if exists {
				continue
			}
			kind := biz.ClassifyEvalFactKind(statement)
			imp := 0.5
			if kind != "event" {
				imp = 0.7
			}
			prevs, validUntil, err := s.supersedeContradictions(ctx, userID, kind, statement, eventAt, batchIDs, atomEmbeddings[j], routeEmbeddings[j])
			if err != nil {
				return stored, err
			}
			raw, err := s.facts.UpsertFactRow(ctx, biz.FactUpsert{
				ScopeType:       "user",
				ScopeID:         userID,
				UserID:          userID,
				AgentID:         userID,
				Statement:       statement,
				Fingerprint:     fingerprint,
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
				ValidUntil:      validUntil,
				SkipPIIRedact:   true,
				MetadataJSON:    evalSourceMetadata(sessionID, i, j, atomDates, prevs, spans, eventAt, anchor.Format(time.RFC3339Nano), statement),
			})
			if err != nil {
				return stored, err
			}
			id := factJSONString(raw, "id")
			if id != "" {
				batchIDs[id] = struct{}{}
			}
			written = append(written, evalWrittenFact{id: id, raw: raw})
			changed = true
			if slot := biz.EvalFactSlot(statement); slot != "" {
				touchedSlots[slot] = struct{}{}
			}
			for _, prev := range prevs {
				if slot := biz.EvalFactSlot(prev.Statement); slot != "" {
					touchedSlots[slot] = struct{}{}
				}
			}
			if err := s.upsertNumericDigests(ctx, userID, sessionID, i, j, statement, eventAt, atomDates, batchIDs, &derivedWritten); err != nil {
				return stored, err
			}
		}
		stored++
	}
	if err := s.linkEvalSiblings(ctx, written); err != nil {
		return stored, err
	}
	if len(written) > 0 {
		active, err := s.evalActiveWrittenMessages(ctx, userID, written)
		if err != nil {
			return stored, err
		}
		if err := s.upsertEvalEpisode(ctx, userID, sessionID, active); err != nil {
			return stored, err
		}
	}
	if changed {
		if err := s.rollupEvalDerivedRows(ctx, userID, sessionID, touchedSlots, batchIDs, &derivedWritten); err != nil {
			return stored, err
		}
	}
	*indexed = append(written, derivedWritten...)
	return stored, nil
}

// evalCollectedItem is one search hit plus its evidence-dating metadata
// (metadata.event_dates[0] else valid_from else created_at) used by the
// range filter, chronological ordering and the Q9 evidence prefix.
type evalCollectedItem struct {
	item         biz.EvalMemoryItem
	day          string
	sess         string
	raw          []byte
	spans        []biz.EvalDateSpan
	sourceTime   string
	messageIndex int
	atomIndex    int
	invalidUntil string
	semantic     float64
	relevance    float64
	kind         string
	historical   bool
	evidenceUnit bool
	support      string
}

func (c evalCollectedItem) prefix() string {
	day := c.day
	if len(c.spans) > 0 && c.spans[0].Start != c.spans[0].End {
		day = c.spans[0].Start + ".." + c.spans[0].End
	}
	return evalItemPrefix(day, c.sess)
}

func (s *evalMemoryStore) SearchMemories(ctx context.Context, userID, query string, topK int32, options ...string) ([]biz.EvalMemoryItem, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, apierror.BadRequest("MEMORY_EVAL", "user_id is required")
	}
	now := time.Now().UTC()
	anchor, known, err := s.evalSearchAnchor(ctx, userID, now)
	if err != nil {
		return nil, err
	}
	qa := biz.AnalyzeEvalQuery(query, anchor)
	if !known {
		a, b := biz.AnalyzeEvalQuery(query, now), biz.AnalyzeEvalQuery(query, now.AddDate(-3, -2, -1))
		if !evalSameRange(a, b) {
			qa.RangeStart, qa.RangeEnd = nil, nil
		}
	}
	queries := biz.EvalRetrievalQueries(query, options...)
	var embedding []float32
	if s.vec != nil {
		if v, err := s.vec.EmbedText(ctx, query); err == nil {
			embedding = v
		} else {
			s.lg.Warn("eval query embed failed, degrade to keyword recall",
				loggateway.StepID("memoryeval.search_degrade"), loggateway.Err(err))
		}
	}
	poolLimit := evalCandidateLimit(topK)
	s.applyEvalQueryModes(ctx, &qa, embedding)
	// A future horizon in a trend question asks for a forecast, not evidence
	// recorded in that future window. Keep the observed series available.
	if qa.Trend && qa.RangeStart != nil && qa.RangeStart.After(anchor) {
		qa.RangeStart, qa.RangeEnd = nil, nil
	}
	rows, err := s.facts.RecallL3Facts(ctx, "user", userID, userID, query, embedding, poolLimit, 0)
	if err != nil {
		return nil, err
	}
	for _, expanded := range queries[1:] {
		var optionEmbedding []float32
		if s.vec != nil {
			var embedErr error
			optionEmbedding, embedErr = s.vec.EmbedText(ctx, expanded)
			if embedErr != nil {
				s.lg.Warn("eval option embedding failed, using lexical evidence", loggateway.StepID("memoryeval.options"), loggateway.Err(embedErr))
			}
		}
		extra, err := s.facts.RecallL3Facts(ctx, "user", userID, userID, expanded, optionEmbedding, poolLimit, 0)
		if err != nil {
			return nil, err
		}
		rows = append(rows, extra...)
	}
	seen := map[string]struct{}{}
	seenIndex := map[string]int{}
	var items []evalCollectedItem
	appendRow := func(raw []byte, fallback float64) {
		env, ok := evalFactEnvelope(raw, fallback)
		if !ok || env.item.ID == "" {
			return
		}
		if evalIsDerivedRaw(raw) {
			if env.invalidUntil != "" || qa.RangeStart != nil || qa.RangeEnd != nil {
				return
			}
			allowed := qa.Aggregate && biz.IsEvalProfileDigestStatement(env.item.Content) ||
				(qa.Aggregate || qa.Historical) && biz.IsEvalSlotHistoryStatement(env.item.Content) ||
				qa.Trend && biz.IsEvalSeriesDigestStatement(env.item.Content)
			if !allowed {
				return
			}
		}
		if _, dup := seen[env.item.ID]; dup {
			idx := seenIndex[env.item.ID]
			if env.semantic > items[idx].semantic {
				items[idx].semantic = env.semantic
			}
			if env.item.Score > items[idx].item.Score {
				items[idx].item.Score = env.item.Score
			}
			return
		}
		seen[env.item.ID] = struct{}{}
		seenIndex[env.item.ID] = len(items)
		items = append(items, env)
	}
	for _, raw := range rows {
		appendRow(raw, 0)
	}
	if err := s.appendEvalDerivedCandidates(ctx, userID, qa, appendRow); err != nil {
		return nil, err
	}
	s.appendSessionHop(ctx, userID, rows, appendRow)
	s.appendLinkHop(ctx, userID, rows, appendRow)
	if qa.Historical || qa.Aggregate || qa.Procedural || qa.Relational || qa.Trend || qa.RangeStart != nil || qa.RangeEnd != nil || len(biz.EvalBackgroundFacets(query)) > 0 {
		if err := s.appendEvalCandidates(ctx, userID, queries, qa, int(poolLimit), appendRow); err != nil {
			return nil, err
		}
	}
	if s.episodes != nil && qa.RangeStart == nil && qa.RangeEnd == nil && !qa.Chronological && !qa.Relational {
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
	items = filterEvalItemsByRange(items, qa)
	items = rankEvalEvidence(items, queries, qa)
	// A6/Q9 evidence gating: strict adaptive floor — empty stays empty.
	if qa.Aggregate && len(evalMeaningfulTokens(evalTopicQuery(query))) == 0 {
		items = filterEvalItemsByMinimum(items, evalRecallMinScore)
	} else {
		items = filterEvalItemsByFloor(items, evalRecallMinScore, qa)
	}
	if len(items) == 0 {
		return []biz.EvalMemoryItem{}, nil
	}
	// A3/A5 aggregate mode: additionally merge the user's most recent L2
	// episodes (deduped) as summary evidence. These are deliberately
	// mode-attached and therefore appended after the fact-evidence floor.
	// The dedup set is rebuilt from the surviving items: the default L2 path
	// marks episode ids seen before the floor, and episodes that did not pass
	// the floor must still be attachable here.
	if qa.Aggregate && s.episodes != nil && qa.RangeStart == nil && qa.RangeEnd == nil && !qa.Chronological && !qa.Relational {
		seen = make(map[string]struct{}, len(items))
		for _, env := range items {
			seen[env.item.ID] = struct{}{}
		}
		s.appendAggregateEpisodes(ctx, userID, query, now, seen, &items)
		items = rankEvalEvidence(items, queries, qa)
		items = filterEvalItemsByRange(items, qa)
	}
	items = s.appendEvalContext(ctx, userID, query, items, qa, topK)
	if qa.Relational {
		items = s.appendEvalRelations(ctx, userID, query, items, qa)
	}
	items = filterEvalItemsByRange(items, qa)
	items = evalRemoveCoveredEpisodes(items)
	items = evalEvidenceUnits(items, qa)
	// A3/Q6 chronological mode: order by the evidence day ascending and skip
	// the MMR cap (topK still bounds the result count).
	if qa.Chronological {
		// Select relevant evidence before chronology so an early noisy row
		// cannot displace the only relevant later event at small top_k.
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].item.Score != items[j].item.Score {
				return items[i].item.Score > items[j].item.Score
			}
			return items[i].item.ID < items[j].item.ID
		})
		if lim := int(topK); lim > 0 && len(items) > lim {
			items = items[:lim]
		}
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].day != items[j].day {
				return items[i].day < items[j].day
			}
			if items[i].sourceTime != items[j].sourceTime {
				a, ea := time.Parse(time.RFC3339Nano, items[i].sourceTime)
				b, eb := time.Parse(time.RFC3339Nano, items[j].sourceTime)
				if ea == nil && eb == nil && !a.Equal(b) {
					return a.Before(b)
				}
			}
			if items[i].messageIndex != items[j].messageIndex {
				return items[i].messageIndex < items[j].messageIndex
			}
			if items[i].atomIndex != items[j].atomIndex {
				return items[i].atomIndex < items[j].atomIndex
			}
			return items[i].item.ID < items[j].item.ID
		})
		if lim := int(topK); lim > 0 && len(items) > lim {
			items = items[:lim]
		}
	} else if qa.Relational {
		items = evalRelationalCapItems(items, topK)
	} else if qa.Aggregate {
		broad := len(evalMeaningfulTokens(evalTopicQuery(query))) == 0
		items = evalAggregateCoverageCapItems(items, topK, broad)
	} else if qa.Trend {
		items = evalTrendCapItems(items, topK)
	} else if qa.Enumerative {
		items = evalEnumerativeCapItems(items, topK)
	} else {
		items = evalMMRCapItems(items, topK)
	}
	out := make([]biz.EvalMemoryItem, 0, len(items))
	remaining := 36000
	for i, env := range items {
		if remaining < 128 {
			break
		}
		it := env.item
		// Q9 evidence gating: identifier masking, then the evidence prefix
		// (which goes before the role prefix inside the statement).
		budget := 1800
		if env.evidenceUnit {
			budget = 6000
		}
		// Reserve a fair share for each selected evidence unit so the fixed
		// total budget cannot silently discard all later sources.
		if fair := remaining / (len(items) - i); fair < budget {
			budget = fair
		}
		if remaining < budget {
			budget = remaining
		}
		content := evalEvidenceExcerpt(MaskEvalEvidence(it.Content, query), queries, budget)
		remaining -= len([]rune(content))
		if env.invalidUntil != "" {
			content = "[superseded; valid_until=" + env.invalidUntil + "] " + content
		}
		if env.support != "" {
			content = "[linked_source=" + env.support + "] " + content
		}
		if qa.Chronological || qa.Historical {
			content = "[timestamp=" + env.sourceTime + "; message_index=" + strconv.Itoa(env.messageIndex) + "; atom_index=" + strconv.Itoa(env.atomIndex) + "] " + content
		}
		it.Content = env.prefix() + content
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
		// Linked causal support may precede the question's event window, but
		// cannot supply events strictly after its upper bound.
		if env.support != "" && (endDay == "" || env.day == "" || env.day <= endDay) {
			out = append(out, env)
			continue
		}
		if qa.Historical && len(env.spans) == 0 && evalDurableStateKind(env.kind) {
			if evalStateIntersectsRange(env, qa) {
				out = append(out, env)
			}
			continue
		}
		if len(env.spans) > 0 {
			match := false
			for _, span := range env.spans {
				if (startDay == "" || span.End >= startDay) && (endDay == "" || span.Start <= endDay) {
					match = true
					break
				}
			}
			if match {
				out = append(out, env)
			}
			continue
		}
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
func filterEvalItemsByFloor(items []evalCollectedItem, base float64, analyses ...biz.EvalQueryAnalysis) []evalCollectedItem {
	var qa biz.EvalQueryAnalysis
	if len(analyses) > 0 {
		qa = analyses[0]
	}
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
		if env.item.Score >= floor || evalFloorExempt(env, qa) {
			out = append(out, env)
		}
	}
	return out
}

// appendAggregateEpisodes merges the user's most recent L2 episodes for
// summary-style queries (AMC C3-A Q8a). Episodes carry their deterministic
// keyword/importance score so the downstream cap ranks them consistently.
func (s *evalMemoryStore) appendAggregateEpisodes(ctx context.Context, userID, query string, now time.Time, seen map[string]struct{}, items *[]evalCollectedItem) {
	recent, err := s.listEvalAggregateEpisodes(ctx, userID, query)
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
func (s *evalMemoryStore) supersedeContradictions(ctx context.Context, userID, newKind, newStmt, newEventAt string, batchIDs map[string]struct{}, embeddings ...[]float32) ([]evalPrevValue, string, error) {
	if !evalGovernableKindForStore(newKind) || biz.IsHypotheticalStatement(stripEvalRolePrefix(newStmt)) {
		return nil, "", nil
	}
	correction := HasEvalCorrectionCue(newStmt)
	if len(embeddings) > 1 && len(embeddings[1]) > 0 {
		if router := s.intentRouter(ctx); router != nil {
			correction = correction || router.Match(embeddings[1], biz.EvalIntentCorrection)
		}
	}
	if biz.EvalFactSlot(newStmt) == "" && (s.vec == nil || !correction) {
		return nil, "", nil
	}
	rows, err := s.listEvalGovernableFacts(ctx, userID, newStmt)
	if err != nil {
		return nil, "", err
	}
	var prevs []evalPrevValue
	var successor string
	invalidated := map[string]struct{}{}
	invalidate := func(old evalFactRow) error {
		if old.id == "" || old.statement == "" {
			return nil
		}
		if old.validUntil != "" && (newEventAt == "" || !evalStrictlyAfter(old.validUntil, newEventAt)) {
			return nil
		}
		if _, dup := invalidated[old.id]; dup {
			return nil
		}
		if invErr := s.invalidateEvalOccurrence(ctx, userID, old.id, newEventAt); invErr != nil {
			return invErr
		}
		invalidated[old.id] = struct{}{}
		prevs = append(prevs, evalPrevValue{
			Statement:  old.statement,
			ValidUntil: evalValidityBoundary(newEventAt),
		})
		if old.sourceSessionID != "" && s.episodes != nil {
			// The merged eval episode keeps every appended turn, so a
			// superseded statement must also leave the L2 transcript,
			// otherwise recall keeps surfacing the stale value.
			if stripErr := s.episodes.StripEpisodeStatement(ctx, userID, old.sourceSessionID, old.statement); stripErr != nil {
				return stripErr
			}
		}
		return nil
	}
	for _, raw := range rows {
		old := decodeEvalFactRow(raw)
		if old.id == "" || old.statement == "" {
			continue
		}
		if evalStrictlyAfter(old.validFrom, newEventAt) && biz.ShouldSupersedeEvalFact(newKind, newStmt, old.kind, old.statement) {
			if successor == "" || evalStrictlyAfter(successor, old.validFrom) {
				successor = old.validFrom
			}
		}
		if !biz.ShouldSupersedeEvalFact(old.kind, old.statement, newKind, newStmt) {
			continue
		}
		if !evalTimeAllowsInvalidate(old.validFrom, newEventAt) {
			continue
		}
		if err := invalidate(old); err != nil {
			return nil, "", err
		}
	}
	// Vector-correction pass (Q7): a correction cue plus a semantically near
	// older fact (cosine >= 0.82, normalization-inequivalent) invalidates the
	// stale value even when it occupies a different slot.
	if s.vec == nil || !correction || s.facts.vectorStore == nil {
		return prevs, successor, nil
	}
	var emb []float32
	var embErr error
	if len(embeddings) > 0 {
		emb = embeddings[0]
	}
	if len(emb) == 0 {
		emb, embErr = s.vec.EmbedText(ctx, newStmt)
	}
	if embErr != nil || len(emb) == 0 {
		if embErr != nil {
			s.lg.Warn("eval correction embed failed, vector supersede skipped",
				loggateway.StepID("memoryeval.correction_embed"), loggateway.Err(embErr))
		}
		return prevs, successor, nil
	}
	hits, searchErr := s.facts.vectorStore.Search(ctx, float32To64(emb), evalCorrectionTopK, 0)
	if searchErr != nil {
		s.lg.Warn("eval correction vector search failed, vector supersede skipped",
			loggateway.StepID("memoryeval.correction_search"), loggateway.Err(searchErr))
		return prevs, successor, nil
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
		return prevs, successor, nil
	}
	candRows, qErr := s.facts.queryFactRowsByIDs(ctx, ids, "user", userID, userID, false)
	if qErr != nil {
		return prevs, successor, qErr
	}
	var candidates []evalFactRow
	for candRows.Next() {
		raw, scanErr := scanFactRowJSON(candRows)
		if scanErr != nil {
			continue
		}
		old := decodeEvalFactRow(raw)
		candidates = append(candidates, old)
	}
	rowsErr := candRows.Err()
	candRows.Close()
	if rowsErr != nil {
		return nil, "", rowsErr
	}
	for _, old := range candidates {
		if old.id == "" || old.statement == "" {
			continue
		}
		if biz.NormalizeForDedup(old.statement) == biz.NormalizeForDedup(newStmt) {
			continue
		}
		if !evalTimeAllowsInvalidate(old.validFrom, newEventAt) {
			continue
		}
		if !biz.EvalCorrectionTargetsFact(old.statement, newStmt) {
			continue
		}
		if err := invalidate(old); err != nil {
			return nil, "", err
		}
	}
	return prevs, successor, nil
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
	groups := map[string][]string{}
	for _, w := range written {
		if w.id != "" {
			key := factJSONString(w.raw, "source_message_id")
			var meta struct {
				MessageIndex int    `json:"message_index"`
				SourceTime   string `json:"source_timestamp"`
			}
			if json.Unmarshal([]byte(factJSONString(w.raw, "metadata_json")), &meta) != nil {
				return apierror.Internal("MEMORY_EVAL", "invalid source metadata")
			}
			if key == "" {
				key = "position:" + strconv.Itoa(meta.MessageIndex)
			}
			key = evalStableHash(key, meta.SourceTime)
			groups[key] = append(groups[key], w.id)
		}
	}
	for _, ids := range groups {
		if len(ids) < 2 {
			continue
		}
		payload, err := json.Marshal(ids)
		if err != nil {
			return err
		}
		for _, id := range ids {
			if err := s.facts.setFactLinks(ctx, id, string(payload)); err != nil {
				return err
			}
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
		TaskID:         evalEpisodeTaskID(userID, sessionID),
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
	if factJSONString(raw, "source_kind") == evalControlSource {
		return evalCollectedItem{}, false
	}
	item, ok := evalItemFromFactJSON(raw, fallback)
	if !ok {
		return evalCollectedItem{}, false
	}
	meta := factJSONString(raw, "metadata_json")
	sess := factJSONString(raw, "source_session_id")
	day := EvalEvidenceDate(meta, item.Timestamp, item.CreatedAt)
	var metadata struct {
		Spans        []biz.EvalDateSpan `json:"event_spans"`
		EventDates   []string           `json:"event_dates"`
		SourceTime   *string            `json:"source_timestamp"`
		LocalTime    string             `json:"source_local_timestamp"`
		Historical   bool               `json:"historical_context"`
		Structured   bool               `json:"structured"`
		MessageIndex int                `json:"message_index"`
		AtomIndex    int                `json:"atom_index"`
	}
	if json.Unmarshal([]byte(meta), &metadata) != nil {
		metadata.SourceTime = nil
	}
	if len(metadata.Spans) == 0 {
		for _, date := range metadata.EventDates {
			if d := evalDayPart(date); d != "" {
				metadata.Spans = append(metadata.Spans, biz.EvalDateSpan{Start: d, End: d})
			}
		}
	}
	sourceTime := item.Timestamp
	if metadata.SourceTime != nil {
		sourceTime = *metadata.SourceTime
		item.Timestamp = sourceTime
		if sourceTime == "" {
			day = EvalEvidenceDate(meta, "", "")
		}
	}
	if len(metadata.Spans) == 0 && len(metadata.EventDates) == 0 && metadata.LocalTime != "" {
		if local, err := time.Parse(time.RFC3339Nano, metadata.LocalTime); err == nil {
			day = local.Format("2006-01-02")
		}
	}
	var scores struct {
		Scores struct {
			Vector float64 `json:"vector"`
		} `json:"scores"`
	}
	if json.Unmarshal(raw, &scores) != nil {
		scores.Scores.Vector = 0
	}
	return evalCollectedItem{item: item, day: day, sess: sess, raw: raw, spans: metadata.Spans,
		sourceTime: sourceTime, messageIndex: metadata.MessageIndex, atomIndex: metadata.AtomIndex,
		invalidUntil: factJSONString(raw, "valid_until"), semantic: scores.Scores.Vector, kind: factJSONString(raw, "fact_kind"), historical: metadata.Historical, evidenceUnit: metadata.Structured}, true
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
	if evalEmptySessionEpisode(raw) {
		return evalCollectedItem{}, false
	}
	item, ok := evalItemFromEpisodeJSON(raw)
	if !ok {
		return evalCollectedItem{}, false
	}
	if item.Score == 0 {
		item.Score = fallback
	}
	meta := factJSONString(raw, "metadata_json")
	sess := factJSONString(raw, "session_id")
	day := EvalEvidenceDate(meta, "", "")
	return evalCollectedItem{item: item, day: day, sess: sess, raw: raw}, true
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
