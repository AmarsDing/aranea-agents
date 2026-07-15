package a2a

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	a2abiz "aranea-agents/internal/biz/a2a"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	a2aclient "trpc.group/trpc-go/trpc-a2a-go/client"
	"trpc.group/trpc-go/trpc-a2a-go/protocol"
)

// InvokeRemoteRegistry calls an external A2A service registered in the workspace catalog.
// It applies retry with exponential backoff for transient errors (network timeout, 5xx).
func InvokeRemoteRegistry(ctx context.Context, remote biz.A2ARemoteAgent, capability, payloadJSON string, timeoutSec int, lg loggateway.Logger, retryPolicy a2abiz.RetryPolicy) (string, error) {
	if !remote.Enabled {
		return "", apierror.Forbidden(apierror.DomainA2A, "remote agent is disabled")
	}
	if err := CheckCalleeCard(remote.DiscoveredCard, nil, capability); err != nil {
		return "", err
	}
	targetURL := strings.TrimSpace(remote.RemoteURL)
	if targetURL == "" {
		targetURL = strings.TrimSpace(remote.AgentCardURL)
	}
	if targetURL == "" {
		return "", apierror.BadRequest(apierror.DomainA2A, "remote_url is required")
	}
	// C-07: reject private/loopback/metadata URLs before any dial.
	if err := validateRemoteURL(targetURL); err != nil {
		lg.Warn("A2A remote invoke SSRF blocked", loggateway.StepID("a2a.invoke.remote_ssrf_blocked"), loggateway.Str("remote_url", targetURL), loggateway.Err(err))
		return "", apierror.BadRequest(apierror.DomainA2A, "remote_url blocked by SSRF policy").WithCause(err)
	}
	if timeoutSec <= 0 {
		timeoutSec = a2abiz.DefaultRemoteInvokeTimeoutSec
	}

	lg.Info("A2A remote invoke started", loggateway.StepID("a2a.invoke.remote"), loggateway.Str("remote_url", targetURL), loggateway.Str("capability", capability))

	opts, err := ClientAuthOptions(remote.AuthType, remote.AuthConfigJSON)
	if err != nil {
		lg.Warn("A2A remote auth options failed", loggateway.StepID("a2a.invoke.remote_auth_fail"), loggateway.Str("remote_url", targetURL), loggateway.Err(err))
		return "", err
	}
	timeout := time.Duration(timeoutSec) * time.Second
	opts = append(opts,
		a2aclient.WithTimeout(timeout),
		a2aclient.WithHTTPClient(newSSRFSafeHTTPClient(timeout)),
	)
	client, err := a2aclient.NewA2AClient(targetURL, opts...)
	if err != nil {
		lg.Warn("A2A remote client creation failed", loggateway.StepID("a2a.invoke.remote_connect_fail"), loggateway.Str("remote_url", targetURL), loggateway.Err(err))
		return "", apierror.BadRequest(apierror.DomainA2A, "connect remote a2a").WithCause(err)
	}

	input := PayloadToInput(payloadJSON, capability)
	msg := protocol.NewMessage(protocol.MessageRoleUser, []protocol.Part{protocol.NewTextPart(input)})
	if md := CapabilityMetadata(capability); md != nil {
		msg.Metadata = md
	}
	blocking := true
	params := protocol.SendMessageParams{
		Message: msg,
		Configuration: &protocol.SendMessageConfiguration{
			Blocking: &blocking,
		},
	}

	var result *protocol.MessageResult
	var lastErr error
	maxAttempts := 1 + retryPolicy.MaxRetries
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			backoff := backoffDuration(retryPolicy.InitialBackoff, retryPolicy.MaxBackoff, attempt-1)
			lg.Info("A2A remote invoke retry", loggateway.StepID("a2a.invoke.remote_retry"), loggateway.Str("remote_url", targetURL), loggateway.Int("attempt", attempt+1), loggateway.Str("backoff", backoff.String()))
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
		result, lastErr = client.SendMessage(ctx, params)
		if lastErr == nil {
			break
		}
		if !isRetryableError(lastErr) {
			lg.Warn("A2A remote invoke non-retryable error", loggateway.StepID("a2a.invoke.remote_non_retryable"), loggateway.Str("remote_url", targetURL), loggateway.Err(lastErr))
			break
		}
		lg.Warn("A2A remote invoke retryable error", loggateway.StepID("a2a.invoke.remote_retryable"), loggateway.Str("remote_url", targetURL), loggateway.Err(lastErr), loggateway.Int("attempt", attempt+1))
	}
	if lastErr != nil {
		lg.Error("A2A remote invoke call failed", loggateway.StepID("a2a.invoke.remote_call_fail"), loggateway.Str("remote_url", targetURL), loggateway.Err(lastErr))
		return "", apierror.Internal(apierror.DomainA2A, "remote invoke failed").WithCause(lastErr)
	}

	text := messageResultText(result)
	out, err := json.Marshal(map[string]any{
		"capability": capability,
		"output":     text,
		"remote_id":  remote.ID,
	})
	if err != nil {
		return text, nil
	}
	return string(out), nil
}

// backoffDuration computes exponential backoff: initialBackoff * 2^step, capped at maxBackoff.
func backoffDuration(initial, max time.Duration, step int) time.Duration {
	d := time.Duration(float64(initial) * math.Pow(2, float64(step)))
	if d > max {
		return max
	}
	return d
}

func messageResultText(result *protocol.MessageResult) string {
	if result == nil || result.Result == nil {
		return ""
	}
	switch v := result.Result.(type) {
	case *protocol.Message:
		return partsText(v.Parts)
	case *protocol.Task:
		if v.Status.Message != nil {
			if t := partsText(v.Status.Message.Parts); t != "" {
				return t
			}
		}
		if v.Artifacts != nil {
			var b strings.Builder
			for _, art := range v.Artifacts {
				if t := partsText(art.Parts); t != "" {
					if b.Len() > 0 {
						b.WriteString("\n")
					}
					b.WriteString(t)
				}
			}
			return b.String()
		}
	}
	return ""
}

func partsText(parts []protocol.Part) string {
	var b strings.Builder
	for _, part := range parts {
		switch p := part.(type) {
		case protocol.TextPart:
			if p.Text != "" {
				if b.Len() > 0 {
					b.WriteString("\n")
				}
				b.WriteString(p.Text)
			}
		case *protocol.TextPart:
			if p != nil && p.Text != "" {
				if b.Len() > 0 {
					b.WriteString("\n")
				}
				b.WriteString(p.Text)
			}
		}
	}
	return b.String()
}
