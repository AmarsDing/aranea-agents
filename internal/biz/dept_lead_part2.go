package biz

import (
	"context"
)

// agentDepartment returns the department ID for an agent by looking up its
// position's parent department in the org tree.
func (m *DeptLeadManager) agentDepartment(ctx context.Context, agentID string) (string, error) {
	a, err := m.agentRepo.GetAgentByID(ctx, agentID)
	if err != nil {
		return "", err
	}
	if a.PositionID == "" {
		return "", nil
	}
	pos, err := m.orgRepo.GetOrgNode(ctx, a.PositionID)
	if err != nil {
		return "", err
	}
	if pos.Level == "department" {
		return pos.ID, nil
	}
	if pos.ParentID != "" {
		parent, err := m.orgRepo.GetOrgNode(ctx, pos.ParentID)
		if err != nil {
			return "", err
		}
		if parent.Level == "department" {
			return parent.ID, nil
		}
	}
	return "", nil
}

// publishBorrowEvent publishes a borrow request event as a v2 SystemNoticeEvent
// (replaces the legacy system-domain ActivityEvent; NOT persisted, WS-only broadcast).
// Uses context.Background() intentionally: event publishing is fire-and-forget
// and should not be cancelled when the originating request context expires.
func (m *DeptLeadManager) publishBorrowEvent(eventType, content string, r BorrowRequest) {
	if m.eventBus == nil {
		return
	}
	meta := map[string]any{
		"event_type":        eventType,
		"borrow_request_id": r.ID,
		"agent_id":          r.AgentID,
		"from_dept_id":      r.FromDeptID,
		"to_dept_id":        r.ToDeptID,
		"team_id":           r.TeamID,
		"status":            r.Status,
		"reviewed_by":       r.ReviewedBy,
		"review_reason":     r.ReviewReason,
	}
	m.eventBus.Publish(context.Background(), NewSystemNoticeEvent("", eventType, content, meta))
}

// BorrowedMemberStatus represents the read-only status of a borrowed member.
type BorrowedMemberStatus struct {
	AgentID        string `json:"agent_id"`
	AgentName      string `json:"agent_name"`
	TargetTeamID   string `json:"target_team_id"`
	TargetTeamName string `json:"target_team_name"`
	TeamStatus     string `json:"team_status"`
	BorrowStatus   string `json:"borrow_status"`
}

// GetBorrowedMemberStatus returns the status of agents borrowed from the given department.
// This provides a read-only view for department leads to monitor their borrowed members.
func (m *DeptLeadManager) GetBorrowedMemberStatus(ctx context.Context, deptID string) ([]BorrowedMemberStatus, error) {
	requests, err := m.borrowRepo.ListPendingBorrowRequests(ctx, deptID)
	if err != nil {
		return nil, err
	}
	var statuses []BorrowedMemberStatus
	for _, r := range requests {
		agent, aErr := m.agentRepo.GetAgentByID(ctx, r.AgentID)
		agentName := r.AgentID
		if aErr == nil {
			agentName = agent.DisplayName
		}
		status := BorrowedMemberStatus{
			AgentID:      r.AgentID,
			AgentName:    agentName,
			TargetTeamID: r.TeamID,
			BorrowStatus: r.Status,
		}
		if m.teamGetter != nil {
			if team, tErr := m.teamGetter.GetTeamByID(ctx, r.TeamID); tErr == nil {
				status.TargetTeamName = team.DisplayName
				status.TeamStatus = string(team.Status)
			}
		}
		statuses = append(statuses, status)
	}
	return statuses, nil
}
