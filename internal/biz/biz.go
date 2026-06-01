package biz

import (
	"context"
	"strings"

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
	NewMemoryUsecase,
	NewTeamUsecase,
	NewAgentCategoryUsecase,
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
	NewSpiritTeamUsecase,
	NewToolSettingRepo,
	NewChannelUsecase,
	NewChannelTurnJobUsecase,
	NewSessionRunUsecase,
	NewAgentMCPTooling,
	NewEvolutionUsecase,
	NewTaskUsecase,
	NewArtifactUsecase,
	NewKnowledgeUsecase,
	NewEvalUsecase,
	NewA2AUsecase,
	NewEcosystemUsecase,
	NewIndustryUsecase,
	NewDepartmentUsecase,
	NewPositionUsecase,
	NewEventStoreUsecase,
	NewWebhookUsecase,
	NewWebhookDispatcher,
	NewAgentTemplateUsecase,
	NewLearningLoopUsecase,
	NewToolResultGate,
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

func requireNonEmpty(val, domain, field string) (string, error) {
	val = strings.TrimSpace(val)
	if val == "" {
		return "", kerrors.BadRequest(domain, field+" is required")
	}
	return val, nil
}
