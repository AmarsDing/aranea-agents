package evaluation

import (
	"context"
	"testing"

	"aranea-agents/pkg/loggateway"
)

type memVersionStore struct {
	rows []DatasetVersion
}

func (m *memVersionStore) InsertVersion(_ context.Context, v DatasetVersion) (DatasetVersion, error) {
	m.rows = append(m.rows, v)
	return v, nil
}

func (m *memVersionStore) GetVersion(_ context.Context, id string) (DatasetVersion, error) {
	for _, v := range m.rows {
		if v.ID == id {
			return v, nil
		}
	}
	return DatasetVersion{}, nil
}

func (m *memVersionStore) ListVersions(_ context.Context, datasetID string, limit int) ([]DatasetVersion, error) {
	var out []DatasetVersion
	for i := len(m.rows) - 1; i >= 0; i-- {
		if m.rows[i].DatasetID == datasetID {
			out = append(out, m.rows[i])
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func TestSnapshotDatasetReusesSameHash(t *testing.T) {
	repo := &mockRepo{cases: []Case{{ID: "c1", DatasetID: "ds1", Input: "hi", ExpectedOutput: "yo"}}}
	vs := &memVersionStore{}
	uc := NewUsecase(Stores{Cases: repo, Versions: vs}, loggateway.NewNoop())
	a, err := uc.SnapshotDataset(context.Background(), "ds1")
	if err != nil || a.Version != 1 {
		t.Fatalf("first snap: %#v err=%v", a, err)
	}
	b, err := uc.SnapshotDataset(context.Background(), "ds1")
	if err != nil || b.ID != a.ID {
		t.Fatalf("same hash must reuse version, got %#v", b)
	}
	repo.cases[0].Input = "changed"
	c, err := uc.SnapshotDataset(context.Background(), "ds1")
	if err != nil || c.Version != 2 {
		t.Fatalf("changed hash must bump version, got %#v err=%v", c, err)
	}
}

func TestCreateRunPinsRequestedVersion(t *testing.T) {
	repo := &mockRepo{cases: []Case{{ID: "c1", DatasetID: "ds1", Input: "live"}}}
	vs := &memVersionStore{rows: []DatasetVersion{{
		ID: "ver-old", DatasetID: "ds1", Version: 3, Hash: "abc",
		Cases: []Case{{ID: "c1", DatasetID: "ds1", Input: "frozen"}},
	}}}
	uc := NewUsecase(Stores{Cases: repo, Runs: repo, Versions: vs}, loggateway.NewNoop())
	run, err := uc.CreateRun(context.Background(), Run{
		DatasetID: "ds1", AgentID: "a1", DatasetVersionID: "ver-old",
	})
	if err != nil {
		t.Fatal(err)
	}
	if run.DatasetVersionID != "ver-old" || run.DatasetVersion != 3 || run.DatasetHash != "abc" {
		t.Fatalf("pin lost: %#v", run)
	}
}

func TestCreateRunRejectsForeignVersion(t *testing.T) {
	repo := &mockRepo{cases: []Case{{ID: "c1", DatasetID: "ds1", Input: "live"}}}
	vs := &memVersionStore{rows: []DatasetVersion{{
		ID: "ver-x", DatasetID: "other", Version: 1, Hash: "x",
	}}}
	uc := NewUsecase(Stores{Cases: repo, Runs: repo, Versions: vs}, loggateway.NewNoop())
	if _, err := uc.CreateRun(context.Background(), Run{
		DatasetID: "ds1", AgentID: "a1", DatasetVersionID: "ver-x",
	}); err == nil {
		t.Fatal("foreign version must be rejected")
	}
}
