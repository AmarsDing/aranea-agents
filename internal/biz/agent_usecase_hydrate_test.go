package biz

import (
	"context"
	stderrors "errors"
	"testing"

	"aranea-agents/internal/biz/shared"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// extrasCountingAgentRepo counts ListExtrasForAgents calls so tests can pin
// which hydrate path pays the extras query.
type extrasCountingAgentRepo struct {
	stubAgentRepo
	extrasCalls int
}

func (s *extrasCountingAgentRepo) ListExtrasForAgents(context.Context, []string) (map[string]AgentListExtras, error) {
	s.extrasCalls++
	return map[string]AgentListExtras{}, nil
}

// P3-1 (2026-10-19): GetForBuild hydrates settings + files (legacy migration
// included) but skips the display-only extras query; Get keeps it.
func TestAgentUsecase_GetForBuild_SkipsExtras(t *testing.T) {
	repo := &extrasCountingAgentRepo{stubAgentRepo: stubAgentRepo{agent: Agent{ID: "a1"}}}
	uc := NewAgentUsecase(AgentUsecaseDeps{Reader: repo, Writer: repo, Settings: repo, Files: repo, Position: repo, Tx: repo, Lg: loggateway.NewNoop()})

	ag, err := uc.GetForBuild(context.Background(), "a1")
	if err != nil {
		t.Fatalf("GetForBuild: %v", err)
	}
	if ag.Settings == nil {
		t.Fatal("settings must be hydrated on the build path")
	}
	if repo.extrasCalls != 0 {
		t.Fatalf("extras query must be skipped on build path, got %d calls", repo.extrasCalls)
	}

	if _, err := uc.Get(context.Background(), "a1"); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if repo.extrasCalls != 1 {
		t.Fatalf("full Get must query extras exactly once, got %d", repo.extrasCalls)
	}
}

func TestAgentUsecase_GetForBuild_RequiresID(t *testing.T) {
	repo := &extrasCountingAgentRepo{}
	uc := NewAgentUsecase(AgentUsecaseDeps{Reader: repo, Writer: repo, Settings: repo, Files: repo, Position: repo, Tx: repo, Lg: loggateway.NewNoop()})
	if _, err := uc.GetForBuild(context.Background(), "  "); err == nil {
		t.Fatal("blank id must be rejected")
	}
}

// admissionSettingsRepo controls the settings read for
// GetRuntimeSettingsForAdmission tests and counts upserts (migration writes).
type admissionSettingsRepo struct {
	stubAgentRepo
	settings     AgentRuntimeSettings
	settingsErr  error
	settingsCall int
	upserts      int
}

func (s *admissionSettingsRepo) GetAgentRuntimeSettings(context.Context, string) (AgentRuntimeSettings, error) {
	s.settingsCall++
	return s.settings, s.settingsErr
}

func (s *admissionSettingsRepo) UpsertAgentRuntimeSettings(_ context.Context, settings AgentRuntimeSettings) (AgentRuntimeSettings, error) {
	s.upserts++
	return settings, nil
}

// P3-1: a settings hit is returned RAW (no withSettingDefaults fill-in), so
// "unset (<=0) → caller default" semantics match the hydrated Agent.Settings
// read exactly (assembleHydrated also stores raw).
func TestAgentUsecase_GetRuntimeSettingsForAdmission_ReturnsRawSettings(t *testing.T) {
	repo := &admissionSettingsRepo{
		stubAgentRepo: stubAgentRepo{agent: Agent{ID: "a1"}},
		settings:      AgentRuntimeSettings{AgentID: "a1", L0SummaryThreshold: 0, L0RecentWindowTurns: 7},
	}
	uc := NewAgentUsecase(AgentUsecaseDeps{Reader: repo, Writer: repo, Settings: repo, Files: repo, Position: repo, Tx: repo, Lg: loggateway.NewNoop()})
	got, err := uc.GetRuntimeSettingsForAdmission(context.Background(), "a1")
	if err != nil {
		t.Fatalf("GetRuntimeSettingsForAdmission: %v", err)
	}
	if got.L0SummaryThreshold != 0 {
		t.Fatalf("raw semantics violated: L0SummaryThreshold must stay unset (0), got %v", got.L0SummaryThreshold)
	}
	if got.L0RecentWindowTurns != 7 {
		t.Fatalf("stored value not returned as-is: L0RecentWindowTurns=%d want 7", got.L0RecentWindowTurns)
	}
	if repo.settingsCall != 1 || repo.upserts != 0 {
		t.Fatalf("hot path must be a single read, no upsert: reads=%d upserts=%d", repo.settingsCall, repo.upserts)
	}
}

// Settings row missing + agent exists: lazy legacy config_json migration runs
// (settingsFor parity), persisting the migrated row.
func TestAgentUsecase_GetRuntimeSettingsForAdmission_MigratesLegacyWhenMissing(t *testing.T) {
	repo := &admissionSettingsRepo{
		stubAgentRepo: stubAgentRepo{agent: Agent{ID: "a1"}},
		settingsErr:   shared.ErrNotFound,
	}
	uc := NewAgentUsecase(AgentUsecaseDeps{Reader: repo, Writer: repo, Settings: repo, Files: repo, Position: repo, Tx: repo, Lg: loggateway.NewNoop()})
	got, err := uc.GetRuntimeSettingsForAdmission(context.Background(), "a1")
	if err != nil {
		t.Fatalf("GetRuntimeSettingsForAdmission: %v", err)
	}
	if got.AgentID != "a1" {
		t.Fatalf("migrated settings must carry the agent id, got %q", got.AgentID)
	}
	if got.L0SummaryThreshold != DefaultAgentRuntimeSettings().L0SummaryThreshold {
		t.Fatalf("empty legacy config migrates to defaults: L0SummaryThreshold=%v", got.L0SummaryThreshold)
	}
	if repo.upserts != 1 {
		t.Fatalf("migration must persist the row once, upserts=%d", repo.upserts)
	}
}

func TestAgentUsecase_GetRuntimeSettingsForAdmission_RequiresID(t *testing.T) {
	repo := &admissionSettingsRepo{}
	uc := NewAgentUsecase(AgentUsecaseDeps{Reader: repo, Writer: repo, Settings: repo, Files: repo, Position: repo, Tx: repo, Lg: loggateway.NewNoop()})
	_, err := uc.GetRuntimeSettingsForAdmission(context.Background(), "  ")
	if err == nil || !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("blank id must be BAD_REQUEST, got %v", err)
	}
}

