package evaluation

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

func approxEq(a, b float32) bool {
	return math.Abs(float64(a-b)) < 1e-6
}

type mockRepo struct {
	cases             []Case
	runs              []Run
	insertCasesErr    error
	getRunsByIDsRuns  []Run
	getRunsByIDsErr   error
	judgeAnnotated    []JudgeAnnotatedResult
	judgeAnnotatedErr error
	preferences       []RunPreference
	gateCfg           GateConfig
	staleCutoff       time.Time
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

func (m *mockRepo) InsertCasesWithCountUpdate(_ context.Context, _ string, cases []Case) error {
	m.cases = append(m.cases, cases...)
	return m.insertCasesErr
}

func (m *mockRepo) ListCases(_ context.Context, _ string) ([]Case, error) {
	return m.cases, nil
}

func (m *mockRepo) UpdateCase(_ context.Context, c Case) (Case, error) {
	for i := range m.cases {
		if m.cases[i].ID == c.ID {
			m.cases[i] = c
			return c, nil
		}
	}
	return Case{}, errors.New("case not found")
}

func (m *mockRepo) DeleteCase(_ context.Context, _, caseID string) error {
	out := m.cases[:0]
	for _, c := range m.cases {
		if c.ID != caseID {
			out = append(out, c)
		}
	}
	m.cases = out
	return nil
}

func (m *mockRepo) CreateRun(_ context.Context, r Run) (Run, error) {
	m.runs = append(m.runs, r)
	return r, nil
}

func (m *mockRepo) GetRun(_ context.Context, id string) (Run, error) {
	for _, r := range m.runs {
		if r.ID == id {
			return r, nil
		}
	}
	return Run{}, errors.New("run not found")
}

func (m *mockRepo) UpdateRun(_ context.Context, r Run) error {
	for i := range m.runs {
		if m.runs[i].ID == r.ID {
			m.runs[i] = r
			return nil
		}
	}
	m.runs = append(m.runs, r)
	return nil
}

func (m *mockRepo) DeleteRun(_ context.Context, _ string) error {
	return nil
}

func (m *mockRepo) ListRuns(_ context.Context, datasetID, agentID string, _, _ int) ([]Run, int, error) {
	var out []Run
	for _, r := range m.runs {
		if (datasetID == "" || r.DatasetID == datasetID) && (agentID == "" || r.AgentID == agentID) {
			out = append(out, r)
		}
	}
	return out, len(out), nil
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

func (m *mockRepo) GetGateConfig(_ context.Context, _ string) (GateConfig, error) {
	return m.gateCfg, nil
}

func (m *mockRepo) UpsertGateConfig(_ context.Context, cfg GateConfig) error {
	m.gateCfg = cfg
	return nil
}

func (m *mockRepo) FailStaleRuns(_ context.Context, cutoff time.Time) (int, error) {
	m.staleCutoff = cutoff
	return 0, nil
}

func TestUploadCases(t *testing.T) {
	t.Run("valid JSON array with cases", func(t *testing.T) {
		repo := &mockRepo{}
		uc := NewUsecase(StoresFrom(repo), loggateway.NewNoop())
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
		uc := NewUsecase(StoresFrom(repo), loggateway.NewNoop())
		_, err := uc.UploadCases(context.Background(), "ds-1", `[]`)
		if err == nil {
			t.Fatal("expected error for empty array, got nil")
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		repo := &mockRepo{}
		uc := NewUsecase(StoresFrom(repo), loggateway.NewNoop())
		_, err := uc.UploadCases(context.Background(), "ds-1", `{not json}`)
		if err == nil {
			t.Fatal("expected error for invalid JSON, got nil")
		}
	})

	t.Run("cases with empty Input field are filtered", func(t *testing.T) {
		repo := &mockRepo{}
		uc := NewUsecase(StoresFrom(repo), loggateway.NewNoop())
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
		uc := NewUsecase(StoresFrom(repo), loggateway.NewNoop())
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
		uc := NewUsecase(StoresFrom(repo), loggateway.NewNoop())
		_, err := uc.UploadCases(context.Background(), "", `[{"input":"x"}]`)
		if err == nil {
			t.Fatal("expected error for empty dataset ID, got nil")
		}
	})

	t.Run("whitespace dataset ID", func(t *testing.T) {
		repo := &mockRepo{}
		uc := NewUsecase(StoresFrom(repo), loggateway.NewNoop())
		_, err := uc.UploadCases(context.Background(), "   ", `[{"input":"x"}]`)
		if err == nil {
			t.Fatal("expected error for whitespace dataset ID, got nil")
		}
	})

	t.Run("repo InsertCases error", func(t *testing.T) {
		repo := &mockRepo{insertCasesErr: errors.New("db error")}
		uc := NewUsecase(StoresFrom(repo), loggateway.NewNoop())
		_, err := uc.UploadCases(context.Background(), "ds-1", `[{"input":"x"}]`)
		if err == nil {
			t.Fatal("expected error from repo, got nil")
		}
	})

	t.Run("all cases have empty input", func(t *testing.T) {
		repo := &mockRepo{}
		uc := NewUsecase(StoresFrom(repo), loggateway.NewNoop())
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
		uc := NewUsecase(StoresFrom(repo), loggateway.NewNoop())
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
		uc := NewUsecase(StoresFrom(repo), loggateway.NewNoop())
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
		uc := NewUsecase(StoresFrom(repo), loggateway.NewNoop())
		_, err := uc.UploadCases(context.Background(), "ds-1", `{"input":"x"}`)
		if err == nil {
			t.Fatal("expected error for JSON object, got nil")
		}
	})
}
