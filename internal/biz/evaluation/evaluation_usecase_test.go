package evaluation

import (
	"context"
	"errors"
	"math"
	"testing"

	"aranea-agents/pkg/loggateway"
)

func approxEq(a, b float32) bool {
	return math.Abs(float64(a-b)) < 1e-6
}

type mockRepo struct {
	cases             []Case
	runs              []Run
	insertCasesErr    error
	updateCaseCountFn func(ctx context.Context, id string, delta int) error
	getRunsByIDsRuns  []Run
	getRunsByIDsErr   error
	judgeAnnotated    []JudgeAnnotatedResult
	judgeAnnotatedErr error
	preferences       []RunPreference
	gateCfg           GateConfig
}

func (m *mockRepo) CreateDataset(_ context.Context, d Dataset) (Dataset, error) {
	return d, nil
}

func (m *mockRepo) GetDataset(_ context.Context, _ string) (Dataset, error) {
	return Dataset{}, nil
}

func (m *mockRepo) ListDatasets(_ context.Context, _ string, _, _ int) ([]Dataset, int, error) {
	return nil, 0, nil
}

func (m *mockRepo) DeleteDataset(_ context.Context, _ string) error {
	return nil
}

func (m *mockRepo) UpdateDataset(_ context.Context, _, _, _ string) (Dataset, error) {
	return Dataset{}, nil
}

func (m *mockRepo) UpdateDatasetCaseCount(ctx context.Context, id string, delta int) error {
	if m.updateCaseCountFn != nil {
		return m.updateCaseCountFn(ctx, id, delta)
	}
	return nil
}

func (m *mockRepo) InsertCases(_ context.Context, cases []Case) error {
	m.cases = append(m.cases, cases...)
	return m.insertCasesErr
}

func (m *mockRepo) InsertCasesWithCountUpdate(ctx context.Context, _ string, cases []Case) error {
	if err := m.InsertCases(ctx, cases); err != nil {
		return err
	}
	return m.UpdateDatasetCaseCount(ctx, "", 0)
}

func (m *mockRepo) ListCases(_ context.Context, _ string) ([]Case, error) {
	return nil, nil
}

func (m *mockRepo) CreateRun(_ context.Context, r Run) (Run, error) {
	m.runs = append(m.runs, r)
	return r, nil
}

func (m *mockRepo) GetRun(_ context.Context, _ string) (Run, error) {
	return Run{}, nil
}

func (m *mockRepo) UpdateRun(_ context.Context, _ Run) error {
	return nil
}

func (m *mockRepo) DeleteRun(_ context.Context, _ string) error {
	return nil
}

func (m *mockRepo) ListRuns(_ context.Context, _, _ string, _, _ int) ([]Run, int, error) {
	return nil, 0, nil
}

func (m *mockRepo) InsertCaseResult(_ context.Context, r CaseResult) error {
	return nil
}

func (m *mockRepo) ListCaseResults(_ context.Context, _ string, _, _ int) ([]CaseResult, int, error) {
	return nil, 0, nil
}

func (m *mockRepo) GetCaseResult(_ context.Context, _, _ string) (CaseResult, error) {
	return CaseResult{}, nil
}

func (m *mockRepo) UpdateCaseResultAnnotation(_ context.Context, _, _ string, _ CaseResultAnnotation) (CaseResult, error) {
	return CaseResult{}, nil
}

func (m *mockRepo) ListTrendPoints(_ context.Context, _, _ string, _ int) ([]TrendPoint, error) {
	return nil, nil
}

func (m *mockRepo) GetRunsByIDs(_ context.Context, _ []string) ([]Run, error) {
	return m.getRunsByIDsRuns, m.getRunsByIDsErr
}

func (m *mockRepo) ListJudgeAnnotatedResults(_ context.Context, _, _ string) ([]JudgeAnnotatedResult, error) {
	return m.judgeAnnotated, m.judgeAnnotatedErr
}

func (m *mockRepo) ListFailureGroups(_ context.Context, _, _ string, _ int) ([]FailureGroup, int, error) {
	return nil, 0, nil
}

