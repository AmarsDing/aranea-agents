package data

import (
	"aranea-agents/internal/data/ent"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// entErrToBizErr translates an Ent error into a Kratos business error.
// It preserves the original error via WithCause so the error chain is not lost.
//
// Translation rules:
//   - NotFound      → 404 NotFound
//   - ConstraintError → 409 Conflict
//   - NotLoaded     → 400 BadRequest (eager-loaded edge missing)
//   - default       → 500 InternalServer
func entErrToBizErr(err error, domain, msg string) error {
	if err == nil {
		return nil
	}
	switch {
	case ent.IsNotFound(err):
		return kerrors.NotFound(domain, msg).WithCause(err)
	case ent.IsConstraintError(err):
		return kerrors.Conflict(domain, msg).WithCause(err)
	case ent.IsNotLoaded(err):
		return kerrors.BadRequest(domain, msg).WithCause(err)
	default:
		return kerrors.InternalServer(domain, msg).WithCause(err)
	}
}
