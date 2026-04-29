package service

import "arenea/backend/internal/catalog/application"

// AgentEvolutionService 在 Catalog 应用层实现；本包为类型/构造函数别名，兼容现有 import。
type (
	AgentEvolutionService             = application.AgentEvolutionService
	IdentityPatch                     = application.IdentityPatch
	StrategyPatch                     = application.StrategyPatch
	ProposalInput                     = application.ProposalInput
	ApplyInput                        = application.ApplyInput
	ScanReport                        = application.ScanReport
	ModelCandidate                    = application.ModelCandidate
	EvolutionEventListResult         = application.EvolutionEventListResult
	EvolutionProposalListResult      = application.EvolutionProposalListResult
	EvolutionMetricsReport           = application.EvolutionMetricsReport
	EvolutionSuggestion              = application.EvolutionSuggestion
	EvolutionTrainingExample         = application.EvolutionTrainingExample
)

// NewAgentEvolutionService 委托 internal/catalog/application。
var NewAgentEvolutionService = application.NewAgentEvolutionService