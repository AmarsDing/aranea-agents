package data

import (
	"context"
	"regexp"
	"sort"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// memory_eval_code_pack.go — CAMBench coding-track evidence assembly.
//
// A coding solve trajectory replays hundreds of messages into memory. The
// generic path then applies MMR diversity and splits the fixed rune budget
// evenly across up to top_k fragments, so the answer context became ~90
// pieces of ~400 runes each: the complete final file write, the incremental
// edits and the verified test run were truncated or displaced by
// mid-trajectory exploration that shares the same query words. The coding
// agent saw the right file name but never the reusable fix.
//
// The code pack ranks by outcome value instead of diversity: complete
// patch payloads and the final verified state first, repeated identical
// command/test runs collapse to the latest one, and the result count is
// bounded so the budget loop can keep every surviving unit complete.

// evalCodePackMaxItems bounds a coding answer context. 32 complete units
// still fit the fixed 36000-rune budget with headroom for one large file
// write, its follow-up edits and the final test output.
const (
	evalCodePackMaxItems = 32
	// Reserve roughly 2K for evidence dates/session prefixes inside the
	// SearchMemories 36K content budget.
	evalCodePackMaxRunes = 34000
)

// appendEvalCodeContext anchors a solve query to its best-matching trajectory,
// then fetches every atom of each patch/verified-outcome message in that
// trajectory. This is required for data written before tool calls were kept
// whole: lexical recall may find one escaped JSON fragment, but only the
// complete source message is a usable patch. Restricting the pack to the
// anchor session also prevents same-repository noise trajectories from
// supplying a different implementation.
func evalCodeAnchorScore(env evalCollectedItem, paths, classes []string) float64 {
	score := env.item.Score
	body := strings.ToLower(evalCodePayloadText(env.item.Content))
	for _, path := range paths {
		if strings.Contains(body, strings.ToLower(path)) {
			// An exact source path is much stronger evidence than generic words
			// such as "implementation" or "TimeSeries" in a noise trajectory.
			score += 8
		}
	}
	for _, class := range classes {
		if strings.Contains(body, strings.ToLower(class)) {
			score += 1.5
		}
	}
	return score
}

func (s *evalMemoryStore) evalCodePathSession(ctx context.Context, userID string, paths []string) string {
	for _, path := range paths {
		// A same-repository noise trajectory may mention the path as often as
		// the real solve session. Verified-outcome atoms (clean test run,
		// final summary) exist only in the session that actually fixed the
		// issue, so they break the tie toward the reusable patch.
		rows, err := s.facts.data.RWDB().ReadDB(ctx).QueryContext(ctx, `
			SELECT source_session_id FROM memory_facts
			WHERE scope_type='user' AND scope_id=$1 AND user_id=$1
			AND status='active' AND deleted_at='' AND source_session_id<>''
			AND source_kind NOT IN ('agent_memory_challenge_control','agent_memory_challenge_derived')
			AND strpos(lower(statement),lower($2))>0
			GROUP BY source_session_id
			ORDER BY count(*) + 3*count(*) FILTER (
				WHERE statement ILIKE '% passed in %' OR statement ILIKE '%tests pass%'
				OR statement ILIKE '%implementation is complete%' OR statement ILIKE '%summary of%changes%'
			) DESC, source_session_id LIMIT 1`, userID, path)
		if err != nil {
			continue
		}
		var session string
		if rows.Next() {
			err = rows.Scan(&session)
		}
		rows.Close()
		if err == nil && session != "" {
			return session
		}
	}
	return ""
}

func (s *evalMemoryStore) appendEvalCodeContext(ctx context.Context, userID, query string, items []evalCollectedItem) []evalCollectedItem {
	paths := evalCodeQueryPathRe.FindAllString(query, -1)
	classes := evalCodeQueryClassRe.FindAllString(query, -1)
	// Exact source paths need not rely on lexical recall: the one atom that
	// contains file_path may have been truncated or ranked below the candidate
	// floor, while the remaining atoms alone cannot identify the right session.
	session := s.evalCodePathSession(ctx, userID, paths)
	seedScore := 1.0
	anchorScore := 0.0
	if session == "" {
		seedScore = 0
		for _, env := range items {
			if env.sess == "" || strings.HasPrefix(env.item.ID, "l2:") {
				continue
			}
			score := evalCodeAnchorScore(env, paths, classes)
			if session == "" || score > anchorScore {
				session, seedScore, anchorScore = env.sess, env.item.Score, score
			}
		}
	}
	if session == "" {
		return items
	}
	seen := make(map[string]bool, len(items))
	anchored := make([]evalCollectedItem, 0, len(items)+32)
	for _, env := range items {
		if env.sess == session {
			seen[env.item.ID] = true
			anchored = append(anchored, env)
		}
	}
	codeMessage := `source_message_id IN (
		SELECT source_message_id FROM memory_facts
		WHERE scope_type='user' AND scope_id=$1 AND user_id=$1 AND source_session_id=$2
		AND status='active' AND deleted_at=''
		AND source_kind NOT IN ('agent_memory_challenge_control','agent_memory_challenge_derived')
		AND (statement LIKE '%[tool_use Write]%' OR statement LIKE '%[tool_use Edit]%'
			OR statement LIKE '%[tool_use MultiEdit]%' OR statement LIKE '%diff --git %'
			OR statement ILIKE '% passed in %' OR statement ILIKE '%tests pass%'
			OR statement ILIKE '%implementation is complete%' OR statement ILIKE '%summary of%changes%'
			OR statement ILIKE '%all changes made%'))`
	q := sqlFactSelectSQL(false) + ` WHERE scope_type='user' AND scope_id=$1 AND user_id=$1
		AND source_session_id=$2 AND status='active' AND deleted_at=''
		AND source_kind NOT IN ('agent_memory_challenge_control','agent_memory_challenge_derived')
		AND ` + codeMessage + ` ORDER BY source_message_id,
		COALESCE((metadata_json::jsonb->>'atom_index')::int,0),id ASC LIMIT 512`
	rows, err := s.facts.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, userID, session)
	if err != nil {
		s.lg.Warn("eval code context expansion skipped", loggateway.StepID("memoryeval.code_context"), loggateway.Err(err))
		return anchored
	}
	defer rows.Close()
	for rows.Next() {
		raw, scanErr := scanFactRowJSON(rows)
		if scanErr != nil {
			s.lg.Warn("eval code context row skipped", loggateway.StepID("memoryeval.code_context"), loggateway.Err(scanErr))
			continue
		}
		env, ok := evalFactEnvelope(raw, 0)
		if !ok || seen[env.item.ID] {
			continue
		}
		seen[env.item.ID] = true
		// Patch atoms are immutable trajectory events. Older ingest could mark
		// a docstring as historical or supersede a prose-looking fragment;
		// those generic governance labels must not dismember a source message.
		env.invalidUntil = ""
		env.historical = false
		env.item.Score = seedScore * 0.9
		env.relevance = seedScore * 0.9
		anchored = append(anchored, env)
	}
	if err := rows.Err(); err != nil {
		s.lg.Warn("eval code context expansion incomplete", loggateway.StepID("memoryeval.code_context"), loggateway.Err(err))
	}
	return anchored
}