// narrowPeekRepo implements the optional single-column readers on top of the
// legacy stub, counting hydrated-Get fallbacks so tests can pin which path ran.
type narrowPeekRepo struct {
	stubAgentRepo
	scope      string
	scopeErr   error
	key        string
	keyErr     error
	scopeCalls int
	keyCalls   int
	getCalls   int
}

func (s *narrowPeekRepo) GetAgentByID(ctx context.Context, id string) (Agent, error) {
	s.getCalls++
	return s.stubAgentRepo.GetAgentByID(ctx, id)
}

func (s *narrowPeekRepo) GetAgentWorkspaceScope(context.Context, string) (string, error) {
	s.scopeCalls++
	return s.scope, s.scopeErr
}

func (s *narrowPeekRepo) GetAgentKeyByID(context.Context, string) (string, error) {
	s.keyCalls++
	return s.key, s.keyErr
}

// Narrow single-column reader hit: one SELECT, never the full hydrated Get.
func TestAgentUsecase_PeekWorkspaceScope_NarrowHit(t *testing.T) {
	repo := &narrowPeekRepo{stubAgentRepo: stubAgentRepo{agent: Agent{ID: "a1", WorkspaceID: "ws-wide"}}, scope: "ws-x"}
	uc := NewAgentUsecase(AgentUsecaseDeps{Reader: repo, Writer: repo, Settings: repo, Files: repo, Position: repo, Tx: repo, Lg: loggateway.NewNoop()})
	got, err := uc.PeekWorkspaceScope(context.Background(), "a1")
	if err != nil || got != "ws-x" {
		t.Fatalf("scope=%q err=%v, want ws-x", got, err)
	}
	if repo.scopeCalls != 1 || repo.getCalls != 0 {
		t.Fatalf("narrow read only: scopeCalls=%d getCalls=%d", repo.scopeCalls, repo.getCalls)
	}
}

