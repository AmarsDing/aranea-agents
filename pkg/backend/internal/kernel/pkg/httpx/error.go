package httpx

import (
	"arenea/backend/internal/kernel/errs"
	"context"
	"database/sql"
	"errors"
	"net/http"
)

// ErrorResponse 为 JSON API 的通用错误体（迁移 #28b 自 internal/transport/response.go）。
type ErrorResponse struct {
	Error string `json:"error"`
}

// PublicError 将域错误与 HTTP 状态、对外可展示文案对应起来。
func PublicError(fallbackStatus int, err error) (int, string) {
	if err == nil {
		return fallbackStatus, http.StatusText(fallbackStatus)
	}
	status := StatusForError(fallbackStatus, err)
	switch {
	case status >= http.StatusInternalServerError:
		return status, "internal server error"
	case status == http.StatusNotFound:
		return status, "resource not found"
	case status == http.StatusUnauthorized:
		return status, "unauthorized"
	case status == http.StatusConflict:
		switch {
		case errors.Is(err, errs.ErrL1Overflow),
			errors.Is(err, errs.ErrRevisionConflict),
			errors.Is(err, errs.ErrTaskNotWritable):
			return status, err.Error()
		default:
			return status, "conflict"
		}
	case status == 499:
		return status, "request cancelled"
	default:
		return status, err.Error()
	}
}

// StatusForError 按 domain 哨兵/哨兵 + fallback 选 HTTP 状态码。
func StatusForError(fallbackStatus int, err error) int {
	switch {
	case errors.Is(err, errs.ErrValidation),
		errors.Is(err, errs.ErrInvalidFieldPath),
		errors.Is(err, errs.ErrInvalidFieldValue):
		return http.StatusBadRequest
	case errors.Is(err, errs.ErrNotFound), errors.Is(err, sql.ErrNoRows):
		return http.StatusNotFound
	case errors.Is(err, errs.ErrFieldTooLarge):
		return http.StatusUnprocessableEntity
	case errors.Is(err, errs.ErrConflict),
		errors.Is(err, errs.ErrL1Overflow),
		errors.Is(err, errs.ErrRevisionConflict),
		errors.Is(err, errs.ErrTaskNotWritable):
		return http.StatusConflict
	case errors.Is(err, errs.ErrUnauthorized):
		return http.StatusUnauthorized
	case errors.Is(err, errs.ErrInternal):
		return http.StatusInternalServerError
	case errors.Is(err, context.Canceled):
		return 499
	}
	if fallbackStatus <= 0 {
		return http.StatusInternalServerError
	}
	return fallbackStatus
}
