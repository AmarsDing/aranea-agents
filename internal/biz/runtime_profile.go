package biz

import (
	"context"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"
)

// RuntimeProfileUsecase orchestrates runtime profile CRUD and resolution.
type RuntimeProfileUsecase struct {
	repo RuntimeProfileReadWriter
}

func NewRuntimeProfileUsecase(repo RuntimeProfileReadWriter) *RuntimeProfileUsecase {
	return &RuntimeProfileUsecase{repo: repo}
}

// List returns profiles for an agent. When activeOnly is true, only active
// profiles are returned (ordered by priority desc).
func (uc *RuntimeProfileUsecase) List(ctx context.Context, agentID string, activeOnly bool) ([]RuntimeProfile, error) {
	if strings.TrimSpace(agentID) == "" {
		return nil, apierror.BadRequest(apierror.DomainRuntimeProfile, "agent_id required")
	}
	return uc.repo.List(ctx, agentID, activeOnly)
}

// Get returns a single profile by ID.
func (uc *RuntimeProfileUsecase) Get(ctx context.Context, id string) (RuntimeProfile, error) {
	if strings.TrimSpace(id) == "" {
		return RuntimeProfile{}, apierror.BadRequest(apierror.DomainRuntimeProfile, "id required")
	}
	return uc.repo.GetByID(ctx, id)
}

// GetActive returns the highest-priority active profile for an agent.
// Returns nil (no error) when no active profile exists — callers treat
// nil as "use agent defaults".
func (uc *RuntimeProfileUsecase) GetActive(ctx context.Context, agentID string) (*RuntimeProfile, error) {
	if strings.TrimSpace(agentID) == "" {
		return nil, apierror.BadRequest(apierror.DomainRuntimeProfile, "agent_id required")
	}
	return uc.repo.GetActive(ctx, agentID)
}

// Create creates a new runtime profile. When IsActive is true, any previously
// active profile for the same agent is deactivated first (single-active invariant).
func (uc *RuntimeProfileUsecase) Create(ctx context.Context, p RuntimeProfile) (RuntimeProfile, error) {
	if err := validateProfile(p); err != nil {
		return RuntimeProfile{}, err
	}
	if p.ID == "" {
		p.ID = newAgentCatalogID()
	}
	if p.Version == "" {
		p.Version = "1"
	}
	if p.IsActive {
		if err := uc.deactivateActiveForAgent(ctx, p.AgentID); err != nil {
			return RuntimeProfile{}, err
		}
	}
	return uc.repo.Create(ctx, p)
}

// Update modifies an existing runtime profile.
func (uc *RuntimeProfileUsecase) Update(ctx context.Context, p RuntimeProfile) (RuntimeProfile, error) {
	if strings.TrimSpace(p.ID) == "" {
		return RuntimeProfile{}, apierror.BadRequest(apierror.DomainRuntimeProfile, "id required")
	}
	if err := validateProfile(p); err != nil {
		return RuntimeProfile{}, err
	}
	if p.IsActive {
		existing, err := uc.repo.GetByID(ctx, p.ID)
		if err != nil {
			return RuntimeProfile{}, err
		}
		if existing.AgentID != p.AgentID {
			return RuntimeProfile{}, apierror.BadRequest(apierror.DomainRuntimeProfile, "cannot change agent_id of existing profile")
		}
		if err := uc.deactivateActiveForAgent(ctx, p.AgentID); err != nil {
			return RuntimeProfile{}, err
		}
	}
	return uc.repo.Update(ctx, p)
}

// Delete removes a runtime profile by ID.
func (uc *RuntimeProfileUsecase) Delete(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return apierror.BadRequest(apierror.DomainRuntimeProfile, "id required")
	}
	return uc.repo.Delete(ctx, id)
}

// SetActive activates or deactivates a profile. When activating, any other
// active profile for the same agent is deactivated (single-active invariant).
// TECH-DEBT(debt): deactivateActiveForAgent + repo.SetActive are two separate
// DB operations without a transaction. Under concurrent SetActive calls this
// could briefly leave two profiles active. Business impact is low because
// GetActive returns the highest-priority one. Wrap in ExecInTx when a
// RuntimeProfileTxRepo is introduced.
func (uc *RuntimeProfileUsecase) SetActive(ctx context.Context, id string, active bool) (RuntimeProfile, error) {
	if strings.TrimSpace(id) == "" {
		return RuntimeProfile{}, apierror.BadRequest(apierror.DomainRuntimeProfile, "id required")
	}
	if active {
		p, err := uc.repo.GetByID(ctx, id)
		if err != nil {
			return RuntimeProfile{}, err
		}
		if err := uc.deactivateActiveForAgent(ctx, p.AgentID); err != nil {
			return RuntimeProfile{}, err
		}
	}
	return uc.repo.SetActive(ctx, id, active)
}

// deactivateActiveForAgent deactivates any currently active profile for the
// given agent. Safe to call when no active profile exists.
func (uc *RuntimeProfileUsecase) deactivateActiveForAgent(ctx context.Context, agentID string) error {
	current, err := uc.repo.GetActive(ctx, agentID)
	if err != nil {
		return err
	}
	if current == nil {
		return nil
	}
	_, err = uc.repo.SetActive(ctx, current.ID, false)
	return err
}

func validateProfile(p RuntimeProfile) error {
	if strings.TrimSpace(p.AgentID) == "" {
		return apierror.BadRequest(apierror.DomainRuntimeProfile, "agent_id required")
	}
	if strings.TrimSpace(p.Name) == "" {
		return apierror.BadRequest(apierror.DomainRuntimeProfile, "name required")
	}
	return nil
}

// ResolveForAgent returns the active runtime profile for the given agent,
// or nil when no profile is configured. This is the entry point used by
// the agent builder to apply profile overrides at run time.
func (uc *RuntimeProfileUsecase) ResolveForAgent(ctx context.Context, agentID string) (*RuntimeProfile, error) {
	if strings.TrimSpace(agentID) == "" {
		return nil, nil
	}
	prof, err := uc.repo.GetActive(ctx, agentID)
	if err != nil {
		return nil, err
	}
	return prof, nil
}

// Now returns the current UTC time. Exposed for test stubbing.
func (uc *RuntimeProfileUsecase) Now() time.Time { return time.Now().UTC() }