func evalCodeSummary(content string) bool {
	body := strings.ToLower(evalCodeBody(content))
	return strings.Contains(body, "summary of changes") || strings.Contains(body, "changes summary") ||
		strings.Contains(body, "summary of all changes") || strings.Contains(body, "implementation is complete")
}

func evalCodePatch(content, kind string) bool {
	if biz.IsEvalCaseKind(kind) {
		return true
	}
	body := strings.ToLower(evalCodePayloadText(evalCodeBody(content)))
	return strings.Contains(body, "[tool_use write]") || strings.Contains(body, "[tool_use edit]") ||
		strings.Contains(body, "[tool_use multiedit]") || strings.Contains(body, "diff --git") ||
		strings.Contains(body, `"file_path"`) && strings.Contains(body, `"content"`) ||
		strings.Contains(body, `"file_path"`) && strings.Contains(body, `"new_string"`)
}

func evalCodeTestFile(content string) bool {
	body := strings.ToLower(evalCodePayloadText(evalCodeBody(content)))
	return strings.Contains(body, "/tests/") || strings.Contains(body, "/test_") ||
		strings.Contains(body, "_test.go") || strings.Contains(body, "_test.py")
}

// evalCodeOutcomeSignal scores how close a unit is to the final, verified
// state of a solve trajectory. Lexical relevance alone cannot tell the
// verified end state from an identical-looking intermediate failure.
func evalCodeOutcomeSignal(content string) float64 {
	body := strings.ToLower(evalCodeBody(content))
	signal := 0.0
	pass := strings.Contains(body, "tests pass") || strings.Contains(body, "passed [") ||
		strings.Contains(body, " passed") || strings.Contains(body, " passed,")
	fail := strings.Contains(body, "failed") || strings.Contains(body, "failure")
	summary := evalCodeSummary(content)
	isTest, _ := evalCodeTestOutput(content)
	if summary || isTest {
		switch {
		case pass && (!fail || summary):
			signal += 0.45 // a clean verified run or final summary with an unrelated known failure
		case pass && fail:
			signal += 0.05 // intermediate state, the fix is not done yet
		}
	}
	if summary {
		signal += 0.35
	}
	if strings.Contains(body, "[tool_use write]") || strings.Contains(body, "[tool_use edit]") ||
		strings.Contains(body, "[tool_use multiedit]") {
		signal += 0.3
	}
	if strings.Contains(body, "diff --git") {
		signal += 0.3
	}
	return signal
}

