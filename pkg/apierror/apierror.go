// Package apierror defines the unified error model used across the biz, data,
// and service layers of Aranea-Agents.
//
// Layers:
//   - biz:     return *apierror.Error directly
//   - data:    wrap Ent / SQL errors via apierror.Wrap
//   - service: call apierror.ToKratos before returning to the HTTP handler
package apierror

import (
	"errors"
	"fmt"
	"net/http"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// Code is a stable, machine-readable error identifier.
type Code string

const (
	CodeNotFound     Code = "NOT_FOUND"
	CodeBadRequest   Code = "BAD_REQUEST"
	CodeUnauthorized Code = "UNAUTHORIZED"
	CodeForbidden    Code = "FORBIDDEN"
	CodeConflict     Code = "CONFLICT"
	CodeInternal     Code = "INTERNAL"
	CodeUnavailable  Code = "UNAVAILABLE"
	CodeRateLimit    Code = "RATE_LIMITED"
)

// Error is the canonical error type for all Aranea product-layer errors.
type Error struct {
	// Code is a stable, machine-readable identifier.
	Code Code
	// Domain identifies the subsystem that produced the error (e.g. "agent", "session").
	Domain string
	// Message is a human-readable description suitable for log output.
	Message string
	// Cause is the underlying error that triggered this error (optional).
	Cause error
	// Meta holds arbitrary key-value pairs for structured logging / tracing.
	Meta map[string]string
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s/%s] %s: %v", e.Domain, e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s/%s] %s", e.Domain, e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.Cause }

// Is satisfies errors.Is when target is *Error with the same Code AND Domain.
// Both fields must match so that errors.Is(err, ErrAgentKeyConflict) does not
// accidentally match ErrMessageDuplicate (both are CodeConflict but different
// Domain). For Code-only matching, use apierror.From + explicit field comparison.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	if !ok {
		return false
	}
	return e.Code == t.Code && e.Domain == t.Domain
}

// WithMeta returns a shallow copy of the error with additional metadata.
func (e *Error) WithMeta(key, value string) *Error {
	cp := *e
	cp.Meta = make(map[string]string, len(e.Meta)+1)
	for k, v := range e.Meta {
		cp.Meta[k] = v
	}
	cp.Meta[key] = value
	return &cp
}

// WithCause returns a shallow copy of the error with the given cause.
func (e *Error) WithCause(cause error) *Error {
	cp := *e
	cp.Cause = cause
	return &cp
}

// ---- Constructors ----

func newf(code Code, domain, msg string, args ...any) *Error {
	if len(args) > 0 {
		msg = fmt.Sprintf(msg, args...)
	}
	return &Error{Code: code, Domain: domain, Message: msg}
}

func NotFound(domain, msg string, args ...any) *Error {
	return newf(CodeNotFound, domain, msg, args...)
}

func BadRequest(domain, msg string, args ...any) *Error {
	return newf(CodeBadRequest, domain, msg, args...)
}

func Unauthorized(domain, msg string, args ...any) *Error {
	return newf(CodeUnauthorized, domain, msg, args...)
}

func Forbidden(domain, msg string, args ...any) *Error {
	return newf(CodeForbidden, domain, msg, args...)
}

func Conflict(domain, msg string, args ...any) *Error {
	return newf(CodeConflict, domain, msg, args...)
}

func Internal(domain, msg string, args ...any) *Error {
	return newf(CodeInternal, domain, msg, args...)
}

func Unavailable(domain, msg string, args ...any) *Error {
	return newf(CodeUnavailable, domain, msg, args...)
}

func RateLimit(domain, msg string, args ...any) *Error {
	return newf(CodeRateLimit, domain, msg, args...)
}

// Wrap wraps a foreign error (e.g. Ent, SQL) with the given code and domain.
// If err is nil, Wrap returns nil.
func Wrap(err error, code Code, domain string) *Error {
	if err == nil {
		return nil
	}
	// Don't double-wrap.
	var ae *Error
	if errors.As(err, &ae) {
		return ae
	}
	return &Error{Code: code, Domain: domain, Message: err.Error(), Cause: err}
}

// From extracts *Error from an error chain. Returns (nil, false) if err is nil
// or the chain contains no *Error.
func From(err error) (*Error, bool) {
	if err == nil {
		return nil, false
	}
	var ae *Error
	if errors.As(err, &ae) {
		return ae, true
	}
	return nil, false
}

// ToKratos converts any error to a Kratos HTTP-aware error.
// If err is already a kerrors.Error it is returned unchanged.
// If err is *apierror.Error it is mapped to the appropriate HTTP status.
// All other errors become 500 Internal Server Error.
//
// Mapping rules:
//   - reason = Domain + "_" + Code (e.g. "AGENT_NOT_FOUND") so the frontend can
//     distinguish errors from different subsystems.
//   - CodeInternal errors: Message is replaced with a generic string to avoid
//     leaking internal details (SQL, file paths, etc.) to the client. The
//     original message is preserved in the kerrors metadata for server-side logs.
func ToKratos(err error) error {
	if err == nil {
		return nil
	}
	// Already a Kratos error — pass through.
	var ke *kerrors.Error
	if errors.As(err, &ke) {
		return err
	}

	ae, ok := From(err)
	if !ok {
		return kerrors.InternalServer("INTERNAL", "internal error")
	}

	reason := ae.Domain + "_" + string(ae.Code)
	msg := ae.Message

	// Sanitize internal errors: never expose raw internal messages to clients.
	if ae.Code == CodeInternal {
		msg = "internal error"
	}

	switch ae.Code {
	case CodeNotFound:
		return kerrors.NotFound(reason, msg)
	case CodeBadRequest:
		return kerrors.BadRequest(reason, msg)
	case CodeUnauthorized:
		return kerrors.New(http.StatusUnauthorized, reason, msg)
	case CodeForbidden:
		return kerrors.Forbidden(reason, msg)
	case CodeConflict:
		return kerrors.New(http.StatusConflict, reason, msg)
	case CodeUnavailable:
		return kerrors.ServiceUnavailable(reason, msg)
	case CodeRateLimit:
		return kerrors.New(http.StatusTooManyRequests, reason, msg)
	case CodeInternal:
		return kerrors.InternalServer(reason, msg)
	default:
		return kerrors.InternalServer("UNKNOWN_CODE", msg)
	}
}
