package a2a

import (
	"context"
	"errors"
	"strings"
	"testing"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

func TestListRemoteAgents(t *testing.T) {
	tests := []struct {
		name      string
		workspace string
		setup     func(*mockUsecaseRepo)
		wantErr   bool
		wantCode  int32
		check     func(t *testing.T, agents []RemoteAgent)
	}{
		{
			name:      "returns_remote_agents_from_repo",
			workspace: "ws-1",
			setup: func(r *mockUsecaseRepo) {
				r.listRemoteAgentsFn = func(_ context.Context, ws string) ([]RemoteAgent, error) {
					return []RemoteAgent{
						{ID: "r-1", Workspace: ws, DisplayName: "Remote 1", Enabled: true},
						{ID: "r-2", Workspace: ws, DisplayName: "Remote 2", Enabled: false},
					}, nil
				}
			},
			check: func(t *testing.T, agents []RemoteAgent) {
				if len(agents) != 2 {
					t.Fatalf("len(agents) = %d, want 2", len(agents))
				}
				if agents[0].ID != "r-1" {
					t.Errorf("agents[0].ID = %q, want %q", agents[0].ID, "r-1")
				}
				if agents[1].ID != "r-2" {
					t.Errorf("agents[1].ID = %q, want %q", agents[1].ID, "r-2")
				}
			},
		},
		{
			name:      "empty_result",
			workspace: "ws-empty",
			setup: func(r *mockUsecaseRepo) {
				r.listRemoteAgentsFn = func(_ context.Context, _ string) ([]RemoteAgent, error) {
					return nil, nil
				}
			},
			check: func(t *testing.T, agents []RemoteAgent) {
				if len(agents) != 0 {
					t.Errorf("len(agents) = %d, want 0", len(agents))
				}
			},
		},
		{
			name:      "repo_error",
			workspace: "ws-1",
			setup: func(r *mockUsecaseRepo) {
				r.listRemoteAgentsFn = func(_ context.Context, _ string) ([]RemoteAgent, error) {
					return nil, errors.New("db connection lost")
				}
			},
			wantErr: true,
		},
		{
			name:      "nil_usecase_returns_internal_server",
			workspace: "ws-1",
			setup:     func(_ *mockUsecaseRepo) {},
			wantErr:   true,
			wantCode:  500,
			check: func(t *testing.T, _ []RemoteAgent) {
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockUsecaseRepo{}
			tt.setup(repo)
			uc := NewUsecase(repo, repo, repo, repo, nil)
			if tt.name == "nil_usecase_returns_internal_server" {
				var nilUc *Usecase
				agents, err := nilUc.ListRemoteAgents(context.Background(), tt.workspace)
				if err == nil {
					t.Fatal("ListRemoteAgents() expected error, got nil")
				}
				se := kerrors.FromError(err)
				if se == nil {
					t.Fatalf("expected kratos error, got %T", err)
				}
				if se.Code != tt.wantCode {
					t.Errorf("code = %d, want %d", se.Code, tt.wantCode)
				}
				if tt.check != nil {
					tt.check(t, agents)
				}
				return
			}
			agents, err := uc.ListRemoteAgents(context.Background(), tt.workspace)
			if tt.wantErr {
				if err == nil {
					t.Fatal("ListRemoteAgents() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("ListRemoteAgents() unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, agents)
			}
		})
	}
}

func TestDeleteRemoteAgent(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		setup    func(*mockUsecaseRepo)
		wantErr  bool
		wantCode int32
		check    func(t *testing.T, err error)
	}{
		{
			name: "valid_delete",
			id:   "r-1",
			setup: func(r *mockUsecaseRepo) {
				r.deleteRemoteAgentFn = func(_ context.Context, id string) error {
					if id != "r-1" {
						t.Errorf("delete called with id = %q, want %q", id, "r-1")
					}
					return nil
				}
			},
		},
		{
			name:     "empty_id_returns_bad_request",
			id:       "",
			wantErr:  true,
			wantCode: 400,
		},
		{
			name:     "whitespace_id_returns_bad_request",
			id:       "   ",
			wantErr:  true,
			wantCode: 400,
		},
		{
			name: "repo_error",
			id:   "r-1",
			setup: func(r *mockUsecaseRepo) {
				r.deleteRemoteAgentFn = func(_ context.Context, _ string) error {
					return errors.New("db delete failed")
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockUsecaseRepo{}
			if tt.setup != nil {
				tt.setup(repo)
			}
			uc := NewUsecase(repo, repo, repo, repo, nil)
			err := uc.DeleteRemoteAgent(context.Background(), tt.id)
			if tt.wantErr {
				if err == nil {
					t.Fatal("DeleteRemoteAgent() expected error, got nil")
				}
				if tt.wantCode != 0 {
					se := kerrors.FromError(err)
					if se == nil {
						t.Fatalf("expected kratos error, got %T", err)
					}
					if se.Code != tt.wantCode {
						t.Errorf("code = %d, want %d", se.Code, tt.wantCode)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("DeleteRemoteAgent() unexpected error: %v", err)
			}
		})
	}
}

func TestPersistRemoteHealth(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		ok       bool
		errMsg   string
		setup    func(*mockUsecaseRepo)
		wantErr  bool
		wantCode int32
	}{
		{
			name:   "valid_persist_ok_true",
			id:     "r-1",
			ok:     true,
			errMsg: "",
			setup: func(r *mockUsecaseRepo) {
				r.updateRemoteAgentHealthFn = func(_ context.Context, id string, ok bool, errMsg string) error {
					if id != "r-1" {
						t.Errorf("id = %q, want %q", id, "r-1")
					}
					if !ok {
						t.Errorf("ok = %v, want true", ok)
					}
					if errMsg != "" {
						t.Errorf("errMsg = %q, want empty", errMsg)
					}
					return nil
				}
			},
		},
		{
			name:   "valid_persist_ok_false_with_err_msg",
			id:     "r-2",
			ok:     false,
			errMsg: "connection refused",
			setup: func(r *mockUsecaseRepo) {
				r.updateRemoteAgentHealthFn = func(_ context.Context, id string, ok bool, errMsg string) error {
					if id != "r-2" {
						t.Errorf("id = %q, want %q", id, "r-2")
					}
					if ok {
						t.Errorf("ok = %v, want false", ok)
					}
					if errMsg != "connection refused" {
						t.Errorf("errMsg = %q, want %q", errMsg, "connection refused")
					}
					return nil
				}
			},
		},
		{
			name:     "empty_id_returns_bad_request",
			id:       "",
			ok:       true,
			errMsg:   "",
			wantErr:  true,
			wantCode: 400,
		},
		{
			name:     "whitespace_id_returns_bad_request",
			id:       "  ",
			ok:       true,
			errMsg:   "",
			wantErr:  true,
			wantCode: 400,
		},
		{
			name:   "repo_error",
			id:     "r-1",
			ok:     true,
			errMsg: "",
			setup: func(r *mockUsecaseRepo) {
				r.updateRemoteAgentHealthFn = func(_ context.Context, _ string, _ bool, _ string) error {
					return errors.New("db update failed")
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockUsecaseRepo{}
			if tt.setup != nil {
				tt.setup(repo)
			}
			uc := NewUsecase(repo, repo, repo, repo, nil)
			err := uc.PersistRemoteHealth(context.Background(), tt.id, tt.ok, tt.errMsg)
			if tt.wantErr {
				if err == nil {
					t.Fatal("PersistRemoteHealth() expected error, got nil")
				}
				if tt.wantCode != 0 {
					se := kerrors.FromError(err)
					if se == nil {
						t.Fatalf("expected kratos error, got %T", err)
					}
					if se.Code != tt.wantCode {
						t.Errorf("code = %d, want %d", se.Code, tt.wantCode)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("PersistRemoteHealth() unexpected error: %v", err)
			}
		})
	}
}

func TestAppendAudit(t *testing.T) {
	tests := []struct {
		name    string
		entry   AuditEntry
		setup   func(*mockUsecaseRepo)
		wantErr bool
		check   func(t *testing.T, entry AuditEntry)
	}{
		{
			name: "valid_append",
			entry: AuditEntry{
				InvokeID:      "inv-1",
				CallerAgentID: "agent-1",
				CalleeAgentID: "agent-2",
				Capability:    "chat",
				Status:        "success",
			},
			setup: func(r *mockUsecaseRepo) {
				r.insertAuditFn = func(_ context.Context, entry AuditEntry) error {
					if entry.InvokeID != "inv-1" {
						t.Errorf("InvokeID = %q, want %q", entry.InvokeID, "inv-1")
					}
					if entry.ID == "" {
						t.Error("ID should be auto-generated")
					}
					return nil
				}
			},
		},
		{
			name: "auto_generates_id_when_empty",
			entry: AuditEntry{
				InvokeID: "inv-2",
			},
			setup: func(r *mockUsecaseRepo) {
				r.insertAuditFn = func(_ context.Context, entry AuditEntry) error {
					if !strings.HasPrefix(entry.ID, "a2a-") {
						t.Errorf("auto-generated ID = %q, want prefix 'a2a-'", entry.ID)
					}
					return nil
				}
			},
		},
		{
			name: "preserves_existing_id",
			entry: AuditEntry{
				ID:       "custom-audit-id",
				InvokeID: "inv-3",
			},
			setup: func(r *mockUsecaseRepo) {
				r.insertAuditFn = func(_ context.Context, entry AuditEntry) error {
					if entry.ID != "custom-audit-id" {
						t.Errorf("ID = %q, want %q", entry.ID, "custom-audit-id")
					}
					return nil
				}
			},
		},
		{
			name: "repo_error",
			entry: AuditEntry{
				InvokeID: "inv-4",
			},
			setup: func(r *mockUsecaseRepo) {
				r.insertAuditFn = func(_ context.Context, _ AuditEntry) error {
					return errors.New("db insert failed")
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockUsecaseRepo{}
			if tt.setup != nil {
				tt.setup(repo)
			}
			uc := NewUsecase(repo, repo, repo, repo, nil)
			err := uc.AppendAudit(context.Background(), tt.entry)
			if tt.wantErr {
				if err == nil {
					t.Fatal("AppendAudit() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("AppendAudit() unexpected error: %v", err)
			}
		})
	}
}

func TestListAudit(t *testing.T) {
	tests := []struct {
		name     string
		callerID string
		calleeID string
		limit    int
		offset   int
		setup    func(*mockUsecaseRepo)
		wantErr  bool
		check    func(t *testing.T, entries []AuditEntry, total int)
	}{
		{
			name:     "returns_audit_entries",
			callerID: "agent-1",
			calleeID: "",
			limit:    10,
			offset:   0,
			setup: func(r *mockUsecaseRepo) {
				r.listAuditFn = func(_ context.Context, callerID, calleeID string, limit, offset int) ([]AuditEntry, int, error) {
					if callerID != "agent-1" {
						t.Errorf("callerID = %q, want %q", callerID, "agent-1")
					}
					if limit != 10 {
						t.Errorf("limit = %d, want 10", limit)
					}
					return []AuditEntry{
						{ID: "a-1", CallerAgentID: "agent-1", Status: "success"},
						{ID: "a-2", CallerAgentID: "agent-1", Status: "error"},
					}, 2, nil
				}
			},
			check: func(t *testing.T, entries []AuditEntry, total int) {
				if len(entries) != 2 {
					t.Fatalf("len(entries) = %d, want 2", len(entries))
				}
				if total != 2 {
					t.Errorf("total = %d, want 2", total)
				}
			},
		},
		{
			name:     "empty_result",
			callerID: "",
			calleeID: "",
			limit:    20,
			offset:   0,
			setup: func(r *mockUsecaseRepo) {
				r.listAuditFn = func(_ context.Context, _, _ string, _, _ int) ([]AuditEntry, int, error) {
					return nil, 0, nil
				}
			},
			check: func(t *testing.T, entries []AuditEntry, total int) {
				if len(entries) != 0 {
					t.Errorf("len(entries) = %d, want 0", len(entries))
				}
				if total != 0 {
					t.Errorf("total = %d, want 0", total)
				}
			},
		},
		{
			name:     "default_limit_when_zero",
			callerID: "",
			calleeID: "",
			limit:    0,
			offset:   0,
			setup: func(r *mockUsecaseRepo) {
				r.listAuditFn = func(_ context.Context, _, _ string, limit, _ int) ([]AuditEntry, int, error) {
					if limit != 50 {
						t.Errorf("limit = %d, want default 50", limit)
					}
					return nil, 0, nil
				}
			},
		},
		{
			name:     "default_limit_when_negative",
			callerID: "",
			calleeID: "",
			limit:    -1,
			offset:   0,
			setup: func(r *mockUsecaseRepo) {
				r.listAuditFn = func(_ context.Context, _, _ string, limit, _ int) ([]AuditEntry, int, error) {
					if limit != 50 {
						t.Errorf("limit = %d, want default 50", limit)
					}
					return nil, 0, nil
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockUsecaseRepo{}
			tt.setup(repo)
			uc := NewUsecase(repo, repo, repo, repo, nil)
			entries, total, err := uc.ListAudit(context.Background(), tt.callerID, tt.calleeID, tt.limit, tt.offset)
			if tt.wantErr {
				if err == nil {
					t.Fatal("ListAudit() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("ListAudit() unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, entries, total)
			}
		})
	}
}

func TestGetRemoteAgent(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		setup    func(*mockUsecaseRepo)
		wantErr  bool
		wantCode int32
		check    func(t *testing.T, agent RemoteAgent)
	}{
		{
			name: "returns_remote_agent",
			id:   "r-1",
			setup: func(r *mockUsecaseRepo) {
				r.getRemoteAgentFn = func(_ context.Context, id string) (RemoteAgent, error) {
					return RemoteAgent{ID: id, DisplayName: "Remote 1", Workspace: "ws-1", Enabled: true}, nil
				}
			},
			check: func(t *testing.T, agent RemoteAgent) {
				if agent.ID != "r-1" {
					t.Errorf("ID = %q, want %q", agent.ID, "r-1")
				}
				if agent.DisplayName != "Remote 1" {
					t.Errorf("DisplayName = %q, want %q", agent.DisplayName, "Remote 1")
				}
			},
		},
		{
			name:     "empty_id_returns_bad_request",
			id:       "",
			wantErr:  true,
			wantCode: 400,
		},
		{
			name:     "whitespace_id_returns_bad_request",
			id:       "   ",
			wantErr:  true,
			wantCode: 400,
		},
		{
			name: "not_found",
			id:   "r-nonexistent",
			setup: func(r *mockUsecaseRepo) {
				r.getRemoteAgentFn = func(_ context.Context, _ string) (RemoteAgent, error) {
					return RemoteAgent{}, errors.New("not found")
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockUsecaseRepo{}
			if tt.setup != nil {
				tt.setup(repo)
			}
			uc := NewUsecase(repo, repo, repo, repo, nil)
			agent, err := uc.GetRemoteAgent(context.Background(), tt.id)
			if tt.wantErr {
				if err == nil {
					t.Fatal("GetRemoteAgent() expected error, got nil")
				}
				if tt.wantCode != 0 {
					se := kerrors.FromError(err)
					if se == nil {
						t.Fatalf("expected kratos error, got %T", err)
					}
					if se.Code != tt.wantCode {
						t.Errorf("code = %d, want %d", se.Code, tt.wantCode)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("GetRemoteAgent() unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, agent)
			}
		})
	}
}

func TestFinishInvocation(t *testing.T) {
	tests := []struct {
		name    string
		inv     Invocation
		setup   func(*mockUsecaseRepo)
		wantErr bool
	}{
		{
			name: "valid_finish",
			inv: Invocation{
				ID:           "inv-1",
				CalleeAgentID: "agent-2",
				Capability:   "chat",
				Status:       "success",
				ResultJSON:   `{"reply":"hello"}`,
			},
			setup: func(r *mockUsecaseRepo) {
				r.updateInvocationFn = func(_ context.Context, inv Invocation) error {
					if inv.ID != "inv-1" {
						t.Errorf("ID = %q, want %q", inv.ID, "inv-1")
					}
					if inv.Status != "success" {
						t.Errorf("Status = %q, want %q", inv.Status, "success")
					}
					return nil
				}
			},
		},
		{
			name: "finish_with_error_status",
			inv: Invocation{
				ID:           "inv-2",
				CalleeAgentID: "agent-3",
				Status:       "error",
				ErrorMessage: "timeout exceeded",
			},
			setup: func(r *mockUsecaseRepo) {
				r.updateInvocationFn = func(_ context.Context, inv Invocation) error {
					if inv.Status != "error" {
						t.Errorf("Status = %q, want %q", inv.Status, "error")
					}
					if inv.ErrorMessage != "timeout exceeded" {
						t.Errorf("ErrorMessage = %q, want %q", inv.ErrorMessage, "timeout exceeded")
					}
					return nil
				}
			},
		},
		{
			name: "empty_id_delegates_to_repo",
			inv: Invocation{
				Status: "success",
			},
			setup: func(r *mockUsecaseRepo) {
				r.updateInvocationFn = func(_ context.Context, inv Invocation) error {
					return nil
				}
			},
		},
		{
			name: "repo_error",
			inv: Invocation{
				ID:     "inv-1",
				Status: "success",
			},
			setup: func(r *mockUsecaseRepo) {
				r.updateInvocationFn = func(_ context.Context, _ Invocation) error {
					return errors.New("db update failed")
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockUsecaseRepo{}
			if tt.setup != nil {
				tt.setup(repo)
			}
			uc := NewUsecase(repo, repo, repo, repo, nil)
			err := uc.FinishInvocation(context.Background(), tt.inv)
			if tt.wantErr {
				if err == nil {
					t.Fatal("FinishInvocation() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("FinishInvocation() unexpected error: %v", err)
			}
		})
	}
}
