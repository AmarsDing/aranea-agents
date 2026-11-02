package data

import (
	"fmt"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

func codeTestItem(id, sess, content string, score float64, messageIndex int) evalCollectedItem {
	return evalCollectedItem{
		item:         biz.EvalMemoryItem{ID: id, Content: content, Score: score},
		sess:         sess,
		messageIndex: messageIndex,
	}
}

// The coding trajectory replay must surface the verified end state
// (complete write + final passing run + change summary), not dozens of
// lexically similar exploration fragments.
func TestEvalCodeCapItems_PrefersVerifiedEndState(t *testing.T) {
	sess := "s-code"
	writePayload := `assistant: [tool_use Write] {"file_path": "/testbed/astropy/timeseries/core.py", "content": "class TimeSeries(QTable):\n    pass\nclass BinnedTimeSeries(QTable):\n    pass"}`
	var items []evalCollectedItem
	items = append(items,
		codeTestItem("task", sess, "user: This PR introduces a new astropy.timeseries sub-package. Investigate the issue above and make the necessary source changes.", 0.94, 1),
		codeTestItem("failed-run", sess, "user: Platform: Linux\ncollected 23 items\nastropy/timeseries/tests/test_timeseries.py::test_fold FAILED [ 30% ]\n==== 1 failed, 22 passed ====", 0.80, 100),
		codeTestItem("write", sess, writePayload, 0.79, 267),
	)
	// The same passing test output replayed three times: only the latest
	// run may survive.
	for i, mi := range []int{335, 395, 442} {
		items = append(items, codeTestItem(fmt.Sprintf("pass-run-%d", i), sess,
			"user: Platform: Linux\ncollected 23 items\nastropy/timeseries/tests/test_timeseries.py::test_fold PASSED [ 30% ]\n==== 23 passed in 2.0s ====", 0.78, mi))
	}
	items = append(items,
		codeTestItem("summary", sess, "assistant: Here's a summary of all changes made: TimeSeries and BinnedTimeSeries implemented. All 23 tests pass.", 0.70, 459),
		codeTestItem("complete", sess, "assistant: The implementation is complete with all tests passing.", 0.68, 455),
	)
	// Mid-trajectory exploration prose shares the query words but carries no
	// patch and no verified state.
	for i := 0; i < 40; i++ {
		items = append(items, codeTestItem(fmt.Sprintf("noise-%d", i), sess,
			fmt.Sprintf("assistant: Let me inspect the astropy timeseries TimeSeries QTable source again, iteration %d.", i), 0.72, 20+i))
	}
	out := evalCodeCapItems(items, "This PR introduces a new astropy.timeseries sub-package based on QTable. Investigate the issue above and make the necessary source changes.", 100)
	var ids []string
	for _, env := range out {
		ids = append(ids, env.item.ID)
	}
	joined := strings.Join(ids, ",")
	for _, want := range []string{"task", "write", "pass-run-2", "summary", "complete"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("code pack lost %s: %v", want, ids)
		}
	}
	if strings.Contains(joined, "pass-run-0") || strings.Contains(joined, "pass-run-1") {
		t.Fatalf("repeated identical test runs were not collapsed: %v", ids)
	}
	if len(out) > evalCodePackMaxItems {
		t.Fatalf("code pack exceeds %d items: %d", evalCodePackMaxItems, len(out))
	}
	// Once a clean final run exists, an intermediate failed run is noise.
	if strings.Contains(joined, "failed-run") {
		t.Fatalf("intermediate failure survived beside the clean final run: %v", ids)
	}
}

// A full rewrite of the same file supersedes the earlier write; follow-up
// edits to that file are distinct patches and all survive.
func TestEvalCodeCapItems_LatestWriteWinsEditsSurvive(t *testing.T) {
	sess := "s-rewrite"
	write := func(mi int, body string) evalCollectedItem {
		return codeTestItem(fmt.Sprintf("write-%d", mi), sess,
			fmt.Sprintf(`assistant: [tool_use Write] {"file_path": "/testbed/core.py", "content": "%s"}`, body), 0.7, mi)
	}
	edit := func(mi int, old string) evalCollectedItem {
		return codeTestItem(fmt.Sprintf("edit-%d", mi), sess,
			fmt.Sprintf(`assistant: [tool_use Edit] {"replace_all": false, "file_path": "/testbed/core.py", "old_string": "%s", "new_string": "fixed"}`, old), 0.7, mi)
	}
	items := []evalCollectedItem{
		write(83, "initial draft"),
		edit(278, "def add_column"),
		write(267, "rewritten with fixes"),
		edit(294, "def __getitem__"),
	}
	// Recency, not a higher lexical score, decides which full rewrite wins.
	items[0].item.Score = 0.99
	out := evalCodeCapItems(items, "fix(core): patch core.py regression", 100)
	var ids []string
	for _, env := range out {
		ids = append(ids, env.item.ID)
	}
	joined := strings.Join(ids, ",")
	if strings.Contains(joined, "write-83") {
		t.Fatalf("superseded initial write survived: %v", ids)
	}
	for _, want := range []string{"write-267", "edit-278", "edit-294"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("code pack lost %s: %v", want, ids)
		}
	}
}

