package biz

import (
	"context"
	"strings"

	"aranea-agents/internal/biz/monitor"
	"aranea-agents/internal/biz/monitor/heal"
	"aranea-agents/internal/biz/session"

	"aranea-agents/pkg/apierror"

	"github.com/google/wire"
)

type AgentExistenceCheckerFunc func(ctx context.Context, agentName string) bool

// AgentIDExistenceChecker checks whether an agent exists by its ID.
type AgentIDExistenceChecker interface {
	AgentExistsByID(ctx context.Context, agentID string) bool
	AgentIsActiveByID(ctx context.Context, agentID string) bool
}

var ProviderSet = wire.NewSet(
	NewFlowLogUsecase,
	NewTurnMemoryWorker,
	NewAdminUsecase,
	NewAvatarUsecase,
	NewChannelIconRefresher,
	NewMemoryUsecase,
	// NOTE(B6/B7): NewTeamUsecase is wrapped by cmd/admin/wire.go's
	// provideTeamUsecase, which also injects the TeamGraphGuard into
	// GraphDefinitionUsecase — an imperative cross-usecase step wire cannot
	// express via the bare constructor here, so it is excluded from this set.
	NewHookUsecase,
	NewHookDeliveryUsecase,
	NewHookResolver,
	NewCronUsecase,
	NewPluginUsecase,
	NewScopeAgentLookup,
	NewSessionUsecase,
	NewSessionActivityLister,
	NewSessionAgentLookup,
	NewSessionTeamLookup,
	NewSynthesisEngine,
	NewOrchestrationCache,
	NewToolSettingRepo,
	ProvideChannelUsecase,
	NewChannelPeerUsecase,
	NewChannelTurnJobUsecase,
	NewSessionRunUsecase,
	NewAgentMCPTooling,
	// NOTE(A1): SkillEvolutionOrchestrator + triggers, and the evolution
	// usecases that need SetOrchestrator (EvolutionUsecase / LearningLoopUsecase),
	// are assembled in cmd/admin/wire.go — trigger
	// registration and SetOrchestrator are imperative steps that wire cannot
	// express via bare constructors here. ProvideEvolutionUsecase is wrapped by
	// wire.go's provideEvolutionUsecase, so it is excluded from this set.
	NewTaskUsecase,
	NewArtifactUsecase,
	// P3：自动装配双轨关联 + 资源管理器可选能力（接口断言，未实现则降级）。
	ProvideKnowledgeUsecase,
	NewEvalUsecase,
	NewA2AUsecase,
	ProvideA2AAgentLookup,
	NewEcosystemUsecase,
	NewEcosystemPresetUsecase,
	NewWebhookUsecase,
	NewWebhookDispatcher,
	NewAgentTemplateUsecase,
	NewRuntimeProfileUsecase,
	NewOrganizationUsecase,
	NewPositionPromptUsecase,
	ProvideDeptTeamLister,
	ProvideDeptAgentPositionClearer,
	ProvideGraphReaderForTeam,
	ProvideGraphWriterForTeam,
	ProvideTeamGraphAssetStore,
	ProvideTeamLinkedGraphReader,
	NewToolResultGate,
	NewExperienceAnalyticsUsecase,
	NewSkillHealthUsecase,
	NewSkillScoringUsecase,
	NewSkillReportUsecase,
	NewSkillDedupUsecase,
	NewSkillSimilarityEngine,
	NewRuleBasedContentFuser,
	ProvideSkillEmbedder,
	ProvideDefaultDedupWeights,
	NewDefaultKnowledgeProvider,
	NewDefaultPluginProvider,
	monitor.WireProviderSet,
	heal.WireProviderSet,
	session.SessionMetricsProviderSet,
	session.SessionCompressionProviderSet,
	ProvideAgentExistenceChecker,
	ProvideAgentIDExistenceChecker,
	NewMemoryWorkerStats,
	NewMemoryCanaryStatus,
)

func ProvideAgentRoleChecker(repo AgentRepository) AgentRoleChecker {
	return func(ctx context.Context, agentKey string, role string) bool {
		agent, err := repo.GetAgentByAgentKey(ctx, agentKey)
		if err != nil {
			return false
		}
		for _, r := range agent.Roles {
			if r == role {
				return true
			}
		}
		return false
	}
}

