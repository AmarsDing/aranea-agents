package application

import (
	"arenea/backend/internal/kernel/errs"
	"fmt"
)

func validationError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errs.ErrValidation, fmt.Sprintf(format, args...))
}

func conflictError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errs.ErrConflict, fmt.Sprintf(format, args...))
}
