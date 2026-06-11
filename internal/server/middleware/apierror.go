package middleware

import (
	"context"

	"aranea-agents/pkg/apierror"

	"github.com/go-kratos/kratos/v2/middleware"
)

// APIToKratos is a Kratos middleware that translates apierror.Error values
// returned by service/biz/data layers into kerrors.Error so that the
// transport layer (HTTP/gRPC) can produce the correct status code.
//
// Without this middleware, any *apierror.Error returned by inner layers
// would be treated as an unknown error and mapped to 500 by the Kratos
// error encoder.
//
// Install this middleware in the server middleware chain, after recovery
// and before any business middleware:
//
//	kratoshttp.Middleware(
//	    tracing.Server(),
//	    recovery.Recovery(),
//	    middleware.APIToKratos(),
//	    validate.Middleware(),
//	)
func APIToKratos() middleware.Middleware {
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			reply, err := handler(ctx, req)
			if err != nil {
				err = apierror.ToKratos(err)
			}
			return reply, err
		}
	}
}
