package biz

import (
	"context"

	"github.com/google/wire"
)

type AgentExistenceCheckerFunc func(agentName string) bool

var ProviderSet = wire.NewSet(
	NewEventBusConsumer,
	NewEventBusSideConsumers,
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
	NewEventStoreUsecase,
	NewWebhookUsecase,
	NewWebhookDispatcher,
	ProvideAgentExistenceChecker,
)

func ProvideAgentRoleChecker(repo AgentRepository) AgentRoleChecker {
	return func(agentKey string, role string) bool {
		agent, err := repo.GetAgentByAgentKey(context.Background(), agentKey)
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
	return func(role string) ([]string, error) {
		result, err := repo.SearchAgents(context.Background(), AgentListQuery{Limit: 1000})
		if err != nil {
			return nil, err
		}
		var matched []string
		for _, a := range result.Items {
			for _, r := range a.Roles {
				if r == role {
					matched = append(matched, a.AgentKey)
					break
				}
			}
		}
		return matched, nil
	}
}

func ProvideAgentExistenceChecker(repo AgentRepository) AgentExistenceCheckerFunc {
	return func(agentName string) bool {
		_, err := repo.GetAgentByAgentKey(context.Background(), agentName)
		return err == nil
	}
}
