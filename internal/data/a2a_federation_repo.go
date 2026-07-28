package data

import (
	"context"
	"time"

	biza2a "aranea-agents/internal/biz/a2a"
	dataent "aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/federationauditlog"
	"aranea-agents/internal/data/ent/federationorg"
	"aranea-agents/internal/data/ent/federationpolicy"
	"aranea-agents/internal/data/ent/predicate"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// A2AFederationRepo implements the federation narrow repos via Ent.
// Exported so Wire can bind each narrow interface to this concrete type.
type A2AFederationRepo struct {
	data *Data
	lg   loggateway.Logger
}

var (
	_ biza2a.FederationOrgRepo    = (*A2AFederationRepo)(nil)
	_ biza2a.FederationPolicyRepo = (*A2AFederationRepo)(nil)
	_ biza2a.FederationAuditRepo  = (*A2AFederationRepo)(nil)
)

// NewA2AFederationRepo constructs the federation repo.
func NewA2AFederationRepo(d *Data, lg loggateway.Logger) *A2AFederationRepo {
	return &A2AFederationRepo{data: d, lg: lg.With(loggateway.Domain("A2A_FED"))}
}

func (r *A2AFederationRepo) ensureDB() error {
	if r == nil || r.data == nil {
		return apierror.Internal("A2A_FED", "federation db nil")
	}
	return nil
}

// --- Org ---

func entFederationOrgToBiz(e *dataent.FederationOrg) biza2a.FederationOrg {
	return biza2a.FederationOrg{
		ID:             e.ID,
		Name:           e.Name,
		Domain:         e.Domain,
		PublicBaseURL:  e.PublicBaseURL,
		TrustLevel:     string(e.TrustLevel),
		AuthType:       e.AuthType,
		AuthConfigJSON: e.AuthConfigJSON,
		Status:         string(e.Status),
		JoinedAt:       e.JoinedAt,
		UpdatedAt:      e.UpdatedAt,
	}
}

func (r *A2AFederationRepo) UpsertOrg(ctx context.Context, org biza2a.FederationOrg) (biza2a.FederationOrg, error) {
	if err := r.ensureDB(); err != nil {
		return biza2a.FederationOrg{}, err
	}
	if org.TrustLevel == "" {
		org.TrustLevel = biza2a.TrustLevelNeutral
	}
	if org.Status == "" {
		org.Status = biza2a.OrgStatusActive
	}
	create := r.data.RW().Write(ctx).FederationOrg.Create().
		SetName(org.Name).
		SetDomain(org.Domain).
		SetPublicBaseURL(org.PublicBaseURL).
		SetTrustLevel(federationorg.TrustLevel(org.TrustLevel)).
		SetAuthType(org.AuthType).
		SetAuthConfigJSON(org.AuthConfigJSON).
		SetStatus(federationorg.Status(org.Status))
	if org.ID != "" {
		create = create.SetID(org.ID)
	}
	// Upsert by unique domain. joined_at and id are excluded from the update set
	// so re-registration preserves the original registration time and identity.
	err := create.
		OnConflictColumns(federationorg.FieldDomain).
		Update(func(u *dataent.FederationOrgUpsert) {
			u.UpdateName()
			u.UpdatePublicBaseURL()
			u.UpdateTrustLevel()
			u.UpdateAuthType()
			u.UpdateAuthConfigJSON()
			u.UpdateStatus()
			u.UpdateUpdatedAt()
		}).
		Exec(ctx)
	if err != nil {
		return biza2a.FederationOrg{}, entErrToBizErr(err, "A2A_FED")
	}
	row, err := r.data.RW().Read(ctx).FederationOrg.Query().
		Where(federationorg.DomainEQ(org.Domain)).
		Only(ctx)
	if err != nil {
		return biza2a.FederationOrg{}, entErrToBizErr(err, "A2A_FED")
	}
	return entFederationOrgToBiz(row), nil
}

func (r *A2AFederationRepo) GetOrg(ctx context.Context, id string) (biza2a.FederationOrg, error) {
	if err := r.ensureDB(); err != nil {
		return biza2a.FederationOrg{}, err
	}
	row, err := r.data.RW().Read(ctx).FederationOrg.Get(ctx, id)
	if err != nil {
		return biza2a.FederationOrg{}, entErrToBizErr(err, "A2A_FED")
	}
	return entFederationOrgToBiz(row), nil
}

func (r *A2AFederationRepo) ListOrgs(ctx context.Context) ([]biza2a.FederationOrg, error) {
	if err := r.ensureDB(); err != nil {
		return nil, err
	}
	rows, err := r.data.RW().Read(ctx).FederationOrg.Query().
		Order(dataent.Asc(federationorg.FieldJoinedAt), dataent.Asc(federationorg.FieldID)).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "A2A_FED")
	}
	out := make([]biza2a.FederationOrg, 0, len(rows))
	for _, row := range rows {
		out = append(out, entFederationOrgToBiz(row))
	}
	return out, nil
}

