package data

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func TestEvalStore_CodeContextSurvivesLargeTopK(t *testing.T) {
	s := setupEvalStore(t)
	var code strings.Builder
	code.WriteString("1\tclass ColumnStore:\n2\t    def check_schema(self, required):\n3\t        seen = set(self.names)\n")
	for line := 4; line < 34; line++ {
		if line == 17 {
			code.WriteString("17\t        if self.strict is False:\n18\t            return True\n")
			line++
			continue
		}
		fmt.Fprintf(&code, "%d\t        # Keep the surrounding validation context for required fields.\n", line)
	}
	code.WriteString("34\t        if required.difference(seen):\n35\t            raise ValueError(\"missing required fields\")\n36\t        return required.issubset(seen)\n")
	messages := []biz.EvalMessage{{Role: "tool", MessageID: "source", Content: code.String()}}
	for i := 0; i < 100; i++ {
		messages = append(messages, biz.EvalMessage{Role: "assistant", MessageID: fmt.Sprint(i), Content: fmt.Sprintf("ColumnStore required fields validation exception debugging note %d: inspect the schema and surrounding context.", i)})
	}
	ctx := context.Background()
	if _, err := s.AddMessages(ctx, "code-context", "source-session", messages); err != nil {
		t.Fatal(err)
	}
	items, err := s.SearchMemories(ctx, "code-context", "How to fix ColumnStore check_schema() missing required fields exception?", 100)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range items {
		if strings.Contains(item.Content, "def check_schema") {
			found = true
			for _, required := range []string{"class ColumnStore:", "if self.strict is False:", "if required.difference(seen):", "raise ValueError(\"missing required fields\")", "return required.issubset(seen)"} {
				if !strings.Contains(item.Content, required) {
					t.Fatalf("code context lost %q among %d results: %s", required, len(items), item.Content)
				}
			}
			if strings.Contains(item.Content, "[…]") {
				t.Fatalf("small code unit was split into partial windows: %s", item.Content)
			}
		}
	}
	if !found {
		t.Fatalf("source definition absent among %d results", len(items))
	}
}

