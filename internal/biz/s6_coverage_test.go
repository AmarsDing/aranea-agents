package biz_test

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
)

// --- memKnowledgeRepo ---

type memKnowledgeRepo struct {
	collections map[string]biz.KnowledgeCollection
	documents   map[string]biz.KnowledgeDocument
	chunks      []biz.KnowledgeChunk
}

func newMemKnowledgeRepo() *memKnowledgeRepo {
	return &memKnowledgeRepo{
		collections: make(map[string]biz.KnowledgeCollection),
		documents:   make(map[string]biz.KnowledgeDocument),
	}
}

func (m *memKnowledgeRepo) CreateCollection(_ context.Context, c biz.KnowledgeCollection) (biz.KnowledgeCollection, error) {
	m.collections[c.ID] = c
	return c, nil
}
func (m *memKnowledgeRepo) GetCollection(_ context.Context, id string) (biz.KnowledgeCollection, error) {
	c, ok := m.collections[id]
	if !ok {
		return biz.KnowledgeCollection{}, biz.ErrNotFound
	}
	return c, nil
}
func (m *memKnowledgeRepo) ListCollections(_ context.Context, _ string, limit, _ int) ([]biz.KnowledgeCollection, int, error) {
	var out []biz.KnowledgeCollection
	for _, c := range m.collections {
		out = append(out, c)
	}
	return out, len(out), nil
}
func (m *memKnowledgeRepo) DeleteCollection(_ context.Context, id string) error {
	delete(m.collections, id)
	return nil
}
func (m *memKnowledgeRepo) UpdateCollectionCounts(_ context.Context, id string, docD, chunkD int) error {
	c := m.collections[id]
	c.DocumentCount += docD
	c.ChunkCount += chunkD
	m.collections[id] = c
	return nil
}
func (m *memKnowledgeRepo) CreateDocument(_ context.Context, d biz.KnowledgeDocument) (biz.KnowledgeDocument, error) {
	m.documents[d.ID] = d
	return d, nil
}
func (m *memKnowledgeRepo) GetDocument(_ context.Context, id string) (biz.KnowledgeDocument, error) {
	d, ok := m.documents[id]
	if !ok {
		return biz.KnowledgeDocument{}, biz.ErrNotFound
	}
	return d, nil
}
func (m *memKnowledgeRepo) UpdateDocumentStatus(_ context.Context, id, status, errMsg string, cc int) error {
	d := m.documents[id]
	d.Status = status
	d.ErrorMessage = errMsg
	d.ChunkCount = cc
	m.documents[id] = d
	return nil
}
func (m *memKnowledgeRepo) ListDocuments(_ context.Context, _ string, _, _ int) ([]biz.KnowledgeDocument, int, error) {
	var out []biz.KnowledgeDocument
	for _, d := range m.documents {
		out = append(out, d)
	}
	return out, len(out), nil
}
func (m *memKnowledgeRepo) DeleteDocument(_ context.Context, id string) error {
	delete(m.documents, id)
	return nil
}
func (m *memKnowledgeRepo) InsertChunks(_ context.Context, chunks []biz.KnowledgeChunk) error {
	m.chunks = append(m.chunks, chunks...)
	return nil
}
func (m *memKnowledgeRepo) DeleteChunksByDocument(_ context.Context, _ string) error { return nil }
func (m *memKnowledgeRepo) SearchChunks(_ context.Context, q biz.KnowledgeSearchQuery, _ []float32) ([]biz.KnowledgeChunk, error) {
	return m.chunks, nil
}

// --- Tests ---

func TestKnowledgeUsecase_UnavailableWithoutRepo(t *testing.T) {
	uc := biz.NewKnowledgeUsecase(nil)
	ctx := context.Background()
	_, _, err := uc.ListCollections(ctx, "", 10, 0)
	if err == nil {
		t.Fatal("expected error when repo is nil")
	}
}