func (r *A2AFederationRepo) UpdateOrgTrust(ctx context.Context, id, trustLevel string) error {
	if err := r.ensureDB(); err != nil {
		return err
	}
	_, err := r.data.RW().Write(ctx).FederationOrg.UpdateOneID(id).
		SetTrustLevel(federationorg.TrustLevel(trustLevel)).
		Save(ctx)
	return entErrToBizErr(err, "A2A_FED")
}

func (r *A2AFederationRepo) DeleteOrg(ctx context.Context, id string) error {
	if err := r.ensureDB(); err != nil {
		return err
	}
	err := r.data.RW().Write(ctx).FederationOrg.DeleteOneID(id).Exec(ctx)
	return entErrToBizErr(err, "A2A_FED")
}

// --- Policy ---

func entFederationPolicyToBiz(e *dataent.FederationPolicy) biza2a.FederationPolicy {
	return biza2a.FederationPolicy{
		ID:          e.ID,
		CallerOrgID: e.CallerOrgID,
		CalleeOrgID: e.CalleeOrgID,
		Action:      string(e.Action),
		MaxPerMin:   e.MaxPerMin,
		DailyQuota:  e.DailyQuota,
		CreatedAt:   e.CreatedAt,
		UpdatedAt:   e.UpdatedAt,
	}
}

func (r *A2AFederationRepo) UpsertPolicy(ctx context.Context, p biza2a.FederationPolicy) (biza2a.FederationPolicy, error) {
	if err := r.ensureDB(); err != nil {
		return biza2a.FederationPolicy{}, err
	}
	if p.Action == "" {
		p.Action = biza2a.PolicyActionAllow
	}
	create := r.data.RW().Write(ctx).FederationPolicy.Create().
		SetCallerOrgID(p.CallerOrgID).
		SetCalleeOrgID(p.CalleeOrgID).
		SetAction(federationpolicy.Action(p.Action)).
		SetMaxPerMin(p.MaxPerMin).
		SetDailyQuota(p.DailyQuota)
	if p.ID != "" {
		create = create.SetID(p.ID)
	}
	// Upsert by unique org pair; id and created_at preserved on conflict.
	err := create.
		OnConflictColumns(federationpolicy.FieldCallerOrgID, federationpolicy.FieldCalleeOrgID).
		Update(func(u *dataent.FederationPolicyUpsert) {
			u.UpdateAction()
			u.UpdateMaxPerMin()
			u.UpdateDailyQuota()
			u.UpdateUpdatedAt()
		}).
		Exec(ctx)
	if err != nil {
		return biza2a.FederationPolicy{}, entErrToBizErr(err, "A2A_FED")
	}
	row, err := r.data.RW().Read(ctx).FederationPolicy.Query().
		Where(
			federationpolicy.CallerOrgIDEQ(p.CallerOrgID),
			federationpolicy.CalleeOrgIDEQ(p.CalleeOrgID),
		).
		Only(ctx)
	if err != nil {
		return biza2a.FederationPolicy{}, entErrToBizErr(err, "A2A_FED")
	}
	return entFederationPolicyToBiz(row), nil
}

func (r *A2AFederationRepo) GetPolicy(ctx context.Context, callerOrgID, calleeOrgID string) (biza2a.FederationPolicy, error) {
	if err := r.ensureDB(); err != nil {
		return biza2a.FederationPolicy{}, err
	}
	row, err := r.data.RW().Read(ctx).FederationPolicy.Query().
		Where(
			federationpolicy.CallerOrgIDEQ(callerOrgID),
			federationpolicy.CalleeOrgIDEQ(calleeOrgID),
		).
		Only(ctx)
	if err != nil {
		return biza2a.FederationPolicy{}, entErrToBizErr(err, "A2A_FED")
	}
	return entFederationPolicyToBiz(row), nil
}

func (r *A2AFederationRepo) ListPolicies(ctx context.Context) ([]biza2a.FederationPolicy, error) {
	if err := r.ensureDB(); err != nil {
		return nil, err
	}
	rows, err := r.data.RW().Read(ctx).FederationPolicy.Query().
		Order(dataent.Asc(federationpolicy.FieldCallerOrgID), dataent.Asc(federationpolicy.FieldCalleeOrgID)).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "A2A_FED")
	}
	out := make([]biza2a.FederationPolicy, 0, len(rows))
	for _, row := range rows {
		out = append(out, entFederationPolicyToBiz(row))
	}
	return out, nil
}

func (r *A2AFederationRepo) DeletePolicy(ctx context.Context, id string) error {
	if err := r.ensureDB(); err != nil {
		return err
	}
	err := r.data.RW().Write(ctx).FederationPolicy.DeleteOneID(id).Exec(ctx)
	return entErrToBizErr(err, "A2A_FED")
}

// --- Audit ---