func TestEvalCodeTestOutput_PatchIsNotPytestRun(t *testing.T) {
	patch := `assistant: [tool_use Write] {\"file_path\": \"/testbed/core.py\", \"content\": \"# collected 23 items; 1 failed, 22 passed\"}`
	if isTest, _ := evalCodeTestOutput(patch); isTest {
		t.Fatal("a source Write quoting pytest output was misclassified as a test run")
	}
}

func TestEvalCodePatch_CaseCommit(t *testing.T) {
	for _, kind := range []string{biz.EvalFactKindCaseFix, biz.EvalFactKindCaseFeature} {
		if !evalCodePatch("assistant: feat(api/v2): add token endpoint; breaking change", kind) {
			t.Fatalf("case kind %q was not preserved as patch evidence", kind)
		}
	}
}

func TestEvalCodeOutcomeSignal(t *testing.T) {
	clean := evalCodeOutcomeSignal("user: collected 23 items\n==== 23 passed in 2.06s ====")
	dirty := evalCodeOutcomeSignal("user: collected 23 items\n==== 1 failed, 22 passed ====")
	if clean <= dirty {
		t.Fatalf("clean run signal %v must beat intermediate failure %v", clean, dirty)
	}
	if evalCodeOutcomeSignal(`assistant: [tool_use Write] {"file_path": "/x.py"}`) <= 0 {
		t.Fatal("a write payload must carry an outcome signal")
	}
	if evalCodeOutcomeSignal("assistant: Here's a summary of all changes made: ...") <= 0 {
		t.Fatal("a change summary must carry an outcome signal")
	}
	if got := evalCodeOutcomeSignal("assistant: Summary of all changes: all 23 tests pass; one pre-existing unrelated failure remains."); got < 0.8 {
		t.Fatalf("final summary was downgraded by an unrelated failure: %v", got)
	}
	if evalCodeOutcomeSignal("assistant: Let me look at the code.") != 0 {
		t.Fatal("exploration prose must carry no outcome signal")
	}
}

func TestEvalCodeErrorCapItems_KeepsSourceAndDropsUnrelatedPatches(t *testing.T) {
	source := "user: [tool_result] 19\tdef autocheck_required_columns(cls):\n" +
		"20\t    pass\n57\t    def _check_required_columns(self):\n" +
		"58\t        if self.colnames[:len(required_columns)] != required_columns:\n" +
		"59\t            raise ValueError('missing required columns')\n" +
		"60\t    def remove_column(self, name):\n61\t        pass"
	items := []evalCollectedItem{
		codeTestItem("source", "s-required-columns", source, 0.82, 75),
		codeTestItem("unrelated-patch", "s-other-task", `assistant: [tool_use Edit] {"file_path": "/testbed/astropy/table/table.py", "old_string": "old", "new_string": "new"}`, 0.90, 100),
		codeTestItem("unrelated-write", "s-new-package", `assistant: [tool_use Write] {"file_path": "/testbed/astropy/timeseries/core.py", "content": "class TimeSeries: pass"}`, 0.88, 267),
		codeTestItem("unrelated-test-write", "s-new-package", `assistant: [tool_use Write] {"file_path": "/testbed/astropy/timeseries/tests/test_timeseries.py", "content": "def test_required_columns(): pass"}`, 0.88, 97),
		codeTestItem("matching-edit", "s-required-columns", `assistant: [tool_use Edit] {"file_path": "/testbed/astropy/timeseries/core.py", "old_string": "def _check_required_columns", "new_string": "def _check_required_columns"}`, 0.70, 101),
		codeTestItem("unrelated-summary", "s-other-task", "assistant: Here's a summary of the fix: pandas index handling. All tests pass.", 0.80, 156),
	}
	for _, query := range []string{
		"_check_required_columns missing required columns message fix expected as the first columns but found",
		"remove required column flux missing required columns error message test fix",
	} {
		out := evalCodeErrorCapItems(items, query, 30)
		ids := map[string]bool{}
		found := false
		for _, env := range out {
			ids[env.item.ID] = true
			if strings.Contains(env.item.Content, "def _check_required_columns") {
				found = true
			}
		}
		if !found {
			t.Fatalf("error query lost relevant source for %q", query)
		}
		for _, unwanted := range []string{"unrelated-patch", "unrelated-write", "unrelated-test-write", "unrelated-summary"} {
			if ids[unwanted] {
				t.Fatalf("unrelated %s survived error-driven packing for %q", unwanted, query)
			}
		}
		if strings.Contains(query, "_check_required_columns") != ids["matching-edit"] {
			t.Fatalf("exact-symbol patch retention mismatch for %q: %v", query, ids)
		}
	}
}

