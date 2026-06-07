package biz

import (
	"context"
	"fmt"
	"sync"
	"time"

	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// EcosystemLoadedStatus tracks per-industry load state.
type EcosystemLoadedStatus map[string]IndustryLoadInfo

// IndustryLoadInfo holds the load state for a single industry.
type IndustryLoadInfo struct {
	Loaded        bool   `json:"loaded"`
	LoadedAt      string `json:"loaded_at,omitempty"`
	Agents        int    `json:"agents,omitempty"`
	Teams         int    `json:"teams,omitempty"`
	OrgNodes       int    `json:"org_nodes,omitempty"`
}

// LoadResult contains the result of loading an industry.
type LoadResult struct {
	AgentsCreated int `json:"agents_created"`
	TeamsCreated  int `json:"teams_created"`
	OrgNodes      int `json:"org_nodes"`
}

// UnloadResult contains the result of unloading an industry.
type UnloadResult struct {
	AgentsDeleted        int `json:"agents_deleted"`
	TeamsDeleted         int `json:"teams_deleted"`
	OrgNodesDeleted      int `json:"org_nodes_deleted"`
	TeamsModified        int `json:"teams_modified,omitempty"`
}

// EcosystemLoadResponse is the API response for load.
type EcosystemLoadResponse struct {
	Results       map[string]*LoadResult `json:"results"`
	AlreadyLoaded []string               `json:"already_loaded,omitempty"`
	Errors        map[string]string      `json:"errors,omitempty"`
}

// EcosystemUnloadResponse is the API response for unload.
type EcosystemUnloadResponse struct {
	Results map[string]*UnloadResult `json:"results"`
	Errors  map[string]string        `json:"errors,omitempty"`
}

// EcosystemPresetRepo reads/writes ecosystem_loaded status and performs unload deletions.
type EcosystemPresetRepo interface {
	GetEcosystemLoaded(ctx context.Context) (EcosystemLoadedStatus, error)
	SetEcosystemLoaded(ctx context.Context, status EcosystemLoadedStatus) error
	DeleteOrgNodesByCompany(ctx context.Context, companyKey string) (int, error)
	DeleteAgentsByIndustry(ctx context.Context, industryKey string) (int, error)
	DeleteTeamsByIndustry(ctx context.Context, industryKey string) (deleted int, modified int, err error)
}

// SeedPackFunc matches the signature of data.SeedPackIndustry.
// The client parameter is typed as any to avoid import cycles (actual type: *ent.Client).
type SeedPackFunc func(ctx context.Context, client any, scenarioDir string, industryKey string, kindOverride string, lg loggateway.Logger) (int, int, error)

// EcosystemPresetUsecase manages ecosystem preset load/unload operations.
type EcosystemPresetUsecase struct {
	repo        EcosystemPresetRepo
	seedPack    SeedPackFunc
	lg          loggateway.Logger
	scenarioDir string
	mu          sync.Mutex // protects load/unload from concurrent execution
}

// NewEcosystemPresetUsecase constructs an EcosystemPresetUsecase.
func NewEcosystemPresetUsecase(repo EcosystemPresetRepo, seedFn SeedPackFunc, scenarioDir string, lg loggateway.Logger) *EcosystemPresetUsecase {
	return &EcosystemPresetUsecase{repo: repo, seedPack: seedFn, scenarioDir: scenarioDir, lg: lg}
}

// DefaultIndustries lists the default industry keys for ecosystem presets.
var DefaultIndustries = []string{"finance", "selfmedia", "softwaredev"}

// LoadEcosystemPreset loads ecosystem preset data for the specified industries.
// The client parameter is the ent.Client (typed as any to avoid import cycles).
func (uc *EcosystemPresetUsecase) LoadEcosystemPreset(ctx context.Context, industries []string, force bool, client any) (*EcosystemLoadResponse, error) {
	uc.mu.Lock()
	defer uc.mu.Unlock()

	if len(industries) == 0 {
		industries = DefaultIndustries
	}

	status, err := uc.repo.GetEcosystemLoaded(ctx)
	if err != nil {
		return nil, kerrors.InternalServer("ECOSYSTEM", fmt.Sprintf("read ecosystem status: %s", err.Error()))
	}
	if status == nil {
		status = make(EcosystemLoadedStatus)
	}

	resp := &EcosystemLoadResponse{
		Results:       make(map[string]*LoadResult),
		AlreadyLoaded: []string{},
		Errors:        make(map[string]string),
	}

	for _, ind := range industries {
		info, exists := status[ind]
		if exists && info.Loaded && !force {
			resp.AlreadyLoaded = append(resp.AlreadyLoaded, ind)
			continue
		}

		agents, teams, err := uc.seedPack(ctx, client, uc.scenarioDir, ind, "ecosystem_preset", uc.lg)
		if err != nil {
			resp.Errors[ind] = err.Error()
			uc.lg.Error("failed to load industry preset",
				loggateway.StepID("ecosystem.load"),
				loggateway.Str("industry", ind),
				loggateway.Err(err))
			continue
		}

		status[ind] = IndustryLoadInfo{
			Loaded:   true,
			LoadedAt: time.Now().Format(time.RFC3339),
			Agents:   agents,
			Teams:    teams,
		}
		resp.Results[ind] = &LoadResult{AgentsCreated: agents, TeamsCreated: teams}
	}

	if err := uc.repo.SetEcosystemLoaded(ctx, status); err != nil {
		return nil, kerrors.InternalServer("ECOSYSTEM", fmt.Sprintf("save ecosystem status: %s", err.Error()))
	}

	return resp, nil
}

// UnloadEcosystemPreset unloads ecosystem preset data for the specified industries.
func (uc *EcosystemPresetUsecase) UnloadEcosystemPreset(ctx context.Context, industries []string) (*EcosystemUnloadResponse, error) {
	uc.mu.Lock()
	defer uc.mu.Unlock()

	if len(industries) == 0 {
		return nil, kerrors.BadRequest("ECOSYSTEM", "industries list is required")
	}

	status, err := uc.repo.GetEcosystemLoaded(ctx)
	if err != nil {
		return nil, kerrors.InternalServer("ECOSYSTEM", fmt.Sprintf("read ecosystem status: %s", err.Error()))
	}
	if status == nil {
		status = make(EcosystemLoadedStatus)
	}

	resp := &EcosystemUnloadResponse{
		Results: make(map[string]*UnloadResult),
		Errors:  make(map[string]string),
	}

	for _, ind := range industries {
		info, exists := status[ind]
		if !exists || !info.Loaded {
			resp.Errors[ind] = "industry not loaded"
			continue
		}

		var (
			taxDeleted    int
			agentsDeleted int
			teamsDeleted  int
			teamsModified int
			partialErr    string
		)

		taxDeleted, err = uc.repo.DeleteOrgNodesByCompany(ctx, ind)
		if err != nil {
			partialErr = fmt.Sprintf("delete taxonomy: %v", err)
			uc.lg.Warn("ecosystem unload: taxonomy deletion failed",
				loggateway.Str("industry", ind),
				loggateway.Err(err))
		}

		agentsDeleted, err = uc.repo.DeleteAgentsByIndustry(ctx, ind)
		if err != nil {
			if partialErr != "" {
				partialErr += "; "
			}
			partialErr += fmt.Sprintf("delete agents: %v", err)
			uc.lg.Warn("ecosystem unload: agents deletion failed",
				loggateway.Str("industry", ind),
				loggateway.Err(err))
		}

		teamsDeleted, teamsModified, err = uc.repo.DeleteTeamsByIndustry(ctx, ind)
		if err != nil {
			if partialErr != "" {
				partialErr += "; "
			}
			partialErr += fmt.Sprintf("delete teams: %v", err)
			uc.lg.Warn("ecosystem unload: teams deletion failed",
				loggateway.Str("industry", ind),
				loggateway.Err(err))
		}

		// Mark as unloaded regardless of partial failures, since data has been partially deleted
		status[ind] = IndustryLoadInfo{Loaded: false}
		resp.Results[ind] = &UnloadResult{
			AgentsDeleted:        agentsDeleted,
			TeamsDeleted:         teamsDeleted,
			OrgNodesDeleted: taxDeleted,
			TeamsModified:        teamsModified,
		}
		if partialErr != "" {
			resp.Errors[ind] = partialErr
		}
	}

	if err := uc.repo.SetEcosystemLoaded(ctx, status); err != nil {
		return nil, kerrors.InternalServer("ECOSYSTEM", fmt.Sprintf("save ecosystem status: %s", err.Error()))
	}

	return resp, nil
}

// GetEcosystemStatus returns the current ecosystem_loaded status.
func (uc *EcosystemPresetUsecase) GetEcosystemStatus(ctx context.Context) (EcosystemLoadedStatus, error) {
	return uc.repo.GetEcosystemLoaded(ctx)
}
