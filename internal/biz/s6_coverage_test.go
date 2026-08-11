package biz_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/evaluation"
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
func (m *memKnowledgeRepo) UpdateCollectionSyncState(_ context.Context, id, state string, lastSyncAt time.Time) error {
	c := m.collections[id]
	c.SyncState = state
	c.LastSyncAt = lastSyncAt.UTC().Format(time.RFC3339)
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
func (m *memKnowledgeRepo) GetDocumentByRelPath(_ context.Context, collectionID, relPath string) (biz.KnowledgeDocument, error) {
	for _, d := range m.documents {
		if d.CollectionID == collectionID && d.RelPath == relPath {
			return d, nil
		}
	}
	return biz.KnowledgeDocument{}, biz.ErrNotFound
}
func (m *memKnowledgeRepo) UpdateDocumentRelPath(_ context.Context, id, newRelPath string) error {
	d := m.documents[id]
	d.RelPath = newRelPath
	m.documents[id] = d
	return nil
}
func (m *memKnowledgeRepo) UpdateDocumentSyncMeta(_ context.Context, id string, meta biz.KnowledgeDocumentSyncMeta) error {
	d := m.documents[id]
	d.ContentHash = meta.ContentHash
	d.Summary = meta.Summary
	d.SummaryHash = meta.SummaryHash
	d.Tags = meta.Tags
	d.DocType = meta.DocType
	m.documents[id] = d
	return nil
}
func (m *memKnowledgeRepo) UpdateDocumentStatus(_ context.Context, id, status, errMsg string, cc int) error {
	d := m.documents[id]
	d.Status = status
	d.ErrorMessage = errMsg
	d.ChunkCount = cc
	m.documents[id] = d
	return nil
}
func (m *memKnowledgeRepo) UpdateDocumentContent(_ context.Context, id, contentText string, organized bool) error {
	d := m.documents[id]
	d.ContentText = contentText
	d.Organized = organized
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
func (m *memKnowledgeRepo) ListDocumentsPendingReembed(context.Context, string) ([]biz.KnowledgeDocument, error) {
	return nil, nil
}
func (m *memKnowledgeRepo) DeleteDocument(_ context.Context, id string) error {
	delete(m.documents, id)
	return nil
}
func (m *memKnowledgeRepo) MoveDocument(_ context.Context, id, targetCollectionID string) (biz.KnowledgeDocument, error) {
	d, ok := m.documents[id]
	if !ok {
		return biz.KnowledgeDocument{}, biz.ErrNotFound
	}
	d.CollectionID = targetCollectionID
	m.documents[id] = d
	return d, nil
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
	uc := biz.NewKnowledgeUsecase(nil, nil, nil)
	ctx := context.Background()
	_, _, err := uc.ListCollections(ctx, "", 10, 0)
	if err == nil {
		t.Fatal("expected error when repo is nil")
	}
}

func TestKnowledgeUsecase_CreateCollection(t *testing.T) {
	uc := biz.NewKnowledgeUsecase(newMemKnowledgeRepo(), newMemKnowledgeRepo(), newMemKnowledgeRepo())
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
	uc := biz.NewKnowledgeUsecase(newMemKnowledgeRepo(), newMemKnowledgeRepo(), newMemKnowledgeRepo())
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
	uc := biz.NewKnowledgeUsecase(repo, repo, repo)
	c, _ := uc.CreateCollection(context.Background(), biz.KnowledgeCollection{Name: "x", EmbeddingModel: "m"})
	if err := uc.DeleteCollection(context.Background(), c.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := repo.collections[c.ID]; ok {
		t.Error("expected collection to be deleted")
	}
}

func TestKnowledgeUsecase_Search_EmptyInput(t *testing.T) {
	uc := biz.NewKnowledgeUsecase(newMemKnowledgeRepo(), newMemKnowledgeRepo(), newMemKnowledgeRepo())
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
func (m *memEvalRepo2) InsertCasesWithCountUpdate(ctx context.Context, datasetID string, cases []biz.EvalCase) error {
	if err := m.InsertCases(ctx, cases); err != nil {
		return err
	}
	return m.UpdateDatasetCaseCount(ctx, datasetID, len(cases))
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
func (m *memEvalRepo2) ListCaseResults(_ context.Context, runID string, _, _ int) ([]biz.EvalCaseResult, int, error) {
	var out []biz.EvalCaseResult
	for _, r := range m.results {
		if r.RunID == runID {
			out = append(out, r)
		}
	}
	return out, len(out), nil
}
func (m *memEvalRepo2) GetCaseResult(_ context.Context, runID, resultID string) (biz.EvalCaseResult, error) {
	for _, r := range m.results {
		if r.RunID == runID && r.ID == resultID {
			return r, nil
		}
	}
	return biz.EvalCaseResult{}, errors.New("not found")
}

func (m *memEvalRepo2) DeleteRun(_ context.Context, id string) error {
	delete(m.runs, id)
	return nil
}

func (m *memEvalRepo2) UpdateDataset(_ context.Context, id, name, description string) (biz.EvalDataset, error) {
	d, ok := m.datasets[id]
	if !ok {
		return biz.EvalDataset{}, errors.New("not found")
	}
	d.Name = name
	d.Description = description
	m.datasets[id] = d
	return d, nil
}
func (m *memEvalRepo2) UpdateCaseResultAnnotation(_ context.Context, runID, resultID string, patch biz.EvalCaseResultAnnotation) (biz.EvalCaseResult, error) {
	for i, r := range m.results {
		if r.RunID == runID && r.ID == resultID {
			if patch.HumanPass != nil {
				m.results[i].HumanPass = patch.HumanPass
			}
			if patch.HumanScore != nil {
				m.results[i].HumanScore = patch.HumanScore
			}
			if patch.HumanComment != nil {
				m.results[i].HumanComment = *patch.HumanComment
			}
			m.results[i].AnnotatedAt = "now"
			m.results[i].AnnotatedBy = patch.AnnotatedBy
			return m.results[i], nil
		}
	}
	return biz.EvalCaseResult{}, errors.New("not found")
}

func (m *memEvalRepo2) ListTrendPoints(_ context.Context, agentID, datasetID string, limit int) ([]biz.EvalTrendPoint, error) {
	var out []biz.EvalTrendPoint
	for _, r := range m.runs {
		if r.AgentID != agentID || r.Status != "completed" {
			continue
		}
		if datasetID != "" && r.DatasetID != datasetID {
			continue
		}
		out = append(out, biz.EvalTrendPoint{
			RunID:              r.ID,
			CreatedAt:          r.CreatedAt,
			TriggerSource:      r.TriggerSource,
			ExactMatchScore:    r.ExactMatchScore,
			ContainsMatchScore: r.ContainsMatchScore,
			LLMJudgeScore:      r.LLMJudgeScore,
			ToolCallAccuracy:   r.ToolCallAccuracy,
			PassAtK:            r.PassAtK,
			PassHatK:           r.PassHatK,
		})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (m *memEvalRepo2) GetRunsByIDs(_ context.Context, ids []string) ([]biz.EvalRun, error) {
	out := make([]biz.EvalRun, 0, len(ids))
	for _, id := range ids {
		if r, ok := m.runs[id]; ok {
			out = append(out, r)
		}
	}
	return out, nil
}

// Stub for evaluation.Repo.ListJudgeAnnotatedResults (judge calibration P1-3);
// s6 coverage tests don't exercise the calibration path.
func (m *memEvalRepo2) ListJudgeAnnotatedResults(_ context.Context, _, _ string) ([]evaluation.JudgeAnnotatedResult, error) {
	return nil, nil
}

// Stubs for evaluation.Repo governance methods (P2-1/P2-3/P3-3); s6 coverage
// tests don't exercise failure grouping, pairwise preference, or the gate.
func (m *memEvalRepo2) ListFailureGroups(_ context.Context, _, _ string, _ int) ([]evaluation.FailureGroup, int, error) {
	return nil, 0, nil
}

func (m *memEvalRepo2) InsertRunPreference(_ context.Context, _ evaluation.RunPreference) error {
	return nil
}

func (m *memEvalRepo2) ListRunPreferences(_ context.Context, _ string, _ int) ([]evaluation.RunPreference, error) {
	return nil, nil
}

func (m *memEvalRepo2) GetGateConfig(_ context.Context) (evaluation.GateConfig, error) {
	return evaluation.GateConfig{}, nil
}

func (m *memEvalRepo2) UpsertGateConfig(_ context.Context, _ evaluation.GateConfig) error {
	return nil
}

func TestEvalUsecase_CreateDataset(t *testing.T) {
	uc := biz.NewEvalUsecase(newMemEvalRepo2(), nil)
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
	uc := biz.NewEvalUsecase(repo, nil)
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
	uc := biz.NewEvalUsecase(newMemEvalRepo2(), nil)
	_, err := uc.UploadCases(context.Background(), "some-id", "not-json")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestEvalUsecase_CreateRun(t *testing.T) {
	uc := biz.NewEvalUsecase(newMemEvalRepo2(), nil)
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
func (m *memA2ARepo) MapEndpointEnabled(_ context.Context, agentIDs []string) (map[string]bool, error) {
	out := make(map[string]bool, len(agentIDs))
	for _, id := range agentIDs {
		if c, ok := m.cards[id]; ok {
			out[id] = c.Enabled
		}
	}
	return out, nil
}
func (m *memA2ARepo) CreateRemoteAgent(_ context.Context, agent biz.A2ARemoteAgent) (biz.A2ARemoteAgent, error) {
	return agent, nil
}
func (m *memA2ARepo) ListRemoteAgents(_ context.Context, _ string) ([]biz.A2ARemoteAgent, error) {
	return nil, nil
}
func (m *memA2ARepo) DeleteRemoteAgent(_ context.Context, _ string) error { return nil }
func (m *memA2ARepo) GetRemoteAgent(_ context.Context, id string) (biz.A2ARemoteAgent, error) {
	return biz.A2ARemoteAgent{}, biz.ErrNotFound
}
func (m *memA2ARepo) DiscoverRemoteCard(_ context.Context, _ biz.RemoteCardDiscoverInput) (biz.A2AAgentCard, error) {
	return biz.A2AAgentCard{}, nil
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

func (m *memA2ARepo) UpdateRemoteAgentHealth(_ context.Context, _ string, _ bool, _ string) error {
	return nil
}

func TestA2AUsecase_UpdateAndGetCard(t *testing.T) {
	repo := newMemA2ARepo()
	uc := biz.NewA2AUsecase(repo, repo, repo, repo, nil)
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
	repo := newMemA2ARepo()
	uc := biz.NewA2AUsecase(repo, repo, repo, repo, nil)
	// Agent doesn't exist → ErrNotFound
	_, err := uc.GetAgentCard(context.Background(), "unknown")
	if err == nil {
		t.Error("expected error for unknown agent")
	}
}

func TestA2AUsecase_StartInvocation(t *testing.T) {
	repo := newMemA2ARepo()
	uc := biz.NewA2AUsecase(repo, repo, repo, repo, nil)
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
