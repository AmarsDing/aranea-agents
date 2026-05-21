package service

import (
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
)

func graphExecutionFinishErr(exec *biz.GraphExecution) error {
	if exec == nil {
		return nil
	}
	switch strings.TrimSpace(exec.Status) {
	case "failed", "cancelled":
		if msg := strings.TrimSpace(exec.ErrorMessage); msg != "" {
			return fmt.Errorf("%s", msg)
		}
		return fmt.Errorf("graph execution %s", exec.Status)
	default:
		return nil
	}
}