var (
	evalToolWritePathRe   = regexp.MustCompile(`\[tool_use Write\].{0,200}?"file_path"\s*:\s*"([^"]+)"`)
	evalCodeQueryPathRe   = regexp.MustCompile(`(?i)\b[\w.-]+/[\w./-]+\.(?:py|go|js|jsx|ts|tsx|java|rs|cpp|cc|c|h|hpp|rb|php|cs)\b`)
	evalCodeQueryClassRe  = regexp.MustCompile(`\b[A-Z][a-z0-9]+(?:[A-Z][A-Za-z0-9]*)+\b`)
	evalCodeQuerySymbolRe = regexp.MustCompile(`\b[_a-z][a-z0-9]*(?:_[a-z0-9]+)+\b`)
)

// evalCodeSpecificity makes an exact source path or symbol outrank an
// unrelated patch that merely shares generic words such as "fix" or
// "required". Error-driven follow-ups often name the precise function they
// are editing; that source is more useful than another task's final diff.
func evalCodeQuerySymbols(query string) []string {
	out := evalCodeQuerySymbolRe.FindAllString(strings.ToLower(query), -1)
	seen := map[string]bool{}
	for _, symbol := range out {
		seen[symbol] = true
	}
	tokens := strings.FieldsFunc(strings.ToLower(query), func(r rune) bool {
		return r != '_' && (r < 'a' || r > 'z') && (r < '0' || r > '9')
	})
	for i, token := range tokens {
		if token == "" {
			continue
		}
		for j := i + 1; j < len(tokens) && j <= i+3; j++ {
			if tokens[j] == "" {
				continue
			}
			symbol := token + "_" + tokens[j]
			if len(symbol) < 7 {
				continue
			}
			for _, candidate := range []string{symbol, symbol + "s"} {
				if !seen[candidate] {
					seen[candidate] = true
					out = append(out, candidate)
				}
			}
		}
	}
	return out
}

func evalCodeSpecificity(content, query string) float64 {
	body := strings.ToLower(evalCodePayloadText(content))
	score := 0.0
	for _, path := range evalCodeQueryPathRe.FindAllString(query, -1) {
		if strings.Contains(body, strings.ToLower(path)) {
			score += 8
		}
	}
	for _, class := range evalCodeQueryClassRe.FindAllString(query, -1) {
		if strings.Contains(body, strings.ToLower(class)) {
			score += 1.5
		}
	}
	for _, symbol := range evalCodeQuerySymbols(query) {
		if strings.Contains(body, symbol) {
			score += 4
		}
	}
	return score
}

