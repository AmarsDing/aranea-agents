package agent

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/tools/deferred"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

const (
	defaultRetryMaxAttempts      = 2
	defaultRetryInitialIntervalMs = 500
	defaultRetryBackoffFactor    = 2.0
	defaultRetryMaxIntervalMs    = 5000
)

func buildToolFilter(s *biz.AgentRuntimeSettings, dm *deferred.DeferredToolManager, lg loggateway.Logger) trpctool.FilterFunc {
	var filters []trpctool.FilterFunc
	if denyList, err := biz.JSONStringList(s.ToolsDenyJSON); err != nil {
		lg.Warn("tools deny list JSON parse failed; deny list will NOT be enforced",
			loggateway.StepID("agent.tool_build"),
			loggateway.Err(err),
		)
	} else if len(denyList) > 0 {
		filters = append(filters, trpctool.NewExcludeToolNamesFilter(denyList...))
	}
	if dm != nil {
		filters = append(filters, dm.ToolFilter())
	}
	if len(filters) == 0 {
		return nil
	}
	return func(ctx context.Context, t trpctool.Tool) bool {
		for _, f := range filters {
			if !f(ctx, t) {
				return false
			}
		}
		return true
	}
}

func buildToolRetryPolicy(s *biz.AgentRuntimeSettings) *trpctool.RetryPolicy {
	if !s.ToolsEnabled || !s.ToolsRetryEnabled {
		return nil
	}
	maxAttempts := s.ToolsRetryMaxAttempts
	if maxAttempts < defaultRetryMaxAttempts {
		maxAttempts = defaultRetryMaxAttempts
	}
	initialMs := s.ToolsRetryInitialIntervalMs
	if initialMs <= 0 {
		initialMs = defaultRetryInitialIntervalMs
	}
	backoff := s.ToolsRetryBackoffFactor
	if backoff <= 0 {
		backoff = defaultRetryBackoffFactor
	}
	maxMs := s.ToolsRetryMaxIntervalMs
	if maxMs <= 0 {
		maxMs = defaultRetryMaxIntervalMs
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
