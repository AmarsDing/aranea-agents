package biz

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestDecideMemoryConflict(t *testing.T) {
	tests := []struct {
		name       string
		kind       string
		neighbors  []MemoryConflictNeighbor
		wantAction ConflictAction
		wantTarget string
	}{
		{"non-governable kind ignored", "knowledge", []MemoryConflictNeighbor{{FactID: "f1", Score: 0.99, FactKind: "knowledge"}}, ConflictActionNone, ""},
		{"fact kind ignored", "fact", []MemoryConflictNeighbor{{FactID: "f1", Score: 0.99, FactKind: "fact"}}, ConflictActionNone, ""},
		{"event kind ignored", "event", []MemoryConflictNeighbor{{FactID: "f1", Score: 0.99, FactKind: "event"}}, ConflictActionNone, ""},
		{"no neighbors", "preference", nil, ConflictActionNone, ""},
		{"supersede on high same-kind", "preference", []MemoryConflictNeighbor{{FactID: "f1", Score: 0.95, FactKind: "preference"}}, ConflictActionSupersede, "f1"},
		{"high different-kind marks conflict", "preference", []MemoryConflictNeighbor{{FactID: "f1", Score: 0.95, FactKind: "constraint"}}, ConflictActionMarkConflict, "f1"},
		{"mid band marks conflict", "constraint", []MemoryConflictNeighbor{{FactID: "f1", Score: 0.85, FactKind: "preference"}}, ConflictActionMarkConflict, "f1"},
		{"below band no action", "preference", []MemoryConflictNeighbor{{FactID: "f1", Score: 0.79, FactKind: "preference"}}, ConflictActionNone, ""},
		{"boundary 0.92 supersedes", "profile", []MemoryConflictNeighbor{{FactID: "f1", Score: 0.92, FactKind: "profile"}}, ConflictActionSupersede, "f1"},
		{"boundary 0.80 marks", "preference", []MemoryConflictNeighbor{{FactID: "f1", Score: 0.80, FactKind: "preference"}}, ConflictActionMarkConflict, "f1"},
		{"supersede wins over mark", "preference", []MemoryConflictNeighbor{
			{FactID: "f-mark", Score: 0.85, FactKind: "preference"},
			{FactID: "f-super", Score: 0.96, FactKind: "preference"},
		}, ConflictActionSupersede, "f-super"},
		{"picks highest same-kind for supersede", "preference", []MemoryConflictNeighbor{
			{FactID: "f1", Score: 0.93, FactKind: "preference"},
			{FactID: "f2", Score: 0.97, FactKind: "preference"},
		}, ConflictActionSupersede, "f2"},
		{"skips empty fact ids", "preference", []MemoryConflictNeighbor{{FactID: "", Score: 0.99, FactKind: "preference"}}, ConflictActionNone, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecideMemoryConflict(tt.kind, tt.neighbors)
			if got.Action != tt.wantAction || got.TargetFactID != tt.wantTarget {
				t.Fatalf("DecideMemoryConflict(%q)=%+v, want action=%q target=%q", tt.kind, got, tt.wantAction, tt.wantTarget)
			}
		})
	}
}

func TestIsConflictGovernableFactKind(t *testing.T) {
	for _, k := range []string{"preference", "constraint", "profile"} {
		if !IsConflictGovernableFactKind(k) {
			t.Fatalf("kind %q should be governable", k)
		}
	}
	for _, k := range []string{"fact", "event", "knowledge", "", "other"} {
		if IsConflictGovernableFactKind(k) {
			t.Fatalf("kind %q should not be governable", k)
		}
	}
}

type fakeNeighborSearcher struct {
	neighbors []MemoryConflictNeighbor
	err       error
	called    bool
}

func (f *fakeNeighborSearcher) SearchFactNeighbors(_ context.Context, _, _ string, _ []float32, _ int, _ float64) ([]MemoryConflictNeighbor, error) {
	f.called = true
	return f.neighbors, f.err
}

type fakeConflictRowReader struct {
	kindByID map[string]string
}

