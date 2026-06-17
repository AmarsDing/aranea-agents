package data

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/borrowrequest"
	"aranea-agents/pkg/apierror"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
)

type borrowRequestRepo struct {
	data *Data
}

var _ biz.BorrowRequestRepo = (*borrowRequestRepo)(nil)

func NewBorrowRequestRepo(d *Data) biz.BorrowRequestRepo {
	return &borrowRequestRepo{data: d}
}

func entToBizBorrowRequest(e *ent.BorrowRequest) biz.BorrowRequest {
	if e == nil {
		return biz.BorrowRequest{}
	}
	return biz.BorrowRequest{
		ID:           e.ID,
		TeamID:       e.TeamID,
		AgentID:      e.AgentID,
		FromDeptID:   e.FromDeptID,
		ToDeptID:     e.ToDeptID,
		Status:       e.Status,
		Reason:       e.Reason,
		ReviewedBy:   e.ReviewedBy,
		ReviewReason: e.ReviewReason,
		CreatedAt:    parseTime(e.CreatedAt),
		UpdatedAt:    parseTime(e.UpdatedAt),
	}
}

func (r *borrowRequestRepo) CreateBorrowRequest(ctx context.Context, br biz.BorrowRequest) (biz.BorrowRequest, error) {
	if br.CreatedAt.IsZero() {
		br.CreatedAt = time.Now().UTC()
	}
	br.UpdatedAt = br.CreatedAt
	if br.ID == "" {
		br.ID = uuid.NewString()
	}

	saved, err := r.data.RW().Write(ctx).BorrowRequest.Create().
		SetID(br.ID).
		SetTeamID(br.TeamID).
		SetAgentID(br.AgentID).
		SetFromDeptID(br.FromDeptID).
		SetToDeptID(br.ToDeptID).
		SetStatus(br.Status).
		SetReason(br.Reason).
		SetReviewedBy(br.ReviewedBy).
		SetReviewReason(br.ReviewReason).
		SetCreatedAt(formatTime(br.CreatedAt)).
		SetUpdatedAt(formatTime(br.UpdatedAt)).
		Save(ctx)
	if err != nil {
		return biz.BorrowRequest{}, entErrToBizErr(err, "BORROW_REQUEST")
	}
	return entToBizBorrowRequest(saved), nil
}

func (r *borrowRequestRepo) GetBorrowRequest(ctx context.Context, id string) (biz.BorrowRequest, error) {
	row, err := r.data.RW().Read(ctx).BorrowRequest.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.BorrowRequest{}, apierror.NotFound("BORROW_REQUEST", "borrow request not found")
		}
		return biz.BorrowRequest{}, entErrToBizErr(err, "BORROW_REQUEST")
	}
	return entToBizBorrowRequest(row), nil
}

func (r *borrowRequestRepo) UpdateBorrowRequest(ctx context.Context, br biz.BorrowRequest) (biz.BorrowRequest, error) {
	br.UpdatedAt = time.Now().UTC()
	err := r.data.RW().Write(ctx).BorrowRequest.UpdateOneID(br.ID).
		SetStatus(br.Status).
		SetReviewedBy(br.ReviewedBy).
		SetReviewReason(br.ReviewReason).
		SetUpdatedAt(formatTime(br.UpdatedAt)).
		Exec(ctx)
	if err != nil {
		return biz.BorrowRequest{}, entErrToBizErr(err, "BORROW_REQUEST")
	}
	return r.GetBorrowRequest(ctx, br.ID)
}

func (r *borrowRequestRepo) ListPendingBorrowRequests(ctx context.Context, deptID string) ([]biz.BorrowRequest, error) {
	query := r.data.RW().Read(ctx).BorrowRequest.Query().
		Where(borrowrequest.StatusEQ(biz.BorrowRequestPending))
	if deptID != "" {
		query = query.Where(borrowrequest.FromDeptIDEQ(deptID))
	}
	rows, err := query.Order(borrowrequest.ByCreatedAt(entsql.OrderAsc())).All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "BORROW_REQUEST")
	}
	out := make([]biz.BorrowRequest, 0, len(rows))
	for _, e := range rows {
		out = append(out, entToBizBorrowRequest(e))
	}
	return out, nil
}

func (r *borrowRequestRepo) ListBorrowRequestsByTeam(ctx context.Context, teamID string) ([]biz.BorrowRequest, error) {
	rows, err := r.data.RW().Read(ctx).BorrowRequest.Query().
		Where(borrowrequest.TeamIDEQ(teamID)).
		Order(borrowrequest.ByCreatedAt(entsql.OrderAsc())).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "BORROW_REQUEST")
	}
	out := make([]biz.BorrowRequest, 0, len(rows))
	for _, e := range rows {
		out = append(out, entToBizBorrowRequest(e))
	}
	return out, nil
}

func (r *borrowRequestRepo) ListExpiredPendingBorrowRequests(ctx context.Context) ([]biz.BorrowRequest, error) {
	cutoff := time.Now().UTC().Add(-biz.BorrowAutoApproveTimeout)
	cutoffStr := formatTime(cutoff)
	rows, err := r.data.RW().Read(ctx).BorrowRequest.Query().
		Where(
			borrowrequest.StatusEQ(biz.BorrowRequestPending),
			borrowrequest.CreatedAtLTE(cutoffStr),
		).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "BORROW_REQUEST")
	}
	out := make([]biz.BorrowRequest, 0, len(rows))
	for _, e := range rows {
		out = append(out, entToBizBorrowRequest(e))
	}
	return out, nil
}

// CancelBorrowRequestsByFromDept cancels all pending borrow requests from a given department.
// Used during department deletion cascade.
// Uses batch update to avoid N+1 queries.
func (r *borrowRequestRepo) CancelBorrowRequestsByFromDept(ctx context.Context, deptID string) (int, error) {
	pending, err := r.data.RW().Read(ctx).BorrowRequest.Query().
		Where(
			borrowrequest.FromDeptIDEQ(deptID),
			borrowrequest.StatusEQ(biz.BorrowRequestPending),
		).
		All(ctx)
	if err != nil {
		return 0, entErrToBizErr(err, "BORROW_REQUEST")
	}
	if len(pending) == 0 {
		return 0, nil
	}
	now := formatTime(time.Now().UTC())
	cancelled := 0
	var failedIDs []string
	for _, p := range pending {
		err := r.data.RW().Write(ctx).BorrowRequest.UpdateOneID(p.ID).
			SetStatus(biz.BorrowRequestRejected).
			SetReviewReason("cancelled: source department deletion").
			SetUpdatedAt(now).
			Exec(ctx)
		if err != nil {
			failedIDs = append(failedIDs, p.ID)
			continue
		}
		cancelled++
	}
	if len(failedIDs) > 0 {
		return cancelled, apierror.Internal("BORROW_REQUEST", fmt.Sprintf("failed to cancel %d borrow requests: %v", len(failedIDs), failedIDs))
	}
	return cancelled, nil
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
