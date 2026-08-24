package data

import (
	"context"
	"encoding/json"
	"strconv"
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
	evalEpisodeMaxChars = 4000
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
	for i, m := range msgs {
		statement := m.Content
		if m.Role != "" {
			statement = m.Role + ": " + m.Content
		}
		kind := biz.ClassifyEvalFactKind(statement)
		eventAt := normalizeEvalTimestamp(m.Timestamp)
		imp := 0.5
		if kind != "event" {
			imp = 0.7
		}
		if err := s.supersedeContradictions(ctx, userID, kind, statement); err != nil {
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
			MetadataJSON:    evalMetadataJSON(sessionID, i),
		})
		if err != nil {
			return stored, err
		}
		stored++
		id := factJSONString(raw, "id")
		written = append(written, evalWrittenFact{id: id, raw: raw})
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

func (s *evalMemoryStore) SearchMemories(ctx context.Context, userID, query string, topK int32) ([]biz.EvalMemoryItem, error) {
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
	var items []biz.EvalMemoryItem
	appendRow := func(raw []byte, fallbackScore float64) {
		item, ok := evalItemFromFactJSON(raw, fallbackScore)
		if !ok || item.ID == "" {
			return
		}
		if _, dup := seen[item.ID]; dup {
			return
		}
		seen[item.ID] = struct{}{}
		items = append(items, item)
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
				if item, ok := evalItemFromEpisodeJSON(raw); ok {
					if _, dup := seen[item.ID]; dup {
						continue
					}
					seen[item.ID] = struct{}{}
					items = append(items, item)
				}
			}
		}
	}
	return evalMMRCap(items, topK), nil
}

func (s *evalMemoryStore) supersedeContradictions(ctx context.Context, userID, newKind, newStmt string) error {
	if !evalGovernableKindForStore(newKind) {
		return nil
	}
	rows, err := s.facts.ListFactRowsForUser(ctx, "user", userID, userID, "", evalSupersedeLook, 0)
	if err != nil {
		return err
	}
	for _, raw := range rows {
		old := decodeEvalFactRow(raw)
		if old.id == "" || old.statement == "" {
			continue
		}
		if !biz.ShouldSupersedeEvalFact(old.kind, old.statement, newKind, newStmt) {
			continue
		}
		if _, invErr := s.facts.InvalidateFact(ctx, old.id); invErr != nil {
			s.lg.Warn("eval invalidate failed",
				loggateway.StepID("memoryeval.invalidate"), loggateway.Err(invErr),
				loggateway.Str("fact_id", old.id))
		}
	}
	return nil
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

func evalMMRCap(items []biz.EvalMemoryItem, topK int32) []biz.EvalMemoryItem {
	if len(items) == 0 {
		return items
	}
	lim := int(topK)
	if lim <= 0 {
		lim = 100
	}
	texts := make([]string, len(items))
	scores := make([]float64, len(items))
	for i, it := range items {
		texts[i] = it.Content
		scores[i] = it.Score
	}
	order := biz.MMRRerankTexts(texts, scores, lim, evalMMRLambda)
	out := make([]biz.EvalMemoryItem, 0, len(order))
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
	id        string
	statement string
	kind      string
}

func decodeEvalFactRow(raw []byte) evalFactRow {
	var row struct {
		ID        string `json:"id"`
		Statement string `json:"statement"`
		FactKind  string `json:"fact_kind"`
	}
	if json.Unmarshal(raw, &row) != nil {
		return evalFactRow{}
	}
	return evalFactRow{id: row.ID, statement: row.Statement, kind: row.FactKind}
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
	return biz.EvalMemoryItem{ID: row.ID, Content: row.Statement, Score: score, Timestamp: ts}, true
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
	return biz.EvalMemoryItem{ID: "l2:" + row.ID, Content: content, Score: row.Scores.Total, Timestamp: row.CreatedAt}, true
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

func truncateRunes(s string, n int) string {
	if n <= 0 || s == "" {
		return s
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}

// normalizeEvalTimestamp accepts RFC3339(Nano) strings or epoch seconds/millis
// and returns RFC3339Nano; unparseable input yields "" (repo defaults to now).
func normalizeEvalTimestamp(ts string) string {
	ts = strings.TrimSpace(ts)
	if ts == "" {
		return ""
	}
	if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
		return t.UTC().Format(time.RFC3339Nano)
	}
	if n, err := strconv.ParseInt(ts, 10, 64); err == nil {
		if n > 1e12 { // epoch millis
			return time.UnixMilli(n).UTC().Format(time.RFC3339Nano)
		}
		return time.Unix(n, 0).UTC().Format(time.RFC3339Nano)
	}
	return ""
}

func evalMetadataJSON(sessionID string, seq int) string {
	b, err := json.Marshal(map[string]any{"eval_session": sessionID, "eval_seq": seq})
	if err != nil {
		return "{}"
	}
	return string(b)
}
