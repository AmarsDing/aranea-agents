package biz

import "github.com/go-kratos/kratos/v2/errors"

var (
	ErrAdminNotFound = errors.NotFound("ADMIN", "admin not found")
	ErrNotFound      = errors.NotFound("NOT_FOUND", "resource not found")
)
