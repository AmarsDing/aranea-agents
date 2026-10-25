package data

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func evalCandidateLimit(topK int32) int32 {
	if topK <= 0 {
		topK = 100
	}
	if topK > 200 {
		return 800
	}
	if topK < 16 {
		return 64
	}
	return topK * 4
}

func evalSameRange(a, b biz.EvalQueryAnalysis) bool {
	day := func(t *time.Time) string {
		if t == nil {
			return ""
		}
		return t.Format("2006-01-02")
	}
	return day(a.RangeStart) == day(b.RangeStart) && day(a.RangeEnd) == day(b.RangeEnd)
}

func (s *evalMemoryStore) evalSearchAnchor(ctx context.Context, userID string, fallback time.Time) (time.Time, bool, error) {
	var source string
	err := queryRowScan(ctx, s.facts.data.RWDB().ReadDB(ctx), `SELECT COALESCE((SELECT COALESCE(NULLIF(metadata_json::jsonb->>'source_local_timestamp',''),
		CASE WHEN metadata_json::jsonb ? 'source_timestamp' THEN NULLIF(metadata_json::jsonb->>'source_timestamp','') ELSE NULLIF(valid_from,'') END,'')
		FROM memory_facts WHERE scope_type='user' AND scope_id=$1 AND user_id=$1 AND deleted_at='' AND source_kind<>'agent_memory_challenge_derived'
		AND CASE WHEN metadata_json::jsonb ? 'source_timestamp' THEN NULLIF(metadata_json::jsonb->>'source_timestamp','') ELSE NULLIF(valid_from,'') END IS NOT NULL
		ORDER BY (CASE WHEN metadata_json::jsonb ? 'source_timestamp' THEN NULLIF(metadata_json::jsonb->>'source_timestamp','') ELSE NULLIF(valid_from,'') END)::timestamptz DESC,created_at DESC,id ASC LIMIT 1),'')`, []any{userID}, &source)
	if err != nil {
		return fallback, false, err
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999Z07:00", "2006-01-02 15:04:05.999999999Z07", "2006-01-02 15:04:05Z07"} {
		if t, err := time.Parse(layout, source); err == nil {
			return t, true, nil
		}
	}
	return fallback, false, nil
}

func evalTopicQuery(query string) string {
	q := strings.ToLower(query)
	for _, phrase := range []string{"summarize", "summarise", "summary", "overall", "throughout", "all conversations", "conversation history", "entire history", "over time", "please", "we discussed", "what do you know about", "what do you remember about", "profile", "conversations", "conversation", "history", "总结一下", "总结", "概括一下", "概括", "总体", "所有", "全部", "介绍一下", "用户的信息", "我们聊了什么", "我们聊过的", "对话历史", "历史对话", "请"} {
		q = strings.ReplaceAll(q, phrase, " ")
	}
	return strings.TrimSpace(q)
}

func evalMeaningfulTokens(query string) []string {
	var out []string
	for _, token := range tokenizeQuery(query) {
		low := strings.ToLower(token)
		if _, stop := recallStopwords[low]; stop {
			continue
		}
		if low == "myself" || low == "us" || low == "我" || low == "个人信息" {
			continue
		}
		out = append(out, token)
	}
	return out
}

func evalQueryTokens(queries []string) []string {
	var tokens []string
	for _, q := range queries {
		tokens = append(tokens, tokenizeQuery(q)...)
		tokens = append(tokens, biz.EvalRuleAnchors(q)...)
	}
	return uniqueNonEmpty(tokens, 64)
}

func evalLikePattern(token string) string {
	return "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(strings.ToLower(token)) + "%"
}

var evalCalendarDateText = regexp.MustCompile(`(?i)\d{4}年(?:\d{1,2}月(?:\d{1,2}日?)?)?|\d{1,2}月\d{1,2}日|\d{4}-\d{1,2}(?:-\d{1,2})?|\d{1,2}/\d{1,2}/\d{4}|\b(?:last|this|next)\s+(?:week|month|year)\b|\b(?:today|yesterday|tomorrow)\b|(?:上|本|下)(?:个)?(?:周|月)|这个月|去年|今年|明年|昨天|今天|明天|\b(?:jan(?:uary)?|feb(?:ruary)?|mar(?:ch)?|apr(?:il)?|may|jun(?:e)?|jul(?:y)?|aug(?:ust)?|sep(?:tember)?|oct(?:ober)?|nov(?:ember)?|dec(?:ember)?)\s+(?:\d{1,2}(?:st|nd|rd|th)?[,]?\s+\d{4}|\d{4}|\d{1,2}(?:st|nd|rd|th)?)\b|\b[12]\d{3}\b`)