func TestEvalStore_CodeWriteStaysImmutableEvent(t *testing.T) {
	repo := setupL3FTSTestRepo(t, nil, 0)
	store := NewEvalMemoryStore(repo.data, nil, loggateway.NewNoop())
	ctx := context.Background()
	_, err := store.AddMessages(ctx, "code-immutable", "s-write", []biz.EvalMessage{{
		Role: "assistant", MessageID: "m-write",
		Content: `[tool_use Write] {\"file_path\": \"/testbed/core.py\", \"content\": \"# I prefer this required implementation\\nclass TimeSeries: pass\"}`,
	}})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := repo.ListFactRowsForUserAll(ctx, "user", "code-immutable", "code-immutable", "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	var sourceFacts []evalFactRow
	for _, raw := range rows {
		fact := decodeEvalFactRow(raw)
		if fact.sourceSessionID == "s-write" {
			sourceFacts = append(sourceFacts, fact)
		}
	}
	if len(sourceFacts) != 1 {
		t.Fatalf("source rows = %d, want one whole tool payload", len(sourceFacts))
	}
	fact := sourceFacts[0]
	if fact.kind != "event" || fact.validUntil != "" {
		t.Fatalf("tool payload became governable: kind=%s valid_until=%s", fact.kind, fact.validUntil)
	}
}

func TestEvalStore_CodeQueryReassemblesOldFragmentsAndAnchorsSession(t *testing.T) {
	repo := setupL3FTSTestRepo(t, nil, 0)
	store := NewEvalMemoryStore(repo.data, nil, loggateway.NewNoop())
	ctx := context.Background()
	add := func(session string, message, atom int, statement string) {
		t.Helper()
		_, err := repo.UpsertFactRow(ctx, biz.FactUpsert{
			ScopeType: "user", ScopeID: "code-old", UserID: "code-old", AgentID: "code-old",
			Statement: statement, Fingerprint: fmt.Sprintf("%s-%d-%d", session, message, atom),
			FactKind: "event", Confidence: 1, Importance: 0.5, Status: "active", Version: 1,
			SourceKind: "agent_memory_challenge", SourceSessionID: session,
			SourceMessageID: fmt.Sprintf("%s:%d", session, message),
			MetadataJSON:    evalSourceMetadata(session, message, atom, nil, nil, nil, "", "", statement),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	target := "s-target"
	add(target, 0, 0, "user: This PR introduces a new astropy.timeseries sub-package based on QTable.")
	add(target, 0, 1, "user: Investigate the issue above and make the necessary source changes to address it.")
	add(target, 267, 0, `assistant: [tool_use Write] {\"file_path\": \"/testbed/astropy/timeseries/core.py\", \"content\": \"class TimeSeries(QTable):`)
	add(target, 267, 1, `assistant:     pass # migrated last year\nclass BinnedTimeSeries(QTable):\n    pass\"}`)
	add(target, 442, 0, "user: collected 23 items")
	add(target, 442, 1, "user: ==== 23 passed in 2.06s ====")
	add(target, 459, 0, "assistant: Here's a summary of all changes made: TimeSeries implemented. All 23 tests pass.")
	noise := "s-noise"
	add(noise, 0, 0, "user: Converting a binned TimeSeries to a pandas DataFrame failed because indexes were not handled.")
	add(noise, 40, 0, `assistant: [tool_use Write] {\"file_path\": \"/testbed/astropy/table/table.py\", \"content\": \"WRONG_NOISE_IMPLEMENTATION\"}`)

	queries := []string{
		"This PR introduces a new astropy.timeseries sub-package based on QTable. Investigate the issue above and make the necessary source changes to address it.",
		"How should TimeSeries and BinnedTimeSeries be implemented using QTable?",
		"astropy timeseries tests and expected behavior",
		"astropy/timeseries/core.py implementation details",
	}
	for _, query := range queries {
		items, err := store.SearchMemories(ctx, "code-old", query, 100)
		if err != nil {
			t.Fatal(err)
		}
		joined := evalContents(items)
		for _, want := range []string{"[tool_use Write]", "class TimeSeries(QTable):", "class BinnedTimeSeries(QTable):", "23 passed in 2.06s", "summary of all changes"} {
			if !strings.Contains(joined, want) {
				t.Fatalf("query %q lost %q: %s", query, want, joined)
			}
		}
		if strings.Contains(joined, "WRONG_NOISE_IMPLEMENTATION") {
			t.Fatalf("query %q leaked noise trajectory: %s", query, joined)
		}
		if strings.Contains(joined, "[…]") {
			t.Fatalf("query %q partially truncated code evidence: %s", query, joined)
		}
	}
}

func TestEvalStore_CodeContextKeepsWholeHistoricalWrite(t *testing.T) {
	repo := setupL3FTSTestRepo(t, nil, 0)
	store := NewEvalMemoryStore(repo.data, nil, loggateway.NewNoop())
	ctx := context.Background()
	const user = "code-historical-write"
	add := func(message int, statement, validUntil string) {
		t.Helper()
		_, err := repo.UpsertFactRow(ctx, biz.FactUpsert{
			ScopeType: "user", ScopeID: user, UserID: user, AgentID: user,
			Statement: statement, Fingerprint: fmt.Sprintf("historical-%d", message),
			FactKind: "event", Confidence: 1, Importance: 0.5, Status: "active", Version: 1,
			SourceKind: "agent_memory_challenge", SourceSessionID: "s-target",
			SourceMessageID: fmt.Sprintf("s-target:%d", message), ValidUntil: validUntil,
			MetadataJSON: evalSourceMetadata("s-target", message, 0, nil, nil, nil, "", "", statement),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	add(0, "user: This PR introduces a new astropy.timeseries sub-package based on QTable. Investigate the issue above and make the necessary source changes to address it.", "")
	add(267, `assistant: [tool_use Write] {\"file_path\": \"/testbed/astropy/timeseries/core.py\", \"content\": \"# migrated last year\nclass TimeSeries(QTable):\n    pass\nclass BinnedTimeSeries(QTable):\n    pass\"}`, "2020-01-01T00:00:00Z")

	items, err := store.SearchMemories(ctx, user, "This PR introduces a new astropy.timeseries sub-package based on QTable. Investigate the issue above and make the necessary source changes to address it.", 100)
	if err != nil {
		t.Fatal(err)
	}
	joined := evalContents(items)
	for _, want := range []string{"[tool_use Write]", "class TimeSeries(QTable):", "class BinnedTimeSeries(QTable):"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("whole historical Write lost %q: %s", want, joined)
		}
	}
	if strings.Contains(joined, "[superseded;") {
		t.Fatalf("immutable patch was exposed as superseded evidence: %s", joined)
	}
}

func TestEvalIsCodeUnit_StoredSourceFormats(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		want          bool
	}{
		{"fenced", "assistant: ```python\ndef validate(value):\n    return value is not None\n```", true},
		{"numbered", "tool: 8\tclass ColumnStore:\n9\t    def validate(self):\n10\t        return True", true},
		{"prose", "assistant: I will inspect validation later.", false},
		{"numbered prose", "user: 1\tRemember the appointment.\n2\tWe discussed the date.\n3\tTry later.", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := evalIsCodeUnit(tc.content, "event"); got != tc.want {
				t.Fatalf("evalIsCodeUnit = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEvalStore_CodeContextSkipsUnitThatCannotFit(t *testing.T) {
	s := setupEvalStore(t)
	var messages []biz.EvalMessage
	for i := 0; i < 8; i++ {
		body := fmt.Sprintf("diff --git a/validation/field_%d.py b/validation/field_%d.py\n@@ -1,3 +1,4 @@\n+def check_schema(required, seen):\n", i, i)
		body += strings.Repeat("+    # Keep field validation context with the complete guard.\n", 35)
		body += "+    if strict_mode is False:\n+        return True\n"
		body += strings.Repeat("+    # Keep field validation context with the complete guard.\n", 35)
		body += "+    return required.issubset(seen)\n"
		if len([]rune(body)) >= 6000 {
			t.Fatal("fixture must fit one code unit's maximum budget")
		}
		messages = append(messages, biz.EvalMessage{Role: "tool", MessageID: fmt.Sprint(i), Content: body})
	}
	ctx := context.Background()
	if _, err := s.AddMessages(ctx, "code-budget", "source-session", messages); err != nil {
		t.Fatal(err)
	}
	items, err := s.SearchMemories(ctx, "code-budget", "validation field check_schema() exception fix", 8)
	if err != nil || len(items) == 0 {
		t.Fatalf("SearchMemories = %d, %v", len(items), err)
	}
	completeSources := 0
	for _, item := range items {
		if strings.HasPrefix(item.ID, "l2:") {
			continue // A whole session exceeds the per-unit cap and may be excerpted.
		}
		completeSources++
		if strings.Contains(item.Content, "[…]") || !strings.Contains(item.Content, "if strict_mode is False:") || !strings.Contains(item.Content, "return required.issubset(seen)") {
			t.Fatalf("remaining budget split a code unit: %s", item.Content)
		}
	}
	if completeSources == 0 || completeSources >= len(messages) {
		t.Fatalf("complete source count = %d, want a nonempty subset within the total budget", completeSources)
	}
}