func entFederationAuditToBiz(e *dataent.FederationAuditLog) biza2a.FederationAuditLog {
	return biza2a.FederationAuditLog{
		ID:            e.ID,
		Direction:     string(e.Direction),
		CallerOrgID:   e.CallerOrgID,
		CalleeOrgID:   e.CalleeOrgID,
		CallerAgentID: e.CallerAgentID,
		CalleeAgentID: e.CalleeAgentID,
		Capability:    e.Capability,
		Decision:      string(e.Decision),
		Status:        string(e.Status),
		LatencyMs:     e.LatencyMs,
		ErrorMessage:  e.ErrorMessage,
		CreatedAt:     e.CreatedAt,
		UpdatedAt:     e.UpdatedAt,
	}
}

func (r *A2AFederationRepo) CreateAudit(ctx context.Context, log biza2a.FederationAuditLog) (biza2a.FederationAuditLog, error) {
	if err := r.ensureDB(); err != nil {
		return biza2a.FederationAuditLog{}, err
	}
	if log.Direction == "" {
		log.Direction = biza2a.AuditDirectionOutbound
	}
	if log.Status == "" {
		log.Status = biza2a.FederationCallStatusPending
	}
	create := r.data.RW().Write(ctx).FederationAuditLog.Create().
		SetDirection(federationauditlog.Direction(log.Direction)).
		SetCallerOrgID(log.CallerOrgID).
		SetCalleeOrgID(log.CalleeOrgID).
		SetCallerAgentID(log.CallerAgentID).
		SetCalleeAgentID(log.CalleeAgentID).
		SetCapability(log.Capability).
		SetDecision(federationauditlog.Decision(log.Decision)).
		SetStatus(federationauditlog.Status(log.Status)).
		SetLatencyMs(log.LatencyMs).
		SetErrorMessage(log.ErrorMessage)
	if log.ID != "" {
		create = create.SetID(log.ID)
	}
	row, err := create.Save(ctx)
	if err != nil {
		return biza2a.FederationAuditLog{}, entErrToBizErr(err, "A2A_FED")
	}
	return entFederationAuditToBiz(row), nil
}

func (r *A2AFederationRepo) UpdateAuditResult(ctx context.Context, id, status string, latencyMs int64, errMsg string) error {
	if err := r.ensureDB(); err != nil {
		return err
	}
	_, err := r.data.RW().Write(ctx).FederationAuditLog.UpdateOneID(id).
		SetStatus(federationauditlog.Status(status)).
		SetLatencyMs(latencyMs).
		SetErrorMessage(errMsg).
		Save(ctx)
	return entErrToBizErr(err, "A2A_FED")
}

func federationAuditPredicates(filter biza2a.FederationAuditFilter) []predicate.FederationAuditLog {
	var preds []predicate.FederationAuditLog
	if filter.CallerOrgID != "" {
		preds = append(preds, federationauditlog.CallerOrgIDEQ(filter.CallerOrgID))
	}
	if filter.CalleeOrgID != "" {
		preds = append(preds, federationauditlog.CalleeOrgIDEQ(filter.CalleeOrgID))
	}
	if filter.Decision != "" {
		preds = append(preds, federationauditlog.DecisionEQ(federationauditlog.Decision(filter.Decision)))
	}
	if filter.Status != "" {
		preds = append(preds, federationauditlog.StatusEQ(federationauditlog.Status(filter.Status)))
	}
	return preds
}

func (r *A2AFederationRepo) ListAudits(ctx context.Context, filter biza2a.FederationAuditFilter) ([]biza2a.FederationAuditLog, int, error) {
	if err := r.ensureDB(); err != nil {
		return nil, 0, err
	}
	preds := federationAuditPredicates(filter)
	total, err := r.data.RW().Read(ctx).FederationAuditLog.Query().Where(preds...).Count(ctx)
	if err != nil {
		return nil, 0, entErrToBizErr(err, "A2A_FED")
	}
	q := r.data.RW().Read(ctx).FederationAuditLog.Query().
		Where(preds...).
		Order(dataent.Desc(federationauditlog.FieldCreatedAt), dataent.Desc(federationauditlog.FieldID))
	if filter.Limit > 0 {
		q = q.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		q = q.Offset(filter.Offset)
	}
	rows, err := q.All(ctx)
	if err != nil {
		return nil, 0, entErrToBizErr(err, "A2A_FED")
	}
	out := make([]biza2a.FederationAuditLog, 0, len(rows))
	for _, row := range rows {
		out = append(out, entFederationAuditToBiz(row))
	}
	return out, total, nil
}

// CountCallsSince counts allowed outbound calls for the org pair since the given
// time — the daily-quota counter (only decision=allowed consumes quota).
func (r *A2AFederationRepo) CountCallsSince(ctx context.Context, callerOrgID, calleeOrgID string, since time.Time) (int, error) {
	if err := r.ensureDB(); err != nil {
		return 0, err
	}
	n, err := r.data.RW().Read(ctx).FederationAuditLog.Query().
		Where(
			federationauditlog.CallerOrgIDEQ(callerOrgID),
			federationauditlog.CalleeOrgIDEQ(calleeOrgID),
			federationauditlog.DecisionEQ(federationauditlog.DecisionAllowed),
			federationauditlog.CreatedAtGTE(since),
		).
		Count(ctx)
	if err != nil {
		return 0, entErrToBizErr(err, "A2A_FED")
	}
	return n, nil
}
