package biz

import (
	"context"
	"strings"
	"sync"

	"aranea-agents/pkg/loggateway"
)

// TeamDefinitionJSONProvider returns the current definition_json for a team.
// Used by GraphCacheManager to compute and verify definition hashes for cache invalidation.
type TeamDefinitionJSONProvider interface {
	GetTeamDefinitionJSON(ctx context.Context, teamID string) (string, error)
}

// GraphCacheManager manages the in-memory cache of compiled team build configs.
// It owns the teamBuildConfigs map and compiledTeamRepo, providing cache
// operations for graph execution and team graph coordination.
type GraphCacheManager struct {
	teamBuildConfigs map[string]*CompiledTeam
	compiledTeamRepo CompiledTeamRepo
	defProvider      GraphDefinitionProvider
	teamDefProvider  TeamDefinitionJSONProvider
	mu               sync.RWMutex
	lg               loggateway.Logger
}

// NewGraphCacheManager creates a cache manager for compiled team build configs.
func NewGraphCacheManager(compiledTeamRepo CompiledTeamRepo, defProvider GraphDefinitionProvider, teamDefProvider TeamDefinitionJSONProvider, lg loggateway.Logger) *GraphCacheManager {
	return &GraphCacheManager{
		teamBuildConfigs: make(map[string]*CompiledTeam),
		compiledTeamRepo: compiledTeamRepo,
		defProvider:      defProvider,
		teamDefProvider:  teamDefProvider,
		lg:               lg,
	}
}

// GetTeamBuildConfig returns the cached CompiledTeam for an execution ID.
func (cm *GraphCacheManager) GetTeamBuildConfig(execID string) (*CompiledTeam, bool) {
	if cm == nil {
		return nil, false
	}
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	ct, ok := cm.teamBuildConfigs[strings.TrimSpace(execID)]
	return ct, ok
}

// SetTeamBuildConfig stores a CompiledTeam in the cache for an execution ID.
func (cm *GraphCacheManager) SetTeamBuildConfig(execID string, ct *CompiledTeam) {
	if cm == nil {
		return
	}
	cm.mu.Lock()
	if cm.teamBuildConfigs == nil {
		cm.teamBuildConfigs = make(map[string]*CompiledTeam)
	}
	cm.teamBuildConfigs[execID] = ct
	cm.mu.Unlock()
}

// RemoveBuildConfig removes the cached CompiledTeam for an execution ID.
func (cm *GraphCacheManager) RemoveBuildConfig(execID string) {
	if cm == nil {
		return
	}
	cm.mu.Lock()
	delete(cm.teamBuildConfigs, execID)
	cm.mu.Unlock()
}

// BuildConfigForExecution resolves the CompiledTeam for a graph execution.
// It checks the in-memory cache first, then falls back to compiledTeamRepo
// (for team: prefixed graphIDs), and finally resolves from the graph definition.
// When loading from the repo, it verifies the definition hash; on mismatch the
// cached entry is treated as a miss and the definition is re-resolved.
func (cm *GraphCacheManager) BuildConfigForExecution(ctx context.Context, exec *GraphExecution) (*CompiledTeam, error) {
	if exec != nil {
		if ct, ok := cm.GetTeamBuildConfig(exec.ID); ok {
			return ct, nil
		}
	}
	if cm.compiledTeamRepo != nil && exec != nil && strings.HasPrefix(exec.GraphID, GraphIDTeamPrefix) {
		parts := strings.SplitN(exec.GraphID, ":", 2)
		if len(parts) == 2 {
			ct, err := cm.compiledTeamRepo.LoadForSession(ctx, parts[1], exec.GraphID, exec.SessionID)
			if err == nil && ct != nil {
				// Verify definition hash to detect stale cache entries.
				if cm.teamDefProvider != nil && ct.DefinitionHash != "" {
					currentDefJSON, defErr := cm.teamDefProvider.GetTeamDefinitionJSON(ctx, parts[1])
					if defErr == nil {
						currentHash := ComputeDefinitionHash(currentDefJSON)
						if currentHash != ct.DefinitionHash {
							cm.lg.Warn("compiled team cache miss: definition hash mismatch, re-compiling",
								loggateway.StepID("graph.build_config"),
							)
							// Stale cache — fall through to re-resolve from definition.
							goto resolveFromDef
						}
					}
				}
				cm.SetTeamBuildConfig(exec.ID, ct)
				return ct, nil
			}
		}
	}
resolveFromDef:
	def, err := cm.defProvider.GetGraph(ctx, exec.GraphID)
	if err != nil {
		return nil, err
	}
	cfg := FinalizeGraphFailurePolicy(defToBuildConfig(def), nil, nil)
	return NewCompiledTeam(cfg, nil, nil, nil), nil
}

// SaveCompiledTeam persists a compiled team via compiledTeamRepo.
// It computes and stores the definition hash for cache invalidation.
func (cm *GraphCacheManager) SaveCompiledTeam(ctx context.Context, teamID, graphID, sessionID string, ct *CompiledTeam) {
	if cm == nil || cm.compiledTeamRepo == nil {
		return
	}
	// Compute definition hash for cache invalidation.
	if cm.teamDefProvider != nil && ct.DefinitionHash == "" {
		if defJSON, err := cm.teamDefProvider.GetTeamDefinitionJSON(ctx, teamID); err == nil {
			ct.DefinitionHash = ComputeDefinitionHash(defJSON)
		}
	}
	if err := cm.compiledTeamRepo.Save(ctx, teamID, graphID, strings.TrimSpace(sessionID), ct); err != nil {
		cm.lg.Warn("persist compiled team failed", loggateway.StepID("graph.register_team"), loggateway.Err(err))
	}
}
