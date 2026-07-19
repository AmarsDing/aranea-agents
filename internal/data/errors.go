package data

import (
	"database/sql"
	"errors"

	"aranea-agents/internal/biz/shared"
	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/apierror"

	"github.com/lib/pq"
)

// isPgUniqueViolation reports whether err is a PostgreSQL unique-constraint
// violation (SQLSTATE 23505). This is needed because ent's IsConstraintError
// relies on string matching against the driver's error message, which fails
// when the server returns localized (e.g. Chinese) messages.
func isPgUniqueViolation(err error) bool {
	var pgErr *pq.Error
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

// entErrToBizErr translates a data-layer error (Ent, raw SQL, Postgres, or biz
// sentinel) into a domain error (apierror.Error). It preserves the original
// error via the Cause field so the error chain is not lost.
//
// This is the single entry point for error translation in all repo methods.
// Use this instead of apierror.Wrap(err, apierror.CodeInternal, domain) to
// preserve error semantics automatically.
//
// Translation rules:
//   - *apierror.Error (already)     → pass through
//   - Ent NotFound / sql.ErrNoRows  → apierror.CodeNotFound
//   - Ent ConstraintError           → apierror.CodeConflict
//   - shared.ErrMessageDuplicate    → apierror.CodeConflict
//   - shared.ErrAgentKeyConflict    → apierror.CodeConflict
//   - Ent NotLoaded                 → apierror.CodeBadRequest
//   - Postgres 23505 (unique)       → apierror.CodeConflict
//   - Postgres 23503 (foreign key)  → apierror.CodeConflict
//   - Postgres 23502 (not null)     → apierror.CodeBadRequest
//   - Postgres 23514 (check)        → apierror.CodeBadRequest
//   - default                       → apierror.CodeInternal
func entErrToBizErr(err error, domain string) error {
	if err == nil {
		return nil
	}
	// Already an apierror — pass through.
	if ae, ok := apierror.From(err); ok {
		return ae
	}
	// Known biz sentinels that map to specific codes.
	if errors.Is(err, shared.ErrMessageDuplicate) || errors.Is(err, shared.ErrAgentKeyConflict) {
		return apierror.Wrap(err, apierror.CodeConflict, domain)
	}
	// Ent errors.
	if ent.IsNotFound(err) {
		return apierror.Wrap(err, apierror.CodeNotFound, domain)
	}
	if ent.IsConstraintError(err) {
		return apierror.Wrap(err, apierror.CodeConflict, domain)
	}
	if ent.IsNotLoaded(err) {
		return apierror.Wrap(err, apierror.CodeBadRequest, domain)
	}
	// Raw SQL errors.
	if errors.Is(err, sql.ErrNoRows) {
		return apierror.Wrap(err, apierror.CodeNotFound, domain)
	}
	// Postgres errors (lib/pq). Use errors.As to handle wrapped errors.
	var pgErr *pq.Error
	if errors.As(err, &pgErr) {
		switch pgErr.Code.Name() {
		case "unique_violation", "foreign_key_violation":
			return apierror.Wrap(err, apierror.CodeConflict, domain)
		case "not_null_violation", "check_violation":
			return apierror.Wrap(err, apierror.CodeBadRequest, domain)
		}
	}
	// Default — internal.
	return apierror.Wrap(err, apierror.CodeInternal, domain)
}
