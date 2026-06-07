package biz

import (
	"context"
	"strings"

	"aranea-agents/internal/biz/monitor"
	"aranea-agents/internal/biz/session"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/google/wire"
)

type AgentExistenceCheckerFunc func(ctx context.Context, agentName string) bool

// AgentIDExistenceChecker checks whether an agent exists by its ID.
type AgentIDExistenceChecker interface {
	AgentExistsByID(ctx context.Context, agentID string) bool
}

var ProviderSet = wire.NewSet(
	NewEventBusConsumer,
	NewFlowLogUsecase,
	NewTurnMemoryWorker,
	NewAdminUsecase,
	NewAvatarUsecase,
	NewChannelIconRefresher,
	NewMemoryUsecase,
	NewTeamUsecase,
	NewHookUsecase,
	NewHookDeliveryUsecase,
	NewHookResolver,
	NewCronUsecase,
	NewPluginUsecase,
	NewScopeAgentLookup,
	NewSkillUsecase,
	NewSessionUsecase,
	NewSessionAgentLookup,
	NewSessionTeamLookup,
	NewSynthesisEngine,
	NewOrchestrationCache,
	NewToolSettingRepo,
	NewChannelUsecase,
	NewChannelTurnJobUsecase,
	NewSessionRunUsecase,
	NewAgentMCPTooling,
	NewEvolutionUsecase,
	NewTaskUsecase,
	NewArtifactUsecase,
	NewKnowledgeUsecaseFromRepo,
	NewEvalUsecase,
	NewA2AUsecase,
	ProvideA2AAgentLookup,
	NewEcosystemUsecase,
	NewEcosystemPresetUsecase,
	NewEventStoreUsecase,
	NewWebhookUsecase,
	NewWebhookDispatcher,
	NewAgentTemplateUsecase,
	NewLearningLoopUsecase,
	NewSkillEvolutionUsecase,
	NewOrganizationUsecase,
	ProvideDeptTeamLister,
	ProvideDeptAgentPositionClearer,
	ProvideGraphReaderForTeam,
	ProvideGraphWriterForTeam,
	NewToolResultGate,
	NewExperienceAnalyticsUsecase,
	NewSkillHealthUsecase,
	NewSkillDedupUsecase,
	NewDefaultKnowledgeProvider,
	NewDefaultPluginProvider,
	monitor.WireProviderSet,
	session.SessionMetricsProviderSet,
	session.SessionCompressionProviderSet,
	ProvideAgentExistenceChecker,
	ProvideAgentIDExistenceChecker,
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

// ProvideDeptTeamLister provides a DeptTeamLister from the TeamRepository.
func ProvideDeptTeamLister(repo TeamRepository) DeptTeamLister {
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

func requireNonEmpty(val, domain, field string) (string, error) {
	val = strings.TrimSpace(val)
	if val == "" {
		return "", kerrors.BadRequest(domain, field+" is required")
	}
	return val, nil
}