// evalCodeExactEntityHit is deliberately narrower than specificity: a class
// name such as TimeSeries is enough to rank a source reading, but not enough
// to trust another task's patch. Error-driven transfer requires an exact path
// or function symbol before reusing a patch.
func evalCodeExactEntityHit(content, query string) bool {
	body := strings.ToLower(evalCodePayloadText(content))
	for _, path := range evalCodeQueryPathRe.FindAllString(query, -1) {
		if strings.Contains(body, strings.ToLower(path)) {
			return true
		}
	}
	for _, symbol := range evalCodeQuerySymbolRe.FindAllString(query, -1) {
		if strings.Contains(body, symbol) {
			return true
		}
	}
	return false
}

func evalCodeFullWrite(content string) bool {
	return evalToolWritePathRe.FindStringSubmatch(evalCodePayloadText(content)) != nil
}

// evalCodeErrorCapItems assembles transfer evidence for a new failure. Unlike
// a solve replay, there may be no verified fix for this issue in memory. The
// best evidence is therefore the precise function/API source, a small amount
// of directly matching patch history and clean verification style—not the
// complete outcome pack of a merely similar task.
func evalCodeErrorCapItems(items []evalCollectedItem, query string, topK int32) []evalCollectedItem {
	lim := int(topK)
	if lim <= 0 || lim > 12 {
		lim = 12
	}
	const maxRunes = 16000
	type scoredItem struct {
		env   evalCollectedItem
		score float64
	}
	rows := make([]scoredItem, 0, len(items))
	for _, env := range items {
		if evalCodeNoise(env) || env.messageIndex == 0 {
			continue
		}
		specificity := evalCodeSpecificity(env.item.Content, query)
		if specificity == 0 {
			continue
		}
		isPatch := evalCodePatch(env.item.Content, env.kind)
		if isPatch {
			if !evalCodeExactEntityHit(env.item.Content, query) {
				continue
			}
			// Full-file Writes from another task hide the relevant function in
			// thousands of tokens. Numbered source reads and focused Edits are
			// safer transfer evidence.
			if evalCodeFullWrite(env.item.Content) && len([]rune(env.item.Content)) > 6000 {
				continue
			}
		}
		isTest, clean := evalCodeTestOutput(env.item.Content)
		if isTest && (!clean || !strings.Contains(strings.ToLower(query), "test")) {
			// Failed runs are not transferable fixes, and unrequested test output
			// only repeats environment noise around the precise source evidence.
			continue
		}
		if !isPatch && !isTest && !evalIsCodeUnit(env.item.Content, env.kind) {
			// Error-driven retrieval is an evidence API, not a transcript replay:
			// generic reasoning and old task summaries do not belong in this pack.
			continue
		}
		score := env.item.Score + specificity
		if evalIsCodeUnit(env.item.Content, env.kind) && !isPatch {
			score += 2.5 // exact API/source is the primary transfer evidence
		}
		if isPatch {
			score += 0.75
		}
		if isTest && strings.Contains(strings.ToLower(query), "test") {
			score += 0.5
		}
		rows = append(rows, scoredItem{env: env, score: score})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].score != rows[j].score {
			return rows[i].score > rows[j].score
		}
		return rows[i].env.item.ID < rows[j].env.item.ID
	})
	out := make([]evalCollectedItem, 0, lim)
	usedRunes := 0
	sessionCount := map[string]int{}
	seenContent := map[string]struct{}{}
	patchCount := 0
	testCount := 0
	for _, row := range rows {
		if sessionCount[row.env.sess] >= 6 {
			continue
		}
		contentKey := strings.Join(strings.Fields(evalStripEvidencePrefix(row.env.item.Content)), " ")
		if _, exists := seenContent[contentKey]; exists {
			continue
		}
		isPatch := evalCodePatch(row.env.item.Content, row.env.kind)
		isTest, _ := evalCodeTestOutput(row.env.item.Content)
		switch {
		case isPatch && patchCount >= 4:
			continue
		case isTest && testCount >= 2:
			continue
		}
		size := len([]rune(row.env.item.Content))
		if usedRunes+size > maxRunes {
			continue
		}
		usedRunes += size
		sessionCount[row.env.sess]++
		seenContent[contentKey] = struct{}{}
		if isPatch {
			patchCount++
		}
		if isTest {
			testCount++
		}
		row.env.item.Score = row.score
		out = append(out, row.env)
		if len(out) >= lim {
			break
		}
	}
	return out
}