func TestEvalCodeErrorCapItems_DedupsAndSkipsUnrequestedTests(t *testing.T) {
	source := "user: [tool_result] 57\tdef _check_required_columns(self):\n" +
		"58\t    if self.colnames[:1] != ['time']:\n" +
		"59\t        raise ValueError('missing required columns')\n" + strings.Repeat("60\t    return None\n", 400)
	items := []evalCollectedItem{
		codeTestItem("source", "s-one", source, 0.9, 10),
		codeTestItem("source-copy", "s-two", source, 0.8, 10),
		codeTestItem("reasoning", "s-one", "assistant: TimeSeries to_pandas has an unrelated index problem.", 0.99, 11),
		codeTestItem("old-test", "s-one", "user: collected 23 items\n==== 23 passed in 2.06s ====", 0.99, 12),
	}
	out := evalCodeErrorCapItems(items, "TimeSeries invalid missing required columns", 100)
	if len(out) != 1 || out[0].item.ID != "source" {
		t.Fatalf("error pack must contain one precise source, got %+v", out)
	}
	if len([]rune(out[0].item.Content)) > 16000 {
		t.Fatalf("error pack exceeded compact source budget: %d", len([]rune(out[0].item.Content)))
	}
}

func TestEvalCodeCapItems_RealSmokeScaleAndNoise(t *testing.T) {
	sess := "s-real-smoke"
	items := []evalCollectedItem{
		codeTestItem("task", sess, "user: This PR introduces a new astropy.timeseries sub-package based on QTable. Investigate the issue above and make the necessary source changes.", 0.94, 0),
		codeTestItem("write", sess, `assistant: [tool_use Write] {\"file_path\": \"/testbed/astropy/timeseries/core.py\", \"content\": \"`+strings.Repeat("x", 10000)+`\"}`, 0.65, 267),
		codeTestItem("edit", sess, `assistant: [tool_use Edit] {\"file_path\": \"/testbed/astropy/timeseries/core.py\", \"old_string\": \"old\", \"new_string\": \"`+strings.Repeat("y", 3300)+`\"}`, 0.65, 278),
		codeTestItem("pass", sess, "user: collected 23 items\n==== 23 passed in 2.06s ====", 0.60, 442),
		codeTestItem("summary", sess, "assistant: Here's a summary of all changes made: TimeSeries implemented. All 23 tests pass. The pre-existing unrelated failure remains.", 0.60, 459),
		codeTestItem("source-read", sess, "user: 1\tdef test_timeseries():\n2\t    return TimeSeries\n"+strings.Repeat("source", 1400), 0.95, 430),
		codeTestItem("test-write", sess, `assistant: [tool_use Write] {\"file_path\": \"/testbed/astropy/timeseries/tests/test_timeseries.py\", \"content\": \"`+strings.Repeat("z", 25000)+`\"}`, 0.95, 97),
		codeTestItem("empty", sess, "assistant: ", 0.95, 50),
		codeTestItem("thinking", sess, "assistant: [thinking] Let me inspect astropy.timeseries again.", 0.95, 51),
		codeTestItem("bookkeeping", sess, `assistant: [tool_use TaskCreate] {\"subject\": \"Implement astropy.timeseries\"}`, 0.95, 52),
	}
	out := evalCodeCapItems(items, "This PR introduces astropy.timeseries; make the necessary source changes.", 100)
	joined := ""
	totalRunes := 0
	for _, env := range out {
		joined += env.item.ID + "," + env.item.Content + "\n"
		totalRunes += len([]rune(env.item.Content))
	}
	for _, want := range []string{"task", "write", "edit", "pass", "summary"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("real-scale code pack lost %s", want)
		}
	}
	for _, unwanted := range []string{"empty", "thinking", "bookkeeping", "source-read", "test-write"} {
		if strings.Contains(joined, unwanted+",") {
			t.Fatalf("noise %s survived code packing", unwanted)
		}
	}
	if totalRunes > 36000 {
		t.Fatalf("essential real-scale evidence exceeds budget: %d", totalRunes)
	}
	if strings.Contains(joined, "write|") {
		t.Fatal("unexpected signature leaked into content")
	}
}

func TestEvalCodeSignature_EscapedToolWritePath(t *testing.T) {
	first := `assistant: [tool_use Write] {\"file_path\": \"/testbed/core.py\", \"content\": \"old\"}`
	second := `assistant: [tool_use Write] {\"file_path\": \"/testbed/core.py\", \"content\": \"new\"}`
	if evalCodeSignature(first) != evalCodeSignature(second) {
		t.Fatalf("escaped writes to one path were not deduplicated: %q != %q", evalCodeSignature(first), evalCodeSignature(second))
	}
}

func TestEvalIsCodeUnit_ToolPayloadFragments(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		want          bool
	}{
		{"write call", `assistant: [tool_use Write] {"file_path": "/testbed/core.py", "content": "x"}`, true},
		{"edit call", `assistant: [tool_use Edit] {"replace_all": false, "file_path": "/testbed/core.py", "old_string": "a", "new_string": "b"}`, true},
		{"payload fragment", `assistant: = TIME_COLUMN]\n self._init_from_list([self[cname] for cname in new_order], "file_path": "/testbed/core.py", "content": "rest"`, true},
		{"plain prose", "assistant: I will write the file later.", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := evalIsCodeUnit(tc.content, "event"); got != tc.want {
				t.Fatalf("evalIsCodeUnit = %v, want %v", got, tc.want)
			}
		})
	}
}
