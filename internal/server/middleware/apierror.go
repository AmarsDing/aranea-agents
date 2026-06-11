package middleware

import (
	"context"
	"database/sql"
	"errors"

	"aranea-agents/pkg/apierror"

	"github.com/go-kratos/kratos/v2/middleware"
)

// APIToKratos is a Kratos middleware that translates apierror.Error values
// returned by service/biz/data layers into kerrors.Error so that the
// transport layer (HTTP/gRPC) can produce the correct status code.
//
// Translation order:
//  1. kerrors.Error → pass through
//  2. *apierror.Error → map via apierror.ToKratos
//  3. sql.ErrNoRows → 404 Not Found
//  4. everything else → 500 Internal Server Error
func APIToKratos() middleware.Middleware {
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			reply, err := handler(ctx, req)
			if err != nil {
				err = translateError(err)
			}
			return reply, err
		}
	}
}

// translateError converts any internal error into a Kratos-compatible error.
func translateError(err error) error {
	// Already a Kratos error — pass through.
	var ke interface{ Error() string; GRPCStatus() }
	if errors.As(err, &ke) {
		return err
	}

	// *apierror.Error — use the canonical mapper.
	if ae, ok := apierror.From(err); ok {
		return apierror.ToKratos(ae)
	}

	// sql.ErrNoRows — treat as not found.
	if errors.Is(err, sql.ErrNoRows) {
		return apierror.ToKratos(apierror.NotFound("DATA", err.Error()))
	}

	// Fallback — 500 Internal.
	return apierror.ToKratos(err)
}