func (m *mockRepo) InsertRunPreference(_ context.Context, p RunPreference) error {
	m.preferences = append(m.preferences, p)
	return nil
}

func (m *mockRepo) ListRunPreferences(_ context.Context, _ string, _ int) ([]RunPreference, error) {
	return m.preferences, nil
}

func (m *mockRepo) GetGateConfig(_ context.Context) (GateConfig, error) {
	return m.gateCfg, nil
}

func (m *mockRepo) UpsertGateConfig(_ context.Context, cfg GateConfig) error {
	m.gateCfg = cfg
	return nil
}

func TestUploadCases(t *testing.T) {
	t.Run("valid JSON array with cases", func(t *testing.T) {
		repo := &mockRepo{}
		uc := NewUsecase(repo, loggateway.NewNoop())
		json := `[{"input":"hello","expected_output":"world"},{"input":"foo","expected_output":"bar"}]`
		n, err := uc.UploadCases(context.Background(), "ds-1", json)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != 2 {
			t.Fatalf("expected 2 cases, got %d", n)
		}
		if len(repo.cases) != 2 {
			t.Fatalf("expected 2 cases in repo, got %d", len(repo.cases))
		}
		if repo.cases[0].Input != "hello" {
			t.Fatalf("expected first case input 'hello', got %q", repo.cases[0].Input)
		}
		if repo.cases[0].ExpectedOutput != "world" {
			t.Fatalf("expected first case output 'world', got %q", repo.cases[0].ExpectedOutput)
		}
		if repo.cases[1].Input != "foo" {
			t.Fatalf("expected second case input 'foo', got %q", repo.cases[1].Input)
		}
	})

	t.Run("empty array", func(t *testing.T) {
		repo := &mockRepo{}
		uc := NewUsecase(repo, loggateway.NewNoop())
		_, err := uc.UploadCases(context.Background(), "ds-1", `[]`)
		if err == nil {
			t.Fatal("expected error for empty array, got nil")
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		repo := &mockRepo{}
		uc := NewUsecase(repo, loggateway.NewNoop())
		_, err := uc.UploadCases(context.Background(), "ds-1", `{not json}`)
		if err == nil {
			t.Fatal("expected error for invalid JSON, got nil")
		}
	})

	t.Run("cases with empty Input field are filtered", func(t *testing.T) {
		repo := &mockRepo{}
		uc := NewUsecase(repo, loggateway.NewNoop())
		json := `[{"input":"","expected_output":"no-input"},{"input":"   ","expected_output":"whitespace-input"},{"input":"valid","expected_output":"yes"}]`
		n, err := uc.UploadCases(context.Background(), "ds-1", json)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != 1 {
			t.Fatalf("expected 1 valid case, got %d", n)
		}
		if len(repo.cases) != 1 {
			t.Fatalf("expected 1 case in repo, got %d", len(repo.cases))
		}
		if repo.cases[0].Input != "valid" {
			t.Fatalf("expected case input 'valid', got %q", repo.cases[0].Input)
		}
	})

	t.Run("duplicate cases are inserted as-is", func(t *testing.T) {
		repo := &mockRepo{}
		uc := NewUsecase(repo, loggateway.NewNoop())
		json := `[{"input":"same","expected_output":"a"},{"input":"same","expected_output":"b"}]`
		n, err := uc.UploadCases(context.Background(), "ds-1", json)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != 2 {
			t.Fatalf("expected 2 cases (no dedup), got %d", n)
		}
		if len(repo.cases) != 2 {
			t.Fatalf("expected 2 cases in repo, got %d", len(repo.cases))
		}
	})

	t.Run("empty dataset ID", func(t *testing.T) {
		repo := &mockRepo{}
		uc := NewUsecase(repo, loggateway.NewNoop())
		_, err := uc.UploadCases(context.Background(), "", `[{"input":"x"}]`)
		if err == nil {
			t.Fatal("expected error for empty dataset ID, got nil")
		}
	})

	t.Run("whitespace dataset ID", func(t *testing.T) {
		repo := &mockRepo{}
		uc := NewUsecase(repo, loggateway.NewNoop())
		_, err := uc.UploadCases(context.Background(), "   ", `[{"input":"x"}]`)
		if err == nil {
			t.Fatal("expected error for whitespace dataset ID, got nil")
		}
	})

	t.Run("repo InsertCases error", func(t *testing.T) {
		repo := &mockRepo{insertCasesErr: errors.New("db error")}
		uc := NewUsecase(repo, loggateway.NewNoop())
		_, err := uc.UploadCases(context.Background(), "ds-1", `[{"input":"x"}]`)
		if err == nil {
			t.Fatal("expected error from repo, got nil")
		}
	})

	t.Run("all cases have empty input", func(t *testing.T) {
		repo := &mockRepo{}
		uc := NewUsecase(repo, loggateway.NewNoop())
		json := `[{"input":"","expected_output":"a"},{"input":"  ","expected_output":"b"}]`
		n, err := uc.UploadCases(context.Background(), "ds-1", json)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != 0 {
			t.Fatalf("expected 0 cases after filtering, got %d", n)
		}
		if len(repo.cases) != 0 {
			t.Fatalf("expected 0 cases in repo, got %d", len(repo.cases))
		}
	})

	t.Run("metadata_json is preserved", func(t *testing.T) {
		repo := &mockRepo{}
		uc := NewUsecase(repo, loggateway.NewNoop())
		json := `[{"input":"q","expected_output":"a","metadata_json":"{\"tag\":\"unit\"}"}]`
		n, err := uc.UploadCases(context.Background(), "ds-1", json)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != 1 {
			t.Fatalf("expected 1 case, got %d", n)
		}
		if repo.cases[0].MetadataJSON != `{"tag":"unit"}` {
			t.Fatalf("expected metadata_json preserved, got %q", repo.cases[0].MetadataJSON)
		}
	})

	t.Run("case IDs are generated", func(t *testing.T) {
		repo := &mockRepo{}
		uc := NewUsecase(repo, loggateway.NewNoop())
		json := `[{"input":"q","expected_output":"a"}]`
		_, err := uc.UploadCases(context.Background(), "ds-1", json)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if repo.cases[0].ID == "" {
			t.Fatal("expected non-empty case ID")
		}
		if repo.cases[0].DatasetID != "ds-1" {
			t.Fatalf("expected dataset_id 'ds-1', got %q", repo.cases[0].DatasetID)
		}
	})

	t.Run("JSON object instead of array", func(t *testing.T) {
		repo := &mockRepo{}
		uc := NewUsecase(repo, loggateway.NewNoop())
		_, err := uc.UploadCases(context.Background(), "ds-1", `{"input":"x"}`)
		if err == nil {
			t.Fatal("expected error for JSON object, got nil")
		}
	})
}

func TestCompareEvalRuns(t *testing.T) {
	t.Run("delta calculation between runs", func(t *testing.T) {
		repo := &mockRepo{
			getRunsByIDsRuns: []Run{
				{ID: "r1", AgentID: "a1", DatasetID: "d1", CreatedAt: "t1",
					ExactMatchScore: 0.5, ContainsMatchScore: 0.6, LLMJudgeScore: 0.7, ToolCallAccuracy: 0.8, PassAtK: 0.9, PassHatK: 0.4},
				{ID: "r2", AgentID: "a1", DatasetID: "d1", CreatedAt: "t2",
					ExactMatchScore: 0.7, ContainsMatchScore: 0.5, LLMJudgeScore: 0.9, ToolCallAccuracy: 0.6, PassAtK: 0.8, PassHatK: 0.5},
			},
		}
		uc := NewUsecase(repo, loggateway.NewNoop())
		comps, err := uc.CompareEvalRuns(context.Background(), []string{"r1", "r2"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(comps) != 2 {
			t.Fatalf("expected 2 comparisons, got %d", len(comps))
		}

		c0 := comps[0]
		if c0.RunID != "r1" {
			t.Fatalf("expected run_id 'r1', got %q", c0.RunID)
		}
		if c0.DeltaExactMatch != 0.0 {
			t.Fatalf("baseline DeltaExactMatch should be 0, got %v", c0.DeltaExactMatch)
		}
		if c0.DeltaContainsMatch != 0.0 {
			t.Fatalf("baseline DeltaContainsMatch should be 0, got %v", c0.DeltaContainsMatch)
		}
		if c0.DeltaLLMJudge != 0.0 {
			t.Fatalf("baseline DeltaLLMJudge should be 0, got %v", c0.DeltaLLMJudge)
		}
		if c0.DeltaToolAccuracy != 0.0 {
			t.Fatalf("baseline DeltaToolAccuracy should be 0, got %v", c0.DeltaToolAccuracy)
		}

		c1 := comps[1]
		if c1.RunID != "r2" {
			t.Fatalf("expected run_id 'r2', got %q", c1.RunID)
		}
		if !approxEq(c1.ExactMatchScore, 0.7) {
			t.Fatalf("expected ExactMatchScore 0.7, got %v", c1.ExactMatchScore)
		}
		if !approxEq(c1.DeltaExactMatch, 0.2) {
			t.Fatalf("expected DeltaExactMatch 0.2, got %v", c1.DeltaExactMatch)
		}
		if !approxEq(c1.DeltaContainsMatch, -0.1) {
			t.Fatalf("expected DeltaContainsMatch -0.1, got %v", c1.DeltaContainsMatch)
		}
		if !approxEq(c1.DeltaLLMJudge, 0.2) {
			t.Fatalf("expected DeltaLLMJudge 0.2, got %v", c1.DeltaLLMJudge)
		}
		if !approxEq(c1.DeltaToolAccuracy, -0.2) {
			t.Fatalf("expected DeltaToolAccuracy -0.2, got %v", c1.DeltaToolAccuracy)
		}
	})

	t.Run("single run ID returns error", func(t *testing.T) {
		repo := &mockRepo{}
		uc := NewUsecase(repo, loggateway.NewNoop())
		_, err := uc.CompareEvalRuns(context.Background(), []string{"r1"})
		if err == nil {
			t.Fatal("expected error for single run ID, got nil")
		}
	})

	t.Run("empty run IDs returns error", func(t *testing.T) {
		repo := &mockRepo{}
		uc := NewUsecase(repo, loggateway.NewNoop())
		_, err := uc.CompareEvalRuns(context.Background(), nil)
		if err == nil {
			t.Fatal("expected error for empty run IDs, got nil")
		}
	})

	t.Run("repo returns fewer than 2 runs", func(t *testing.T) {
		repo := &mockRepo{
			getRunsByIDsRuns: []Run{
				{ID: "r1", AgentID: "a1", DatasetID: "d1"},
			},
		}
		uc := NewUsecase(repo, loggateway.NewNoop())
		_, err := uc.CompareEvalRuns(context.Background(), []string{"r1", "r-missing"})
		if err == nil {
			t.Fatal("expected error when repo returns fewer than 2 runs, got nil")
		}
	})

	t.Run("repo returns error", func(t *testing.T) {
		repo := &mockRepo{
			getRunsByIDsErr: errors.New("db down"),
		}
		uc := NewUsecase(repo, loggateway.NewNoop())
		_, err := uc.CompareEvalRuns(context.Background(), []string{"r1", "r2"})
		if err == nil {
			t.Fatal("expected error from repo, got nil")
		}
	})

	t.Run("three runs delta against baseline", func(t *testing.T) {
		repo := &mockRepo{
			getRunsByIDsRuns: []Run{
				{ID: "r1", AgentID: "a1", DatasetID: "d1", CreatedAt: "t1",
					ExactMatchScore: 0.4, ContainsMatchScore: 0.5, LLMJudgeScore: 0.6, ToolCallAccuracy: 0.7, PassAtK: 0.8, PassHatK: 0.3},
				{ID: "r2", AgentID: "a1", DatasetID: "d1", CreatedAt: "t2",
					ExactMatchScore: 0.6, ContainsMatchScore: 0.5, LLMJudgeScore: 0.8, ToolCallAccuracy: 0.7, PassAtK: 0.9, PassHatK: 0.4},
				{ID: "r3", AgentID: "a1", DatasetID: "d1", CreatedAt: "t3",
					ExactMatchScore: 0.3, ContainsMatchScore: 0.7, LLMJudgeScore: 0.5, ToolCallAccuracy: 0.9, PassAtK: 0.7, PassHatK: 0.6},
			},
		}
		uc := NewUsecase(repo, loggateway.NewNoop())
		comps, err := uc.CompareEvalRuns(context.Background(), []string{"r1", "r2", "r3"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(comps) != 3 {
			t.Fatalf("expected 3 comparisons, got %d", len(comps))
		}

		if comps[0].DeltaExactMatch != 0.0 {
			t.Fatalf("r1 is baseline, DeltaExactMatch should be 0, got %v", comps[0].DeltaExactMatch)
		}
		if !approxEq(comps[1].DeltaExactMatch, 0.2) {
			t.Fatalf("r2 DeltaExactMatch should be 0.2, got %v", comps[1].DeltaExactMatch)
		}
		if !approxEq(comps[2].DeltaExactMatch, -0.1) {
			t.Fatalf("r3 DeltaExactMatch should be -0.1, got %v", comps[2].DeltaExactMatch)
		}
		if !approxEq(comps[2].DeltaContainsMatch, 0.2) {
			t.Fatalf("r3 DeltaContainsMatch should be 0.2, got %v", comps[2].DeltaContainsMatch)
		}
		if !approxEq(comps[2].DeltaToolAccuracy, 0.2) {
			t.Fatalf("r3 DeltaToolAccuracy should be 0.2, got %v", comps[2].DeltaToolAccuracy)
		}
	})

	t.Run("comparison fields carry run data", func(t *testing.T) {
		repo := &mockRepo{
			getRunsByIDsRuns: []Run{
				{ID: "r1", AgentID: "a1", DatasetID: "d1", CreatedAt: "t1",
					ExactMatchScore: 0.5, ContainsMatchScore: 0.6, LLMJudgeScore: 0.7, ToolCallAccuracy: 0.8, PassAtK: 0.9, PassHatK: 0.4},
				{ID: "r2", AgentID: "a2", DatasetID: "d2", CreatedAt: "t2",
					ExactMatchScore: 0.5, ContainsMatchScore: 0.6, LLMJudgeScore: 0.7, ToolCallAccuracy: 0.8, PassAtK: 0.9, PassHatK: 0.4},
			},
		}
		uc := NewUsecase(repo, loggateway.NewNoop())
		comps, err := uc.CompareEvalRuns(context.Background(), []string{"r1", "r2"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		c1 := comps[1]
		if c1.AgentID != "a2" {
			t.Fatalf("expected AgentID 'a2', got %q", c1.AgentID)
		}
		if c1.DatasetID != "d2" {
			t.Fatalf("expected DatasetID 'd2', got %q", c1.DatasetID)
		}
		if c1.CreatedAt != "t2" {
			t.Fatalf("expected CreatedAt 't2', got %q", c1.CreatedAt)
		}
		if !approxEq(c1.PassAtK, 0.9) {
			t.Fatalf("expected PassAtK 0.9, got %v", c1.PassAtK)
		}
		if !approxEq(c1.PassHatK, 0.4) {
			t.Fatalf("expected PassHatK 0.4, got %v", c1.PassHatK)
		}
	})

	t.Run("zero scores delta is zero", func(t *testing.T) {
		repo := &mockRepo{
			getRunsByIDsRuns: []Run{
				{ID: "r1"}, {ID: "r2"},
			},
		}
		uc := NewUsecase(repo, loggateway.NewNoop())
		comps, err := uc.CompareEvalRuns(context.Background(), []string{"r1", "r2"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if comps[1].DeltaExactMatch != 0 {
			t.Fatalf("expected 0 delta, got %v", comps[1].DeltaExactMatch)
		}
		if comps[1].DeltaContainsMatch != 0 {
			t.Fatalf("expected 0 delta, got %v", comps[1].DeltaContainsMatch)
		}
		if comps[1].DeltaLLMJudge != 0 {
			t.Fatalf("expected 0 delta, got %v", comps[1].DeltaLLMJudge)
		}
		if comps[1].DeltaToolAccuracy != 0 {
			t.Fatalf("expected 0 delta, got %v", comps[1].DeltaToolAccuracy)
		}
	})
}