func TestAgentUsecase_PeekWorkspaceScope_NotFoundMapping(t *testing.T) {
	repo := &narrowPeekRepo{stubAgentRepo: stubAgentRepo{agent: Agent{ID: "a1"}}, scopeErr: shared.ErrNotFound}
	uc := NewAgentUsecase(AgentUsecaseDeps{Reader: repo, Writer: repo, Settings: repo, Files: repo, Position: repo, Tx: repo, Lg: loggateway.NewNoop()})
	if _, err := uc.PeekWorkspaceScope(context.Background(), "a1"); !stderrors.Is(err, shared.ErrNotFound) {
		t.Fatalf("repo ErrNotFound must map to biz ErrNotFound, got %v", err)
	}
}

// Repos without the narrow interface (legacy test fakes, custom repos) fall
// back to the full hydrated Get and read the field off it.
func TestAgentUsecase_PeekWorkspaceScope_LegacyFallback(t *testing.T) {
	repo := &stubAgentRepo{agent: Agent{ID: "a1", WorkspaceID: "ws-x"}}
	uc := NewAgentUsecase(AgentUsecaseDeps{Reader: repo, Writer: repo, Settings: repo, Files: repo, Position: repo, Tx: repo, Lg: loggateway.NewNoop()})
	got, err := uc.PeekWorkspaceScope(context.Background(), "a1")
	if err != nil || got != "ws-x" {
		t.Fatalf("scope=%q err=%v, want ws-x", got, err)
	}
}

func TestAgentUsecase_PeekAgentKey_NarrowHit(t *testing.T) {
	repo := &narrowPeekRepo{stubAgentRepo: stubAgentRepo{agent: Agent{ID: "a1", AgentKey: "wide-key"}}, key: "k-a"}
	uc := NewAgentUsecase(AgentUsecaseDeps{Reader: repo, Writer: repo, Settings: repo, Files: repo, Position: repo, Tx: repo, Lg: loggateway.NewNoop()})
	got, err := uc.PeekAgentKey(context.Background(), "a1")
	if err != nil || got != "k-a" {
		t.Fatalf("key=%q err=%v, want k-a", got, err)
	}
	if repo.keyCalls != 1 || repo.getCalls != 0 {
		t.Fatalf("narrow read only: keyCalls=%d getCalls=%d", repo.keyCalls, repo.getCalls)
	}
}

func TestAgentUsecase_PeekAgentKey_NotFoundMapping(t *testing.T) {
	repo := &narrowPeekRepo{stubAgentRepo: stubAgentRepo{agent: Agent{ID: "a1"}}, keyErr: shared.ErrNotFound}
	uc := NewAgentUsecase(AgentUsecaseDeps{Reader: repo, Writer: repo, Settings: repo, Files: repo, Position: repo, Tx: repo, Lg: loggateway.NewNoop()})
	if _, err := uc.PeekAgentKey(context.Background(), "a1"); !stderrors.Is(err, shared.ErrNotFound) {
		t.Fatalf("repo ErrNotFound must map to biz ErrNotFound, got %v", err)
	}
}

func TestAgentUsecase_PeekAgentKey_LegacyFallback(t *testing.T) {
	repo := &stubAgentRepo{agent: Agent{ID: "a1", AgentKey: "k-a"}}
	uc := NewAgentUsecase(AgentUsecaseDeps{Reader: repo, Writer: repo, Settings: repo, Files: repo, Position: repo, Tx: repo, Lg: loggateway.NewNoop()})
	got, err := uc.PeekAgentKey(context.Background(), "a1")
	if err != nil || got != "k-a" {
		t.Fatalf("key=%q err=%v, want k-a", got, err)
	}
}