// evalCodeSignature identifies repeated runs of the same command or test.
// Only the latest identical run describes the verified end state; earlier
// copies are budget noise. Timings and counts vary between runs, so digits
// do not participate. A file Write is keyed by its path: a later full
// rewrite supersedes the earlier content, while follow-up Edits stay
// distinct incremental patches.
func evalCodeSignature(content string) string {
	body := strings.ToLower(evalStripEvidencePrefix(content))
	if m := evalToolWritePathRe.FindStringSubmatch(evalCodePayloadText(content)); m != nil {
		return "write|" + m[1]
	}
	body = strings.Join(strings.Fields(body), " ")
	body = strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return -1
		}
		return r
	}, body)
	runes := []rune(body)
	if len(runes) > 200 {
		runes = append(runes[:140], runes[len(runes)-60:]...)
	}
	return string(runes)
}

func evalCodeTestOutput(content string) (isTest, cleanPass bool) {
	// Source patches often quote failing/passing tests in comments or string
	// literals. Their structural type wins; never discard a Write/Edit as if
	// it were an intermediate pytest run.
	if evalCodeSummary(content) || evalCodePatch(content, "") {
		return false, false
	}
	body := strings.ToLower(evalCodeBody(content))
	isTest = strings.Contains(body, " passed in ") || strings.Contains(body, "tests pass") ||
		strings.Contains(body, " failed,") || strings.Contains(body, "collected ") ||
		strings.Contains(body, "passed test ") || strings.Contains(body, "passed [")
	cleanPass = isTest && !strings.Contains(body, "failed") && !strings.Contains(body, "failure") &&
		(strings.Contains(body, "passed") || strings.Contains(body, "tests pass"))
	return isTest, cleanPass
}

// evalCodeNoise removes fragments that cannot help reconstruct a patch:
// role-only atoms, planning/tool bookkeeping and private exploration notes.
// A passing verdict inside a thinking message is redundant with the actual
// test output and final summary, so it remains noise as well.
func evalCodeNoise(env evalCollectedItem) bool {
	body := strings.TrimSpace(evalCodeBody(env.item.Content))
	switch strings.ToLower(body) {
	case "", "user:", "assistant:", "system:", "developer:", "tool:":
		return true
	}
	if strings.HasPrefix(body, "[thinking]") {
		return true
	}
	for _, tool := range []string{"[tool_use TaskCreate]", "[tool_use TaskUpdate]", "[tool_use TaskList]", "[tool_use TaskGet]"} {
		if strings.Contains(body, tool) {
			return true
		}
	}
	if strings.HasPrefix(body, "[tool_result]") && !evalIsCodeUnit(env.item.Content, env.kind) {
		isTest, _ := evalCodeTestOutput(body)
		return !isTest
	}
	return false
}

// evalCodeCapItems replaces MMR for code queries. Diversity is the wrong
// objective for a solve trajectory: the final patch, its edits and the
// verified test run all live in the same session and look alike. Order by
// outcome value, collapse repeated runs, keep few complete units.
func evalCodeCapItems(items []evalCollectedItem, query string, topK int32) []evalCollectedItem {
	return evalCodeCapItemsMode(items, query, topK, true)
}

