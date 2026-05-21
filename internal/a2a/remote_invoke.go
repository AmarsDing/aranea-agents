package a2a

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	a2aclient "trpc.group/trpc-go/trpc-a2a-go/client"
	"trpc.group/trpc-go/trpc-a2a-go/protocol"
)

// InvokeRemoteRegistry calls an external A2A service registered in the workspace catalog.
func InvokeRemoteRegistry(ctx context.Context, remote biz.A2ARemoteAgent, capability, payloadJSON string, timeoutSec int) (string, error) {
	if !remote.Enabled {
		return "", kerrors.Forbidden("A2A", "remote agent is disabled")
	}
	if err := CheckCalleeCard(remote.DiscoveredCard, nil, capability); err != nil {
		return "", err
	}
	targetURL := strings.TrimSpace(remote.RemoteURL)
	if targetURL == "" {
		targetURL = strings.TrimSpace(remote.AgentCardURL)
	}
	if targetURL == "" {
		return "", kerrors.BadRequest("A2A", "remote_url is required")
	}
	if timeoutSec <= 0 {
		timeoutSec = 30
	}

	opts, err := ClientAuthOptions(remote.AuthType, remote.AuthConfigJSON)
	if err != nil {
		return "", err
	}
	opts = append(opts, a2aclient.WithTimeout(time.Duration(timeoutSec)*time.Second))
	client, err := a2aclient.NewA2AClient(targetURL, opts...)
	if err != nil {
		return "", kerrors.BadRequest("A2A", "connect remote a2a: "+err.Error())
	}

	input := PayloadToInput(payloadJSON, capability)
	msg := protocol.NewMessage(protocol.MessageRoleUser, []protocol.Part{protocol.NewTextPart(input)})
	if md := CapabilityMetadata(capability); md != nil {
		msg.Metadata = md
	}
	blocking := true
	result, err := client.SendMessage(ctx, protocol.SendMessageParams{
		Message: msg,
		Configuration: &protocol.SendMessageConfiguration{
			Blocking: &blocking,
		},
	})
	if err != nil {
		return "", kerrors.InternalServer("A2A", "remote invoke failed: "+err.Error())
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
