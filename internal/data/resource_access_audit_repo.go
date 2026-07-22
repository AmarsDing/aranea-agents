package data

import (
	"context"
	"time"

	"aranea-agents/internal/biz"

	"github.com/google/uuid"
)

// resourceAccessAuditRepo implements biz.AccessAuditor over Ent (M71).
// Append-only: entries are never updated or deleted.
type resourceAccessAuditRepo struct {
	data *Data
}

var _ biz.AccessAuditor = (*resourceAccessAuditRepo)(nil)

// NewResourceAccessAuditRepo creates the audit repo.
func NewResourceAccessAuditRepo(d *Data) biz.AccessAuditor {
	return &resourceAccessAuditRepo{data: d}
}

func (r *resourceAccessAuditRepo) Record(ctx context.Context, e biz.AuditEntry) error {
	_, err := r.data.RW().Write(ctx).ResourceAccessAudit.Create().
		SetID(uuid.NewString()).
		SetActorAgentID(e.ActorAgentID).
		SetActorRole(e.ActorRole).
		SetAction(e.Action).
		SetTargetAgentID(e.TargetAgentID).
		SetTargetDeptID(e.TargetDeptID).
		SetRelation(e.Relation).
		SetResourceURI(e.ResourceURI).
		SetResult(e.Result).
		SetDenyReason(e.DenyReason).
		SetCreatedAt(time.Now().UTC()).
		Save(ctx)
	if err != nil {
		return entErrToBizErr(err, "RESOURCE_ACCESS")
	}
	return nil
}
