package service

import (
	"strings"

	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

func graphExecutionFinishErr(exec *biz.GraphExecution) error {
	if exec == nil {
		return nil
	}
	switch strings.TrimSpace(exec.Status) {
	case "failed", "cancelled":
		if msg := strings.TrimSpace(exec.ErrorMessage); msg != "" {
			return kerrors.InternalServer("GRAPH", msg)
		}
		return kerrors.InternalServer("GRAPH", "graph execution "+exec.Status)
	default:
		return nil
	}
}
