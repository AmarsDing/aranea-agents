package biz

import (
	"aranea-agents/internal/biz/shared"
	"context"
	"testing"

	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"
)

type duplicateAgentRepo struct {
	AgentRepository
	src     Agent
	created Agent
}

func (r *duplicateAgentRepo) GetAgentByID(ctx context.Context, id string) (Agent, error) {
	if id == r.created.ID {
		a := r.created
		a.Files = []AgentPromptFile{{Name: "SOUL.md", Body: "hello", AgentID: id}}
		return a, nil
	}
	if id == r.src.ID {
		return r.src, nil
	}
	return Agent{}, shared.ErrNotFound
}

func (r *duplicateAgentRepo) GetAgentByAgentKey(ctx context.Context, key string) (Agent, error) {
	return Agent{}, shared.ErrNotFound
}

func (r *duplicateAgentRepo) CreateAgent(ctx context.Context, a Agent) (Agent, error) {
	r.created = a
	return r.created, nil
}

func (r *duplicateAgentRepo) UpdateAgent(ctx context.Context, a Agent) (Agent, error) {
	return a, nil
}

func (r *duplicateAgentRepo) ListExtrasForAgents(ctx context.Context, agentIDs []string) (map[string]AgentListExtras, error) {
	return map[string]AgentListExtras{}, nil
}

func (r *duplicateAgentRepo) ListAgentCreators(ctx context.Context) ([]AgentCreator, error) {
	return nil, nil
}

func (r *duplicateAgentRepo) GetAgentRuntimeSettings(ctx context.Context, agentID string) (AgentRuntimeSettings, error) {
	return AgentRuntimeSettings{AgentID: agentID}, nil
}

func (r *duplicateAgentRepo) UpsertAgentRuntimeSettings(ctx context.Context, s AgentRuntimeSettings) (AgentRuntimeSettings, error) {
	return s, nil
}

func (r *duplicateAgentRepo) ListAgentPromptFiles(ctx context.Context, agentID string) ([]AgentPromptFile, error) {
	if agentID == r.src.ID || agentID == r.created.ID {
		return []AgentPromptFile{{ID: "f1", Name: "SOUL.md", Body: "hello", AgentID: agentID}}, nil
	}
	return nil, nil
}

func (r *duplicateAgentRepo) ReplaceAgentPromptFiles(ctx context.Context, agentID string, files []AgentPromptFile) ([]AgentPromptFile, error) {
	return files, nil
}

func (r *duplicateAgentRepo) ExecInTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}
func (r *duplicateAgentRepo) ClearPositionByDepartment(context.Context, string) (int, error) {
	return 0, nil
}
func (r *duplicateAgentRepo) ToggleFavorite(context.Context, string) (Agent, error) {
	return Agent{}, nil
}

func TestDuplicate_copiesFilesWithNewIDs(t *testing.T) {
	src := Agent{
		ID:          "src-1",
		AgentKey:    "demo",
		DisplayName: "Demo",
		Provider:    "openai",
		Model:       "gpt-4",
	}
	repo := &duplicateAgentRepo{src: src}
	uc := NewAgentUsecase(AgentUsecaseDeps{Reader: repo, Writer: repo, Settings: repo, Files: repo, Position: repo, Tx: repo, Lg: loggateway.NewNoop()})
	got, err := uc.Duplicate(context.Background(), "src-1")
	if err != nil {
		t.Fatalf("Duplicate: %v", err)
	}
	if got.AgentKey == src.AgentKey {
		t.Fatalf("expected new agent_key, got %q", got.AgentKey)
	}
	if len(repo.created.Files) != 1 || repo.created.Files[0].ID != "" {
		t.Fatalf("expected copied file without id, got %+v", repo.created.Files)
	}
}

// P2-B tenant binding: a non-system caller's duplicate always lands in the
// caller's workspace (duplicating a shared/built-in row must not mint another
// shared row visible to every tenant); system callers keep the source
// workspace semantics.
func TestDuplicate_tenantBinding(t *testing.T) {
	cases := []struct {
		name   string
		ctx    context.Context
		srcWS  string
		wantWS string
	}{
		{
			name:   "tenant ctx duplicating shared row binds to caller workspace",
			ctx:    workspace.WithContext(context.Background(), "ws-a"),
			srcWS:  "",
			wantWS: "ws-a",
		},
		{
			name:   "tenant ctx duplicating own row keeps ownership",
			ctx:    workspace.WithContext(context.Background(), "ws-a"),
			srcWS:  "ws-a",
			wantWS: "ws-a",
		},
		{
			name:   "system ctx duplicating shared row keeps shared semantics",
			ctx:    workspace.WithSystemWorkspace(context.Background()),
			srcWS:  "",
			wantWS: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &duplicateAgentRepo{src: Agent{
				ID:          "src-1",
				AgentKey:    "demo",
				DisplayName: "Demo",
				Provider:    "openai",
				Model:       "gpt-4",
				WorkspaceID: tc.srcWS,
			}}
			uc := NewAgentUsecase(AgentUsecaseDeps{Reader: repo, Writer: repo, Settings: repo, Files: repo, Position: repo, Tx: repo, Lg: loggateway.NewNoop()})
			if _, err := uc.Duplicate(tc.ctx, "src-1"); err != nil {
				t.Fatalf("Duplicate: %v", err)
			}
			if repo.created.WorkspaceID != tc.wantWS {
				t.Fatalf("workspace_id = %q, want %q", repo.created.WorkspaceID, tc.wantWS)
			}
		})
	}
}

func TestDuplicate_setsCreatedByFromContext(t *testing.T) {
	src := Agent{
		ID:          "src-1",
		AgentKey:    "demo",
		DisplayName: "Demo",
		Provider:    "openai",
		Model:       "gpt-4",
		CreatedBy:   "99",
	}
	repo := &duplicateAgentRepo{src: src}
	uc := NewAgentUsecase(AgentUsecaseDeps{Reader: repo, Writer: repo, Settings: repo, Files: repo, Position: repo, Tx: repo, Lg: loggateway.NewNoop()})
	ctx := auth.NewContext(context.Background(), &auth.Auth{UserID: 42})
	if _, err := uc.Duplicate(ctx, "src-1"); err != nil {
		t.Fatalf("Duplicate: %v", err)
	}
	if repo.created.CreatedBy != "42" {
		t.Fatalf("created_by = %q, want %q from auth context", repo.created.CreatedBy, "42")
	}
}
