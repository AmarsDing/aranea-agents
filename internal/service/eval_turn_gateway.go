package service

import (
	"context"
)

// EvalTurnGateway runs evaluation agent turns without coupling evaluation to ChatService wiring.
type EvalTurnGateway interface {
	RunEvalAgentTurn(ctx context.Context, agentID, input string) (reply string, err error)
}

// RunEvalAgentTurn implements EvalTurnGateway using native trpc-agent-go turn execution.
func (s *ChatService) RunEvalAgentTurn(ctx context.Context, agentID, input string) (string, error) {
	return s.orch.RunEvalAgentTurn(ctx, agentID, input)
}
