package service

import (
	"context"
	"fmt"
	"strings"

	v1 "aranea-agents/api/kratos/agent/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// BatchUpdateAgents implements POST /v1/agents:batchUpdate (LIST-04 bulk enable/disable/delete).
func (s *AgentService) BatchUpdateAgents(ctx context.Context, req *v1.BatchUpdateAgentsRequest) (*v1.BatchUpdateAgentsResponse, error) {
	ids := req.GetIds()
	if len(ids) == 0 {
		return nil, apierror.BadRequest(apierror.DomainAgent, "ids is required")
	}
	status := strings.TrimSpace(req.GetStatus())
	del := req.GetDelete()
	if (status == "" && !del) || (status != "" && del) {
		return nil, apierror.BadRequest(apierror.DomainAgent, "exactly one of status or delete must be set")
	}
	// P2-B IDOR: per-agent mutate access check, unified with single-agent mutations.
	for _, id := range ids {
		if err := s.assertAgentMutateAccess(ctx, id); err != nil {
			return nil, err
		}
	}
	s.lg.Info("批量更新 Agent", loggateway.StepID("agent.crud.batch_update"),
		loggateway.Str("count", fmt.Sprintf("%d", len(ids))), loggateway.Str("status", status), loggateway.Str("delete", fmt.Sprintf("%t", del)))
	n, err := s.uc.BatchUpdateAgents(ctx, biz.AgentBatchUpdateInput{IDs: ids, Status: status, Delete: del})
	if err != nil {
		s.lg.Error("批量更新 Agent 失败", loggateway.StepID("agent.crud.batch_update"), loggateway.Err(err))
		s.logAgentFlow(ctx, "agent.crud.batch_update", "Agent 批量更新失败", err, event.P("count", fmt.Sprintf("%d", len(ids))))
		return nil, err
	}
	s.logAgentFlow(ctx, "agent.crud.batch_update", "Agent 批量更新完成", nil,
		event.P("affected", fmt.Sprintf("%d", n)))
	for _, id := range ids {
		invalidateAgentBuildCache(id)
	}
	verb := biz.AuditVerbUpdate
	if del {
		verb = biz.AuditVerbDelete
	}
	recordAudit(ctx, s.mon, biz.AdminAuditEntry{
		Action:   biz.AuditAction(verb, "agent"),
		Resource: "agent",
		Summary:  fmt.Sprintf("batch ids=%d affected=%d", len(ids), n),
	})
	return &v1.BatchUpdateAgentsResponse{Affected: int32(n)}, nil
}
