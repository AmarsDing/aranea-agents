package data

import (
	"fmt"
	"time"

	"aranea-agents/internal/biz"
)

// evalCandidateRangeSQL mirrors filterEvalItemsByRange before LIMIT. Only
// user-scoped candidates reach it; an old/new recency window cannot hide an
// event in the middle of a large history. Unknown source times stay unknown.
func evalCandidateRangeSQL(qa biz.EvalQueryAnalysis, args *[]any) string {
	if qa.RangeStart == nil && qa.RangeEnd == nil {
		return ""
	}
	start, end := "", ""
	if qa.RangeStart != nil {
		start = qa.RangeStart.Format("2006-01-02")
	}
	if qa.RangeEnd != nil {
		end = qa.RangeEnd.Format("2006-01-02")
	}
	*args = append(*args, start, end)
	s, e := fmt.Sprintf("$%d", len(*args)-1), fmt.Sprintf("$%d", len(*args))
	meta := `metadata_json::jsonb`
	spans := `CASE WHEN jsonb_typeof(` + meta + `->'event_spans')='array' THEN ` + meta + `->'event_spans' ELSE '[]'::jsonb END`
	dates := `CASE WHEN jsonb_typeof(` + meta + `->'event_dates')='array' THEN ` + meta + `->'event_dates' ELSE '[]'::jsonb END`
	day := `LEFT(CASE WHEN ` + meta + ` ? 'source_timestamp' THEN COALESCE(NULLIF(` + meta + `->>'source_local_timestamp',''),` + meta + `->>'source_timestamp','') ELSE COALESCE(NULLIF(valid_from,''),created_at) END,10)`
	spanMatch := `EXISTS (SELECT 1 FROM jsonb_array_elements(` + spans + `) p WHERE (` + s + `='' OR p->>'end'>=` + s + `) AND (` + e + `='' OR p->>'start'<=` + e + `))`
	dateMatch := `EXISTS (SELECT 1 FROM jsonb_array_elements_text(` + dates + `) d WHERE (` + s + `='' OR d>=` + s + `) AND (` + e + `='' OR d<=` + e + `))`
	pointMatch := `(` + day + `<>'' AND (` + s + `='' OR ` + day + `>=` + s + `) AND (` + e + `='' OR ` + day + `<=` + e + `))`
	undatedMatch := pointMatch
	if qa.Historical {
		startAt, endAt := "", ""
		if qa.RangeStart != nil {
			startAt = qa.RangeStart.Format(time.RFC3339Nano)
		}
		if qa.RangeEnd != nil {
			endAt = qa.RangeEnd.AddDate(0, 0, 1).Format(time.RFC3339Nano)
		}
		*args = append(*args, startAt, endAt)
		sa, ea := fmt.Sprintf("$%d", len(*args)-1), fmt.Sprintf("$%d", len(*args))
		from := `CASE WHEN ` + meta + ` ? 'source_timestamp' THEN NULLIF(` + meta + `->>'source_timestamp','') ELSE COALESCE(NULLIF(valid_from,''),NULLIF(created_at,'')) END`
		stateMatch := `((` + from + `) IS NOT NULL AND (` + ea + `='' OR (` + from + `)::timestamptz<NULLIF(` + ea + `,'')::timestamptz) AND (valid_until='' OR ` + sa + `='' OR NULLIF(valid_until,'')::timestamptz>NULLIF(` + sa + `,'')::timestamptz))`
		undatedMatch = `CASE WHEN fact_kind IN ('user_identity','preference','user_preference','constraint','agent_instruction') THEN ` + stateMatch + ` ELSE ` + pointMatch + ` END`
	}
	return ` AND (` + spanMatch + ` OR (jsonb_array_length(` + spans + `)=0 AND (` + dateMatch + ` OR (jsonb_array_length(` + dates + `)=0 AND (` + undatedMatch + `)))))`
}
