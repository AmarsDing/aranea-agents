package codeexecutor

import (
	"context"
	"regexp"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	trpcagentcodeexec "trpc.group/trpc-go/trpc-agent-go/codeexecutor"
)

var exitCodePattern = regexp.MustCompile(`\[exit (\d+)\]`)

type metricsExecutor struct {
	inner trpcagentcodeexec.CodeExecutor
	kind  string
}

func wrapMetrics(inner trpcagentcodeexec.CodeExecutor, kind string) trpcagentcodeexec.CodeExecutor {
	if inner == nil {
		return nil
	}
	k := strings.TrimSpace(kind)
	if k == "" {
		k = TypeLocal
	}
	return &metricsExecutor{inner: inner, kind: k}
}

func (m *metricsExecutor) CodeBlockDelimiter() trpcagentcodeexec.CodeBlockDelimiter {
	return m.inner.CodeBlockDelimiter()
}

func (m *metricsExecutor) ExecuteCode(ctx context.Context, input trpcagentcodeexec.CodeExecutionInput) (trpcagentcodeexec.CodeExecutionResult, error) {
	blockCount := len(input.CodeBlocks)
	if blockCount == 0 {
		blockCount = 1
	}
	timer := prometheus.NewTimer(codeExecDuration.WithLabelValues(m.kind))
	defer timer.ObserveDuration()

	result, err := m.inner.ExecuteCode(ctx, input)
	status := "error"
	if err == nil {
		status = classifyExecutionStatus(result.Output)
	}
	codeExecRunsTotal.WithLabelValues(m.kind, status).Inc()
	codeExecBlocksTotal.WithLabelValues(m.kind, status).Add(float64(blockCount))
	if status == "oom" {
		codeExecOOMTotal.WithLabelValues(m.kind).Inc()
	}
	return result, err
}

func classifyExecutionStatus(output string) string {
	if strings.Contains(output, "[OOM killed]") {
		return "oom"
	}
	if strings.Contains(output, "[timeout]") {
		return "timeout"
	}
	for _, match := range exitCodePattern.FindAllStringSubmatch(output, -1) {
		if len(match) > 1 && match[1] != "0" {
			return "error"
		}
	}
	return "success"
}
