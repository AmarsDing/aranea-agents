package service

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// enforceQuota blocks when the scope monthly cap is exceeded (no-op if quota unset).
func enforceQuota(ctx context.Context, usage *biz.UsageUsecase, scopeType, scopeID string) error {
	if usage == nil {
		return nil
	}
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	if scopeType == "" || scopeID == "" {
		return nil
	}
	check, err := usage.CheckQuota(ctx, scopeType, scopeID)
	if err != nil {
		return err
	}
	if !check.Allowed {
		return kerrors.Forbidden("USAGE_QUOTA", check.Reason)
	}
	return nil
}

// enforceChatTurnQuotas checks agent, user, and global scopes before a chat turn.
func enforceChatTurnQuotas(ctx context.Context, usage *biz.UsageUsecase, agentID, userID string) error {
	if err := enforceQuota(ctx, usage, "agent", agentID); err != nil {
		return err
	}
	if err := enforceQuota(ctx, usage, "user", userID); err != nil {
		return err
	}
	return enforceQuota(ctx, usage, biz.QuotaScopeGlobal, biz.GlobalQuotaScopeID)
}

// checkTeamMemberQuotas rejects the turn when any enabled team member exceeds agent scope quota.
func (s *ChatService) checkTeamMemberQuotas(ctx context.Context, teamID string) error {
	return s.orch.checkTeamMemberQuotas(ctx, teamID)
}