func (f fakeConflictRowReader) GetFactRowsByIDs(_ context.Context, ids []string) ([][]byte, error) {
	var out [][]byte
	for _, id := range ids {
		if kind, ok := f.kindByID[id]; ok {
			b, _ := json.Marshal(map[string]any{"id": id, "fact_kind": kind})
			out = append(out, b)
		}
	}
	return out, nil
}

type fakeConflictEmbedder struct{ called bool }

func (f *fakeConflictEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	f.called = true
	return []float32{0.1, 0.2}, nil
}

func TestMemoryConflictDetector_DetectConflict(t *testing.T) {
	searcher := &fakeNeighborSearcher{neighbors: []MemoryConflictNeighbor{{FactID: "f1", Score: 0.95}}}
	reader := fakeConflictRowReader{kindByID: map[string]string{"f1": "preference"}}
	det := NewMemoryConflictDetector(searcher, &fakeConflictEmbedder{}, reader)
	if det == nil {
		t.Fatal("detector should not be nil with valid deps")
	}
	dec, err := det.DetectConflict(context.Background(), "agent-1", "u1", "preference", "User prefers tea")
	if err != nil {
		t.Fatal(err)
	}
	if dec.Action != ConflictActionSupersede || dec.TargetFactID != "f1" {
		t.Fatalf("unexpected decision: %+v", dec)
	}
}

func TestMemoryConflictDetector_SkipsNonGovernableWithoutRecall(t *testing.T) {
	searcher := &fakeNeighborSearcher{}
	det := NewMemoryConflictDetector(searcher, &fakeConflictEmbedder{}, fakeConflictRowReader{})
	dec, err := det.DetectConflict(context.Background(), "a", "u", "knowledge", "x")
	if err != nil {
		t.Fatal(err)
	}
	if dec.Action != ConflictActionNone {
		t.Fatalf("want none, got %+v", dec)
	}
	if searcher.called {
		t.Fatal("searcher must not be called for non-governable kind")
	}
}

func TestMemoryConflictDetector_DegradesOnSearchError(t *testing.T) {
	searcher := &fakeNeighborSearcher{err: errors.New("pg down")}
	det := NewMemoryConflictDetector(searcher, &fakeConflictEmbedder{}, fakeConflictRowReader{})
	dec, err := det.DetectConflict(context.Background(), "a", "u", "preference", "x")
	if err != nil {
		t.Fatalf("degraded path must not return error, got %v", err)
	}
	if dec.Action != ConflictActionNone {
		t.Fatalf("want none, got %+v", dec)
	}
}

func TestMemoryConflictDetector_DropsInactiveNeighbors(t *testing.T) {
	searcher := &fakeNeighborSearcher{neighbors: []MemoryConflictNeighbor{{FactID: "f-gone", Score: 0.99}}}
	// reader returns no rows: f-gone is superseded/deleted, excluded from conflict input.
	det := NewMemoryConflictDetector(searcher, &fakeConflictEmbedder{}, fakeConflictRowReader{kindByID: map[string]string{}})
	dec, err := det.DetectConflict(context.Background(), "a", "u", "preference", "x")
	if err != nil {
		t.Fatal(err)
	}
	if dec.Action != ConflictActionNone {
		t.Fatalf("inactive neighbor must be dropped, got %+v", dec)
	}
}

func TestNewMemoryConflictDetector_NilDeps(t *testing.T) {
	if NewMemoryConflictDetector(nil, &fakeConflictEmbedder{}, fakeConflictRowReader{}) != nil {
		t.Fatal("nil searcher must yield nil detector")
	}
	if NewMemoryConflictDetector(&fakeNeighborSearcher{}, nil, fakeConflictRowReader{}) != nil {
		t.Fatal("nil embedder must yield nil detector")
	}
	if NewMemoryConflictDetector(&fakeNeighborSearcher{}, &fakeConflictEmbedder{}, nil) != nil {
		t.Fatal("nil reader must yield nil detector")
	}
}
