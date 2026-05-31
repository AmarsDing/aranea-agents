package agent

import (
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func buildToolFilter(s *biz.AgentRuntimeSettings) trpctool.FilterFunc {
	denyList, err := biz.JSONStringList(s.ToolsDenyJSON)
	if err != nil {
		loggateway.Global().Warn("json string list parse failed",
			loggateway.StepID("system.agent.tool_build"),
			loggateway.Err(err),
		)
		return nil
	}
	if len(denyList) == 0 {
		return nil
	}
	return trpctool.NewExcludeToolNamesFilter(denyList...)
}

func buildToolRetryPolicy(s *biz.AgentRuntimeSettings) *trpctool.RetryPolicy {
	if !s.ToolsEnabled || !s.ToolsRetryEnabled {
		return nil
	}
	maxAttempts := s.ToolsRetryMaxAttempts
	if maxAttempts < 2 {
		maxAttempts = 2
	}
	initialMs := s.ToolsRetryInitialIntervalMs
	if initialMs <= 0 {
		initialMs = 500
	}
	backoff := s.ToolsRetryBackoffFactor
	if backoff <= 0 {
		backoff = 2.0
	}
	maxMs := s.ToolsRetryMaxIntervalMs
	if maxMs <= 0 {
		maxMs = 5000
	}
	return &trpctool.RetryPolicy{
		MaxAttempts:     maxAttempts,
		InitialInterval: time.Duration(initialMs) * time.Millisecond,
		BackoffFactor:   backoff,
		MaxInterval:     time.Duration(maxMs) * time.Millisecond,
		Jitter:          s.ToolsRetryJitter,
		RetryOn:         trpctool.DefaultRetryOn,
	}
}