func ProvideAgentListerByRole(repo AgentRepository) AgentListerByRole {
	return func(ctx context.Context, role string) ([]string, error) {
		result, err := repo.SearchAgents(ctx, AgentListQuery{Role: role, Limit: 100})
		if err != nil {
			return nil, err
		}
		matched := make([]string, 0, len(result.Items))
		for _, a := range result.Items {
			matched = append(matched, a.AgentKey)
		}
		return matched, nil
	}
}

// ProvideDeptTeamLister provides a DeptTeamLister from the TeamReader.
func ProvideDeptTeamLister(repo TeamReader) DeptTeamLister {
	return repo.(DeptTeamLister)
}

// ProvideDeptAgentPositionClearer provides a DeptAgentPositionClearer from the AgentRepository.
func ProvideDeptAgentPositionClearer(repo AgentRepository) DeptAgentPositionClearer {
	return repo.(DeptAgentPositionClearer)
}

// ProvideGraphReaderForTeam provides a GraphReader from GraphUsecase for TeamUsecase.
func ProvideGraphReaderForTeam(uc *GraphUsecase) GraphReader {
	return uc.DefUC().Reader()
}

// ProvideGraphWriterForTeam provides a GraphWriter from GraphUsecase for TeamUsecase.
func ProvideGraphWriterForTeam(uc *GraphUsecase) GraphWriter {
	return uc.DefUC().Writer()
}

// ProvideTeamLinkedGraphReader exposes the team repo's linked-graph query as
// the narrow TeamLinkedGraphReader port（B7 删除保护）。绑定的 TeamReader 未
// 实现时返回 nil（单测/离线工具降级：跳过 external 引用检查）。
func ProvideTeamLinkedGraphReader(repo TeamReader) TeamLinkedGraphReader {
	r, _ := repo.(TeamLinkedGraphReader)
	return r
}

func ProvideAgentExistenceChecker(repo AgentRepository) AgentExistenceCheckerFunc {
	return func(ctx context.Context, agentName string) bool {
		_, err := repo.GetAgentByAgentKey(ctx, agentName)
		return err == nil
	}
}

// agentIDExistenceChecker adapts AgentRepository to AgentIDExistenceChecker.
type agentIDExistenceChecker struct {
	repo AgentRepository
}

func (c *agentIDExistenceChecker) AgentExistsByID(ctx context.Context, agentID string) bool {
	_, err := c.repo.GetAgentByID(ctx, agentID)
	return err == nil
}

func (c *agentIDExistenceChecker) AgentIsActiveByID(ctx context.Context, agentID string) bool {
	agent, err := c.repo.GetAgentByID(ctx, agentID)
	if err != nil {
		return false
	}
	return IsAgentStateActive(ParseAgentState(agent.Status))
}

// ProvideAgentIDExistenceChecker creates an AgentIDExistenceChecker from AgentRepository.
func ProvideAgentIDExistenceChecker(repo AgentRepository) AgentIDExistenceChecker {
	return &agentIDExistenceChecker{repo: repo}
}

// ProvideA2AAgentLookup creates an A2AAgentLookup from AgentRepository.
func ProvideA2AAgentLookup(repo AgentRepository) A2AAgentLookup {
	return NewAgentLookupAdapter(func(ctx context.Context, id string) (string, string, error) {
		ag, err := repo.GetAgentByID(ctx, id)
		if err != nil {
			return "", "", err
		}
		ws := ""
		if ag.Settings != nil {
			ws = ag.Settings.Workspace
		}
		if ws == "" {
			ws = ag.DisplayName
		}
		return ag.DisplayName, ws, nil
	})
}

// ProvideSkillEmbedder returns nil embedder (embedding is optional).
// When an embedding provider is available, replace this provider.
func ProvideSkillEmbedder() DedupEmbedder { return nil }

// ProvideDefaultDedupWeights returns default dedup similarity weights.
func ProvideDefaultDedupWeights() SimilarityWeights { return DefaultDedupWeights() }

func requireNonEmpty(val, domain, field string) (string, error) {
	val = strings.TrimSpace(val)
	if val == "" {
		return "", apierror.BadRequest(domain, field+" is required")
	}
	return val, nil
}
