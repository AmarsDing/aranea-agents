package biz

import (
	"context"
	"strings"
	"sync"

	"aranea-agents/pkg/apierror"
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
// It checks the in-memory cache first, then falls back to compiledTeamRepo,
// and finally resolves from the graph definition. When loading from the repo,
// it verifies the definition hash; on mismatch the cached entry is treated as
// a miss and the definition is re-resolved (F1: the stale branch must actually
// fall through — previously the cached entry was returned regardless).
//
// Repo row keys are (teamID, graphID) pairs; the teamID must be derived on
// resume because GraphExecution does not persist one:
//   - legacy synthetic ids "team:<teamID>[:<teamRunID>]" carry it inline (F2:
//     parsing the composite id must not treat the run id as part of the team id);
//   - linked asset ids (C1+) carry no team — derive it from the owned
//     definition's TeamID (F3). Externally-linked graphs (TeamID empty) skip
//     the repo and resolve from the definition; their CompiledTeam metadata
//     (TaskMeta/RoleManifest) is not recoverable after a restart.
func (cm *GraphCacheManager) BuildConfigForExecution(ctx context.Context, exec *GraphExecution) (*CompiledTeam, error) {
	if exec == nil {
		return nil, apierror.BadRequest("GRAPH", "graph execution is nil")
	}
	if ct, ok := cm.GetTeamBuildConfig(exec.ID); ok {
		return ct, nil
	}
	teamID := compiledTeamIDFromGraphID(exec.GraphID)
	var def *GraphDefinition
	if cm.compiledTeamRepo != nil && teamID == "" && cm.defProvider != nil {
		if d, derr := cm.defProvider.GetGraph(ctx, exec.GraphID); derr == nil && d != nil {
			def = d
			teamID = strings.TrimSpace(d.TeamID)
		}
	}
	if cm.compiledTeamRepo != nil && teamID != "" {
		ct, err := cm.compiledTeamRepo.LoadForSession(ctx, teamID, exec.GraphID, exec.SessionID)
		if err == nil && ct != nil {
			stale := false
			if cm.teamDefProvider != nil && ct.DefinitionHash != "" {
				currentDefJSON, defErr := cm.teamDefProvider.GetTeamDefinitionJSON(ctx, teamID)
				if defErr == nil {
					currentHash := ComputeDefinitionHash(currentDefJSON)
					if currentHash != ct.DefinitionHash {
						cm.lg.Warn("compiled team cache miss: definition hash mismatch, re-compiling",
							loggateway.StepID("graph.build_config"),
						)
						// Stale cache — fall through to re-resolve from definition.
						stale = true
					}
				}
				// defErr != nil: hash verification unavailable (e.g. team def not
				// found); degrade to using the cached entry as-is.
			}
			if !stale {
				cm.SetTeamBuildConfig(exec.ID, ct)
				return ct, nil
			}
		}
	}
	// Final fallback: resolve from graph definition.
	if def == nil {
		var err error
		def, err = cm.defProvider.GetGraph(ctx, exec.GraphID)
		if err != nil {
			if teamID != "" {
				// 终审-1：team 来源的执行（legacy 合成 id，或 F3 owned-def 派生）在
				// 定义不可解析时（团队被删/被改）不得泄露底层 NotFound——否则
				// Resume/TimeTravel 会误导为「图不存在」。转为确定性 Conflict，
				// 与 Y4 恢复一致性守卫同语义：要求新开执行而不是沿旧图恢复。
				return nil, apierror.Conflict("GRAPH",
					"team definition changed and the execution graph is not resolvable; start a new execution instead of resuming")
			}
			return nil, err
		}
	}
	cfg := FinalizeGraphFailurePolicy(defToBuildConfig(def), nil, nil)
	resolved := NewCompiledTeam(cfg, nil, nil, nil)
	// 终审-7：重解析结果入缓存。否则同一次执行每次 Resume/TimeTravel 都会重复走
	// repo-load + hash 告警 + rebuild 全链路（告警刷屏、重复编译浪费 CPU）。
	// 注意缓存键是执行 id，与 repo 命中分支（上文）一致；入缓存后 DefinitionHash
	// 为空，但因首轮 GetTeamBuildConfig 即命中，不会再走 repo 校验路径。
	cm.SetTeamBuildConfig(exec.ID, resolved)
	return resolved, nil
}

// compiledTeamIDFromGraphID extracts the team id from the legacy synthetic
// execution graph id "team:<teamID>[:<teamRunID>]" (F2: SplitN(":")[1] would
// yield "<teamID>:<teamRunID>" for composite ids, which never matches a stored
// row key). Plain asset ids yield "" — the caller derives the team from the
// owned definition instead (F3).
func compiledTeamIDFromGraphID(graphID string) string {
	if !strings.HasPrefix(graphID, GraphIDTeamPrefix) {
		return ""
	}
	rest := strings.TrimPrefix(graphID, GraphIDTeamPrefix)
	if teamID, _, found := strings.Cut(rest, ":"); found {
		return teamID
	}
	return rest
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
