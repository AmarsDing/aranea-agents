package data

import (
	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/apierror"
)

// entErrToBizErr translates an Ent error into a domain error (apierror.Error).
// It preserves the original error via the Cause field so the error chain is not lost.
//
// Translation rules:
//   - NotFound        → apierror.CodeNotFound
//   - ConstraintError → apierror.CodeConflict
//   - NotLoaded       → apierror.CodeBadRequest (eager-loaded edge missing)
//   - default         → apierror.CodeInternal
func entErrToBizErr(err error, domain string) error {
	if err == nil {
		return nil
	}
	switch {
	case ent.IsNotFound(err):
		return apierror.Wrap(err, apierror.CodeNotFound, domain)
	case ent.IsConstraintError(err):
		return apierror.Wrap(err, apierror.CodeConflict, domain)
	case ent.IsNotLoaded(err):
		return apierror.Wrap(err, apierror.CodeBadRequest, domain)
	default:
		return apierror.Wrap(err, apierror.CodeInternal, domain)
	}
}
