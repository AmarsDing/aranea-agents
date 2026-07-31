package a2a

import (
	"context"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// AuditLogger records federation invocation decisions and results
// (design F.5):
//   - RecordAllowed is fail-closed (FED-NFR1 audit integrity): a persistence
//     failure returns an error and the caller MUST abort the invocation (500).
//   - RecordDenied is best-effort: the call is already rejected, so a
//     persistence failure is only logged (Error), never escalated.
//   - RecordResult is best-effort: the invocation outcome already exists, so
//     an update failure is only logged (Warn) and never rewrites the outcome.
type AuditLogger struct {
	repo FederationAuditRepo
	lg   loggateway.Logger
}

// NewAuditLogger constructs an AuditLogger.
func NewAuditLogger(repo FederationAuditRepo, lg loggateway.Logger) *AuditLogger {
	return &AuditLogger{repo: repo, lg: lg}
}

// prepareDecision fills audit defaults: generated ID, outbound direction and
// pending status (a decision row starts pending; RecordResult finalizes it).
func (l *AuditLogger) prepareDecision(entry FederationAuditLog) (FederationAuditLog, error) {
	if entry.ID == "" {
		id, err := NewID()
		if err != nil {
			return FederationAuditLog{}, err
		}
		entry.ID = id
	}
	if entry.Direction == "" {
		entry.Direction = AuditDirectionOutbound
	}
	if entry.Status == "" {
		entry.Status = FederationCallStatusPending
	}
	return entry, nil
}

// RecordAllowed persists an allowed decision. Fail-closed: any persistence
// error is returned so the governance chain rejects the invocation.
func (l *AuditLogger) RecordAllowed(ctx context.Context, entry FederationAuditLog) (FederationAuditLog, error) {
	entry.Decision = DecisionAllowed
	entry, err := l.prepareDecision(entry)
	if err != nil {
		return FederationAuditLog{}, err
	}
	stored, err := l.repo.CreateAudit(ctx, entry)
	if err != nil {
		// K2: fail-closed audit persistence failure — the invocation is aborted.
		if l.lg != nil {
			l.lg.Error("federation allowed-audit persist failed; invocation aborted (fail-closed)",
				loggateway.StepID("a2a.fed.audit.allowed"),
				loggateway.Str("audit_id", entry.ID),
				loggateway.Str("caller_org_id", entry.CallerOrgID),
				loggateway.Str("callee_org_id", entry.CalleeOrgID),
				loggateway.Err(err),
			)
		}
		return FederationAuditLog{}, apierror.Internal(apierror.DomainA2AFed, "persist federation audit decision: %v", err).WithCause(err)
	}
	return stored, nil
}

// RecordDenied persists a denied decision (denied_trust / denied_policy /
// denied_quota). Best-effort: creation failure only logs Error because the
// call has already been rejected.
func (l *AuditLogger) RecordDenied(ctx context.Context, entry FederationAuditLog) {
	entry, err := l.prepareDecision(entry)
	if err != nil {
		if l.lg != nil {
			l.lg.Error("federation denied-audit id generation failed",
				loggateway.StepID("a2a.fed.audit.denied"),
				loggateway.Err(err),
			)
		}
		return
	}
	if _, err := l.repo.CreateAudit(ctx, entry); err != nil && l.lg != nil {
		l.lg.Error("federation denied-audit persist failed; invocation already rejected",
			loggateway.StepID("a2a.fed.audit.denied"),
			loggateway.Str("caller_org_id", entry.CallerOrgID),
			loggateway.Str("callee_org_id", entry.CalleeOrgID),
			loggateway.Str("decision", entry.Decision),
			loggateway.Err(err),
		)
	}
}

// ListAudits delegates audit panel queries to the repo (read path; no
// governance semantics, so it lives on AuditLogger rather than widening the
// usecase's repo dependencies).
func (l *AuditLogger) ListAudits(ctx context.Context, filter FederationAuditFilter) ([]FederationAuditLog, int, error) {
	return l.repo.ListAudits(ctx, filter)
}

// RecordResult finalizes an audit row with the invocation outcome.
// Best-effort: update failure only logs Warn.
func (l *AuditLogger) RecordResult(ctx context.Context, id, status string, latencyMs int64, errMsg string) {
	if err := l.repo.UpdateAuditResult(ctx, id, status, latencyMs, errMsg); err != nil && l.lg != nil {
		l.lg.Warn("federation audit result update failed; invocation outcome unchanged",
			loggateway.StepID("a2a.fed.audit.result"),
			loggateway.Str("audit_id", id),
			loggateway.Str("status", status),
			loggateway.Err(err),
		)
	}
}
