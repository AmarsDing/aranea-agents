// Package trpc bridges biz A2A proxy agents to trpc-agent-go a2aagent.
package trpc

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"

	a2aclient "trpc.group/trpc-go/trpc-a2a-go/client"
	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	a2aagent "trpc.group/trpc-go/trpc-agent-go/agent/a2aagent"
)

// BuildTRPCA2AAgent wraps a remote A2A service as a local agent.Agent.
func BuildTRPCA2AAgent(_ context.Context, ag biz.Agent, cfg biz.A2AProxyConfig) (trpcagent.Agent, error) {
	if strings.TrimSpace(ag.AgentKey) == "" {
		return nil, apierror.BadRequest(apierror.DomainAgent, "agent_key required")
	}
	remoteURL := strings.TrimSpace(cfg.RemoteURL)
	if remoteURL == "" {
		remoteURL = strings.TrimSpace(cfg.AgentCardURL)
	}
	if remoteURL == "" {
		return nil, apierror.BadRequest(apierror.DomainAgent, "a2a_proxy remote_url is required")
	}

	opts := []a2aagent.Option{
		a2aagent.WithName(ag.AgentKey),
		a2aagent.WithDescription(strings.TrimSpace(ag.AgentDescription)),
		a2aagent.WithAgentCardURL(remoteURL),
	}
	if cfg.EnableStreaming {
		opts = append(opts, a2aagent.WithEnableStreaming(true))
	} else {
		opts = append(opts, a2aagent.WithEnableStreaming(false))
	}
	timeoutSec := cfg.TimeoutSeconds
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	clientOpts := []a2aclient.Option{a2aclient.WithTimeout(time.Duration(timeoutSec) * time.Second)}
	if authOpts, err := a2aProxyClientAuthOptions(cfg); err != nil {
		return nil, err
	} else {
		clientOpts = append(clientOpts, authOpts...)
	}
	opts = append(opts, a2aagent.WithA2AClientExtraOptions(clientOpts...))

	proxy, err := a2aagent.New(opts...)
	if err != nil {
		return nil, apierror.Internal(apierror.DomainAgent, "build a2a proxy agent").WithCause(err)
	}
	return proxy, nil
}
