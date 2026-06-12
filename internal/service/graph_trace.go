package service

import (
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

func graphExecutionFinishErr(exec *biz.GraphExecution) error {
	if exec == nil {
		return nil
	}
	switch strings.TrimSpace(exec.Status) {
	case string(biz.GraphExecFailed), string(biz.GraphExecCancelled):
		if msg := strings.TrimSpace(exec.ErrorMessage); msg != "" {
			return apierror.Internal("GRAPH", msg)
		}
		return apierror.Internal("GRAPH", "graph execution %s", exec.Status)
	default:
		return nil
	}
}