func TestKnowledgeUsecase_CreateCollection(t *testing.T) {
	uc := biz.NewKnowledgeUsecase(newMemKnowledgeRepo())
	c, err := uc.CreateCollection(context.Background(), biz.KnowledgeCollection{
		Name:           "test",
		EmbeddingModel: "text-embedding-3-small",
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.ID == "" {
		t.Error("expected non-empty ID")
	}
	if c.Status != "active" {
		t.Errorf("expected status=active, got %q", c.Status)
	}
}

func TestKnowledgeUsecase_CreateCollectionValidation(t *testing.T) {
	uc := biz.NewKnowledgeUsecase(newMemKnowledgeRepo())
	_, err := uc.CreateCollection(context.Background(), biz.KnowledgeCollection{EmbeddingModel: "x"})
	if err == nil {
		t.Error("expected error for missing name")
	}
	_, err = uc.CreateCollection(context.Background(), biz.KnowledgeCollection{Name: "x"})
	if err == nil {
		t.Error("expected error for missing embedding_model")
	}
}

func TestKnowledgeUsecase_DeleteCollection(t *testing.T) {
	repo := newMemKnowledgeRepo()
	uc := biz.NewKnowledgeUsecase(repo)
	c, _ := uc.CreateCollection(context.Background(), biz.KnowledgeCollection{Name: "x", EmbeddingModel: "m"})
	if err := uc.DeleteCollection(context.Background(), c.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := repo.collections[c.ID]; ok {
		t.Error("expected collection to be deleted")
	}
}

func TestKnowledgeUsecase_Search_EmptyInput(t *testing.T) {
	uc := biz.NewKnowledgeUsecase(newMemKnowledgeRepo())
	_, err := uc.Search(context.Background(), biz.KnowledgeSearchQuery{}, nil)
	if err == nil {
		t.Error("expected error for empty collection_id")
	}
}

// --- EvalUsecase tests ---

type memEvalRepo2 struct {
	datasets map[string]biz.EvalDataset
	cases    []biz.EvalCase
	runs     map[string]biz.EvalRun
	results  []biz.EvalCaseResult
}

func newMemEvalRepo2() *memEvalRepo2 {
	return &memEvalRepo2{
		datasets: make(map[string]biz.EvalDataset),
		runs:     make(map[string]biz.EvalRun),
	}
}

func (m *memEvalRepo2) CreateDataset(_ context.Context, d biz.EvalDataset) (biz.EvalDataset, error) {
	m.datasets[d.ID] = d
	return d, nil
}
func (m *memEvalRepo2) GetDataset(_ context.Context, id string) (biz.EvalDataset, error) {
	d, ok := m.datasets[id]
	if !ok {
		return biz.EvalDataset{}, biz.ErrNotFound
	}
	return d, nil
}
func (m *memEvalRepo2) ListDatasets(_ context.Context, _ string, _, _ int) ([]biz.EvalDataset, int, error) {
	var out []biz.EvalDataset
	for _, d := range m.datasets {
		out = append(out, d)
	}
	return out, len(out), nil
}
func (m *memEvalRepo2) DeleteDataset(_ context.Context, id string) error {
	delete(m.datasets, id)
	return nil
}
func (m *memEvalRepo2) UpdateDatasetCaseCount(_ context.Context, id string, delta int) error {
	d := m.datasets[id]
	d.CaseCount += delta
	m.datasets[id] = d
	return nil
}
func (m *memEvalRepo2) InsertCases(_ context.Context, cases []biz.EvalCase) error {
	m.cases = append(m.cases, cases...)
	return nil
}
func (m *memEvalRepo2) ListCases(_ context.Context, _ string) ([]biz.EvalCase, error) {
	return m.cases, nil
}
func (m *memEvalRepo2) CreateRun(_ context.Context, r biz.EvalRun) (biz.EvalRun, error) {
	m.runs[r.ID] = r
	return r, nil
}
func (m *memEvalRepo2) GetRun(_ context.Context, id string) (biz.EvalRun, error) {
	r, ok := m.runs[id]
	if !ok {
		return biz.EvalRun{}, biz.ErrNotFound
	}
	return r, nil
}
func (m *memEvalRepo2) UpdateRun(_ context.Context, r biz.EvalRun) error {
	m.runs[r.ID] = r
	return nil
}
func (m *memEvalRepo2) ListRuns(_ context.Context, _, _ string, _, _ int) ([]biz.EvalRun, int, error) {
	var out []biz.EvalRun
	for _, r := range m.runs {
		out = append(out, r)
	}
	return out, len(out), nil
}
func (m *memEvalRepo2) InsertCaseResult(_ context.Context, r biz.EvalCaseResult) error {
	m.results = append(m.results, r)
	return nil
}
func (m *memEvalRepo2) ListCaseResults(_ context.Context, _ string, _, _ int) ([]biz.EvalCaseResult, int, error) {
	return m.results, len(m.results), nil
}

func TestEvalUsecase_CreateDataset(t *testing.T) {
	uc := biz.NewEvalUsecase(newMemEvalRepo2())
	d, err := uc.CreateDataset(context.Background(), biz.EvalDataset{Name: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if d.ID == "" {
		t.Error("expected non-empty ID")
	}
}

func TestEvalUsecase_UploadCases(t *testing.T) {
	repo := newMemEvalRepo2()
	uc := biz.NewEvalUsecase(repo)
	d, _ := uc.CreateDataset(context.Background(), biz.EvalDataset{Name: "test"})
	n, err := uc.UploadCases(context.Background(), d.ID, `[{"input":"q1","expected_output":"a1"},{"input":"q2","expected_output":"a2"}]`)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("expected 2 uploaded, got %d", n)
	}
}

func TestEvalUsecase_UploadCasesInvalidJSON(t *testing.T) {
	uc := biz.NewEvalUsecase(newMemEvalRepo2())
	_, err := uc.UploadCases(context.Background(), "some-id", "not-json")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestEvalUsecase_CreateRun(t *testing.T) {
	uc := biz.NewEvalUsecase(newMemEvalRepo2())
	r, err := uc.CreateRun(context.Background(), biz.EvalRun{DatasetID: "ds1", AgentID: "ag1"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pending" {
		t.Errorf("expected status=pending, got %q", r.Status)
	}
}

// --- A2AUsecase tests ---

type memA2ARepo struct {
	cards       map[string]biz.A2AAgentCard
	invocations map[string]biz.A2AInvocation
	audit       []biz.A2AAuditEntry
}

func newMemA2ARepo() *memA2ARepo {
	return &memA2ARepo{
		cards:       make(map[string]biz.A2AAgentCard),
		invocations: make(map[string]biz.A2AInvocation),
	}
}

func (m *memA2ARepo) UpsertAgentCard(_ context.Context, card biz.A2AAgentCard) (biz.A2AAgentCard, error) {
	m.cards[card.AgentID] = card
	return card, nil
}
func (m *memA2ARepo) GetAgentCard(_ context.Context, agentID string) (biz.A2AAgentCard, error) {
	c, ok := m.cards[agentID]
	if !ok {
		return biz.A2AAgentCard{}, biz.ErrNotFound
	}
	return c, nil
}
func (m *memA2ARepo) ListEnabledCards(_ context.Context, _, _ string) ([]biz.A2AAgentCard, error) {
	var out []biz.A2AAgentCard
	for _, c := range m.cards {
		if c.Enabled {
			out = append(out, c)
		}
	}
	return out, nil
}
func (m *memA2ARepo) CreateInvocation(_ context.Context, inv biz.A2AInvocation) (biz.A2AInvocation, error) {
	m.invocations[inv.ID] = inv
	return inv, nil
}
func (m *memA2ARepo) UpdateInvocation(_ context.Context, inv biz.A2AInvocation) error {
	m.invocations[inv.ID] = inv
	return nil
}
func (m *memA2ARepo) InsertAudit(_ context.Context, e biz.A2AAuditEntry) error {
	m.audit = append(m.audit, e)
	return nil
}
func (m *memA2ARepo) ListAudit(_ context.Context, _, _ string, _, _ int) ([]biz.A2AAuditEntry, int, error) {
	return m.audit, len(m.audit), nil
}

func TestA2AUsecase_UpdateAndGetCard(t *testing.T) {
	uc := biz.NewA2AUsecase(newMemA2ARepo())
	card, err := uc.UpdateAgentCard(context.Background(), biz.A2AAgentCard{
		AgentID: "agent-1",
		Enabled: true,
		Capabilities: []biz.A2ACapability{
			{Name: "summarize", Description: "Summarize text"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := uc.GetAgentCard(context.Background(), card.AgentID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Enabled {
		t.Error("expected card to be enabled")
	}
	if len(got.Capabilities) != 1 {
		t.Errorf("expected 1 capability, got %d", len(got.Capabilities))
	}
}

func TestA2AUsecase_DisabledByDefault(t *testing.T) {
	uc := biz.NewA2AUsecase(newMemA2ARepo())
	// Agent doesn't exist → ErrNotFound
	_, err := uc.GetAgentCard(context.Background(), "unknown")
	if err == nil {
		t.Error("expected error for unknown agent")
	}
}

func TestA2AUsecase_StartInvocation(t *testing.T) {
	uc := biz.NewA2AUsecase(newMemA2ARepo())
	inv, err := uc.StartInvocation(context.Background(), biz.A2AInvocation{
		CalleeAgentID: "agent-2",
		Capability:    "summarize",
		PayloadJSON:   `{"text":"hello"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if inv.Status != "pending" {
		t.Errorf("expected status=pending, got %q", inv.Status)
	}
	if inv.ID == "" {
		t.Error("expected non-empty ID")
	}
}