func evalCodeCapItemsMode(items []evalCollectedItem, query string, topK int32, solve bool) []evalCollectedItem {
	lim := int(topK)
	if lim <= 0 || lim > evalCodePackMaxItems {
		lim = evalCodePackMaxItems
	}
	maxMessage := map[string]int{}
	for _, env := range items {
		if env.messageIndex > maxMessage[env.sess] {
			maxMessage[env.sess] = env.messageIndex
		}
	}
	type scoredItem struct {
		env   evalCollectedItem
		score float64
	}
	hasCleanPass := false
	for _, env := range items {
		if _, clean := evalCodeTestOutput(env.item.Content); clean {
			hasCleanPass = true
			break
		}
	}
	hasPatch := false
	for _, env := range items {
		if evalCodePatch(env.item.Content, env.kind) {
			hasPatch = true
			break
		}
	}
	latestWrite := map[string]int{}
	for _, env := range items {
		signature := env.sess + "|" + evalCodeSignature(env.item.Content)
		if strings.Contains(signature, "|write|") && env.messageIndex > latestWrite[signature] {
			latestWrite[signature] = env.messageIndex
		}
	}
	rows := make([]scoredItem, 0, len(items))
	for _, env := range items {
		if evalCodeNoise(env) {
			continue
		}
		isTest, clean := evalCodeTestOutput(env.item.Content)
		if !solve && evalCodePatch(env.item.Content, env.kind) && evalCodeTestFile(env.item.Content) &&
			!strings.Contains(strings.ToLower(query), "test") {
			// A large test-file Write from another task is not useful for an
			// implementation/error query that never asked for test evidence.
			continue
		}
		if isTest && hasCleanPass && !clean {
			continue
		}
		if solve && hasPatch && env.messageIndex != 0 && !isTest && !evalCodePatch(env.item.Content, env.kind) && evalIsCodeUnit(env.item.Content, env.kind) {
			// A single-trajectory solve pack already contains the reusable patch,
			// so duplicate source dumps only consume its budget. Generic error
			// queries are different: their exact source reading may be the only
			// relevant evidence, while the patch belongs to another task.
			continue
		}
		if solve && hasPatch && env.messageIndex != 0 && !isTest && !evalCodeSummary(env.item.Content) &&
			!evalCodePatch(env.item.Content, env.kind) && len([]rune(env.item.Content)) > 2000 {
			continue
		}
		signature := env.sess + "|" + evalCodeSignature(env.item.Content)
		if strings.Contains(signature, "|write|") && env.messageIndex < latestWrite[signature] {
			continue
		}
		score := env.item.Score + evalCodeSpecificity(env.item.Content, query)
		if evalIsCodeUnit(env.item.Content, env.kind) {
			// Real code outranks prose that only shares query words.
			score += 0.25
		}
		score += evalCodeOutcomeSignal(env.item.Content)
		if evalCodePatch(env.item.Content, env.kind) {
			if evalCodeTestFile(env.item.Content) {
				score -= 0.35 // test source is secondary to the implementation it verifies
			} else {
				score += 0.20
			}
		}
		if max := maxMessage[env.sess]; max > 0 {
			// The verified end state sits at the trajectory tail.
			score += 0.15 * float64(env.messageIndex) / float64(max)
		}
		rows = append(rows, scoredItem{env: env, score: score})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].score != rows[j].score {
			return rows[i].score > rows[j].score
		}
		return rows[i].env.item.ID < rows[j].env.item.ID
	})
	seen := map[string]bool{}
	out := make([]evalCollectedItem, 0, lim)
	proseUnits := 0
	usedRunes := 0
	for _, row := range rows {
		signature := row.env.sess + "|" + evalCodeSignature(row.env.item.Content)
		if seen[signature] {
			continue
		}
		isTest, _ := evalCodeTestOutput(row.env.item.Content)
		if !evalIsCodeUnit(row.env.item.Content, row.env.kind) && !isTest &&
			evalCodeOutcomeSignal(row.env.item.Content) == 0 && row.env.messageIndex != 0 {
			// A few explanatory facts can clarify intent; unlimited assistant
			// narration cannot. Patch, task and verdict units are exempt.
			if proseUnits >= 4 {
				continue
			}
			proseUnits++
		}
		size := len([]rune(row.env.item.Content))
		if usedRunes+size > evalCodePackMaxRunes {
			continue
		}
		seen[signature] = true
		usedRunes += size
		row.env.item.Score = row.score
		out = append(out, row.env)
		if len(out) >= lim {
			break
		}
	}
	return out
}