func evalCalendarTopic(query string) string {
	q := evalCalendarDateText.ReplaceAllString(strings.ToLower(query), " ")
	for _, phrase := range []string{"what happened", "what did i do", "what events", "what activities", "anything", "events", "occurred", "during", "我做了什么", "做了什么", "发生了什么", "的事", "那天", "当天"} {
		q = strings.ReplaceAll(q, phrase, " ")
	}
	return q
}

// Scope is applied before all history/rule expansion. SQL relevance ordering
// considers the entire user's history instead of a recent-only 200-row window.
func (s *evalMemoryStore) appendEvalCandidates(ctx context.Context, userID string, queries []string, qa biz.EvalQueryAnalysis, limit int, appendRow func([]byte, float64)) error {
	tokens := evalQueryTokens(queries)
	if qa.Relational {
		tokens = append(tokens, biz.EvalEvidenceEntities(queries[0])...)
	}
	tokens = uniqueNonEmpty(append(tokens, biz.EvalBackgroundHints(queries[0])...), 96)
	args := []any{userID}
	var terms []string
	for _, token := range tokens {
		args = append(args, evalLikePattern(token))
		column := "statement_normalized"
		if strings.HasSuffix(token, "(") {
			column = "regexp_replace(statement_normalized,'[[:space:]]','','g')"
		}
		terms = append(terms, fmt.Sprintf("CASE WHEN %s LIKE $%d THEN 1 ELSE 0 END", column, len(args)))
	}
	match := "0::int"
	if len(terms) > 0 {
		match = "(" + strings.Join(terms, "+") + ")"
	}
	where := ` WHERE scope_type='user' AND scope_id=$1 AND user_id=$1 AND status='active' AND deleted_at='' AND source_kind NOT IN ('agent_memory_challenge_control','agent_memory_challenge_derived')`
	if !qa.Historical {
		where += ` AND valid_until=''`
	}
	where += evalCandidateRangeSQL(qa, &args)
	if qa.Procedural {
		match += `+CASE WHEN fact_kind IN ('constraint','agent_instruction') THEN 1 ELSE 0 END`
	}
	args = append(args, limit)
	direction := "DESC"
	if qa.Aggregate || qa.Historical {
		direction = "ASC"
	}
	order := `(` + match + `) DESC, COALESCE(NULLIF(valid_from,''),created_at) ` + direction + `, id ASC`
	if qa.Aggregate {
		order = `ROW_NUMBER() OVER (PARTITION BY source_session_id ORDER BY ` + order + `), ` + order
	}
	q := sqlFactSelectSQL(false) + where + ` ORDER BY ` + order + ` LIMIT ` + fmt.Sprintf("$%d", len(args))
	rows, err := s.facts.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, args...)
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

func rankEvalEvidence(items []evalCollectedItem, queries []string, qa biz.EvalQueryAnalysis) []evalCollectedItem {
	broad := qa.Aggregate && len(evalMeaningfulTokens(evalTopicQuery(queries[0]))) == 0
	calendarOnly := (qa.RangeStart != nil || qa.RangeEnd != nil) && len(evalMeaningfulTokens(evalCalendarTopic(queries[0]))) == 0
	allTokens := make([][]string, len(queries))
	ruleAnchors := map[string]bool{}
	for i, q := range queries {
		allTokens[i] = tokenizeQuery(q)
		if qa.Relational {
			allTokens = append(allTokens, biz.EvalEvidenceEntities(q))
		}
		if anchors := biz.EvalRuleAnchors(q); len(anchors) > 0 {
			// A short function identifier is often the only literal bridge
			// from a new input to its stored demonstrations.
			allTokens = append(allTokens, anchors)
			for _, anchor := range anchors {
				ruleAnchors[anchor] = true
			}
		}
	}
	out := items[:0]
	for _, env := range items {
		if (env.invalidUntil != "" || env.historical) && !qa.Historical {
			continue
		}
		if strings.HasPrefix(env.item.ID, "l2:") {
			env.item.Content = evalEpisodeContentForQuery(env.item.Content, queries[0])
		}
		if strings.TrimSpace(env.item.Content) == "" || !biz.EvalEvidenceApplicable(env.item.Content, queries[0]) {
			continue
		}
		lexical := 0.0
		for _, tokens := range allTokens {
			lexical = math.Max(lexical, keywordOverlapScore(tokens, env.item.Content))
		}
		if len(ruleAnchors) > 0 {
			for _, anchor := range biz.EvalRuleAnchors(env.item.Content) {
				if ruleAnchors[anchor] {
					lexical = math.Max(lexical, 1)
					break
				}
			}
		}
		relevance := math.Max(lexical, env.semantic)
		if !strings.HasPrefix(env.item.ID, "l2:") && biz.EvalPersonalBackgroundRelevant(env.item.Content, queries[0]) {
			relevance = math.Max(relevance, 0.7)
		}
		if broad {
			relevance = math.Max(relevance, 0.4)
		}
		if calendarOnly {
			relevance = math.Max(relevance, 0.4)
		}
		if qa.Procedural && (env.kind == "constraint" || env.kind == "agent_instruction") {
			relevance = math.Max(relevance, 0.5)
		}
		if relevance <= 0 || math.IsNaN(relevance) || math.IsInf(relevance, 0) {
			continue
		}
		env.relevance = relevance
		// Importance/recency may break ties but can never create relevance.
		env.item.Score = 0.85*relevance + 0.15*math.Max(0, math.Min(1, env.item.Score))
		out = append(out, env)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].item.Score != out[j].item.Score {
			return out[i].item.Score > out[j].item.Score
		}
		return out[i].item.ID < out[j].item.ID
	})
	return out
}

