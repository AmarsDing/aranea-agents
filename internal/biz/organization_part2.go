package biz

import (
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"context"
	"path/filepath"
	"strings"
)

// cascadeDeleteDeptLead deletes the department lead agent.
func (u *OrganizationUsecase) cascadeDeleteDeptLead(ctx context.Context, dept OrganizationNode) {
	if u.deptLeadMgr == nil {
		return
	}
	if dlErr := u.deptLeadMgr.DeleteDeptLead(ctx, dept.ID); dlErr != nil {
		u.lg.Warn("failed to delete dept lead during department deletion",
			loggateway.Str("dept_id", dept.ID),
			loggateway.Err(dlErr))
	}
}

// cascadeDeleteChildPositions deletes child position nodes.
func (u *OrganizationUsecase) cascadeDeleteChildPositions(ctx context.Context, dept OrganizationNode) {
	positions, posErr := u.repo.ListOrgNodesByParentID(ctx, dept.ID)
	if posErr != nil {
		u.lg.Warn("failed to list positions during department deletion",
			loggateway.Str("dept_id", dept.ID),
			loggateway.Err(posErr))
		return
	}
	for _, pos := range positions {
		if delErr := u.repo.DeleteOrgNode(ctx, pos.ID); delErr != nil {
			u.lg.Warn("failed to delete position during department deletion",
				loggateway.Str("position_id", pos.ID),
				loggateway.Err(delErr))
		}
	}
}

func (u *OrganizationUsecase) ListByLevel(ctx context.Context, level string) ([]OrganizationNode, error) {
	if strings.TrimSpace(level) == "" {
		return nil, ErrOrgBadRequest("level is required")
	}
	return u.repo.ListOrgNodesByLevel(ctx, level)
}

func (u *OrganizationUsecase) ListByParentID(ctx context.Context, parentID string) ([]OrganizationNode, error) {
	if strings.TrimSpace(parentID) == "" {
		return nil, ErrOrgBadRequest("parent_id is required")
	}
	return u.repo.ListOrgNodesByParentID(ctx, parentID)
}

func (u *OrganizationUsecase) GetByKey(ctx context.Context, key string) (OrganizationNode, error) {
	if strings.TrimSpace(key) == "" {
		return OrganizationNode{}, ErrOrgBadRequest("key is required")
	}
	return u.repo.GetOrgNodeByKey(ctx, key)
}

func (u *OrganizationUsecase) Reorder(ctx context.Context, ids []string) error {
	return u.repo.ReorderOrgNodes(ctx, ids)
}

func (u *OrganizationUsecase) GetAncestors(ctx context.Context, positionID string) (OrgAncestors, error) {
	return u.posPrompt.GetAncestors(ctx, positionID)
}

func (u *OrganizationUsecase) GetPositionPrompt(ctx context.Context, companyKey, positionKey, variant string) (PositionPromptResult, error) {
	return u.posPrompt.GetPositionPrompt(ctx, companyKey, positionKey, variant)
}

func (u *OrganizationUsecase) ListPositionVariants(ctx context.Context, companyKey, positionKey string) ([]VariantInfo, error) {
	return u.posPrompt.ListPositionVariants(ctx, companyKey, positionKey)
}

func (u *OrganizationUsecase) normalizeOrg(ctx context.Context, in *OrganizationNode) error {
	return u.posPrompt.normalizeOrg(ctx, in)
}

func (u *OrganizationUsecase) BuildResponsibility(ctx context.Context, positionID string, mode string) (string, error) {
	return u.posPrompt.BuildResponsibility(ctx, positionID, mode)
}

var scenarioDirFunc = func() string {
	return filepath.Join("internal", "scenario")
}

func ScenarioDir() string {
	return scenarioDirFunc()
}

func ErrOrgBadRequest(msg string) error {
	return apierror.BadRequest("ORG", msg)
}

// publishOrgEvent publishes an organization CRUD event as a v2 SystemNoticeEvent
// (replaces the legacy system-domain ActivityEvent; NOT persisted, WS-only broadcast).
// Uses context.Background() intentionally: event publishing is fire-and-forget
// and should not be cancelled when the originating request context expires.
func (u *OrganizationUsecase) publishOrgEvent(eventType, content string, node OrganizationNode) {
	if u.eventBus == nil {
		return
	}
	meta := map[string]any{
		"event_type": eventType,
		"org_id":     node.ID,
		"org_key":    node.Key,
		"org_name":   node.Name,
		"level":      node.Level,
		"status":     node.Status,
	}
	u.eventBus.Publish(context.Background(), NewSystemNoticeEvent("", eventType, content, meta))
}