func (s *evalMemoryStore) listEvalAggregateEpisodes(ctx context.Context, userID, query string) ([][]byte, error) {
	tokens := uniqueNonEmpty(tokenizeQuery(evalTopicQuery(query)), 32)
	args := []any{userID}
	var terms []string
	for _, token := range tokens {
		args = append(args, evalLikePattern(token))
		terms = append(terms, fmt.Sprintf("CASE WHEN lower(outcome_summary) LIKE $%d THEN 1 ELSE 0 END", len(args)))
	}
	match := "0::int"
	if len(terms) > 0 {
		match = strings.Join(terms, "+")
	}
	args = append(args, evalAggregateRecentEpisodes)
	q := sqlEpisodeSelect + ` WHERE agent_id=$1 AND deleted_at='' ORDER BY (` + match + `) DESC, created_at ASC, id ASC LIMIT ` + fmt.Sprintf("$%d", len(args))
	rows, err := s.episodes.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		raw, err := scanEpisodeRowJSON(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, raw)
	}
	return out, rows.Err()
}

// Context expansion happens only after an actual evidence seed passes the
// floor. Stored siblings preserve short examples, rule exceptions and dialogue
// attribution that may share no query words of their own.
func (s *evalMemoryStore) appendEvalContext(ctx context.Context, userID, query string, items []evalCollectedItem, qa biz.EvalQueryAnalysis, topK int32) []evalCollectedItem {
	seen := map[string]bool{}
	for _, env := range items {
		seen[env.item.ID] = true
	}
	parents := map[string]evalCollectedItem{}
	var ids []string
	for i, seed := range items {
		if i >= evalLinkHopTop {
			break
		}
		for _, id := range uniqueNonEmpty(parseJSONStringSlice(factJSONString(seed.raw, "links")), 24) {
			if seen[id] {
				continue
			}
			if old, ok := parents[id]; !ok {
				ids = append(ids, id)
				parents[id] = seed
			} else if seed.item.Score > old.item.Score {
				parents[id] = seed
			}
		}
	}
	if len(ids) > 64 {
		ids = ids[:64]
	}
	if len(ids) == 0 {
		return items
	}
	rows, err := s.facts.queryFactRowsByIDs(ctx, ids, "user", userID, userID, false)
	if err != nil {
		s.lg.Warn("eval context expansion skipped", loggateway.StepID("memoryeval.context"), loggateway.Err(err))
		return items
	}
	defer rows.Close()
	for rows.Next() {
		raw, err := scanFactRowJSON(rows)
		if err != nil {
			s.lg.Warn("eval context row skipped", loggateway.StepID("memoryeval.context"), loggateway.Err(err))
			continue
		}
		env, ok := evalFactEnvelope(raw, 0)
		if !ok || seen[env.item.ID] || env.invalidUntil != "" && !qa.Historical {
			continue
		}
		if env.historical && !qa.Historical || !biz.EvalEvidenceApplicable(env.item.Content, query) {
			continue
		}
		seed := parents[env.item.ID]
		env.item.Score = seed.item.Score * 0.85
		env.relevance = seed.relevance * 0.85
		seen[env.item.ID] = true
		items = append(items, env)
	}
	if err := rows.Err(); err != nil {
		s.lg.Warn("eval context expansion incomplete", loggateway.StepID("memoryeval.context"), loggateway.Err(err))
	}
	return items
}

func evalEvidenceExcerpt(content string, queries []string, budget int) string {
	return evalRelevantWindows(content, queries, budget)
}
