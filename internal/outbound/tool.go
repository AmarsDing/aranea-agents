package outbound

import (
	"context"
	"encoding/json"
	"strings"

	"aranea-agents/pkg/apierror"

	"trpc.group/trpc-go/trpc-agent-go/tool"
)

const toolName = "message"

type messageToolInput struct {
	Text    string   `json:"text"`
	File    string   `json:"file,omitempty"`
	Files   []string `json:"files,omitempty"`
	Channel string   `json:"channel,omitempty"`
	Target  string   `json:"target,omitempty"`
}

type MessageTool struct {
	router *Router
}

func NewMessageTool(router *Router) *MessageTool {
	return &MessageTool{router: router}
}

func (t *MessageTool) Declaration() *tool.Declaration {
	return &tool.Declaration{
		Name:        toolName,
		Description: "Send text and optional files through registered channels. If channel/target are omitted, resolves from current session context.",
		InputSchema: &tool.Schema{
			Type: "object",
			Properties: map[string]*tool.Schema{
				"text": {
					Type:        "string",
					Description: "Message text to send. Required unless files are provided.",
				},
				"files": {
					Type:        "array",
					Items:       &tool.Schema{Type: "string"},
					Description: "Optional local file paths to send. Only supported on channels with MessageSender capability.",
				},
				"file": {
					Type:        "string",
					Description: "Alias for a single file path.",
				},
				"channel": {
					Type:        "string",
					Description: "Channel id (e.g. telegram, slack, feishu). When omitted, resolves from runtime/session context.",
				},
				"target": {
					Type:        "string",
					Description: "Channel-specific target (e.g. chat_id, open_id, channel_id). When omitted, resolves from runtime/session context.",
				},
			},
		},
	}
}

func (t *MessageTool) Call(ctx context.Context, args []byte) (any, error) {
	if t == nil || t.router == nil {
		return nil, apierror.Internal(apierror.DomainOutbound, "message tool not configured")
	}
	var in messageToolInput
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, apierror.BadRequest(apierror.DomainOutbound, "invalid args").WithCause(err)
	}
	text := strings.TrimSpace(in.Text)
	paths := collectPaths(in.File, in.Files)
	if text == "" && len(paths) == 0 {
		return nil, apierror.BadRequest(apierror.DomainOutbound, "text or files required")
	}
	target, err := ResolveTarget(ctx, DeliveryTarget{
		Channel: in.Channel,
		Target:  in.Target,
	})
	if err != nil {
		return nil, err
	}
	msg := OutboundMessage{Text: text}
	for _, p := range paths {
		msg.Files = append(msg.Files, OutboundFile{Path: p})
	}
	if err := t.router.SendMessage(ctx, target, msg); err != nil {
		return nil, apierror.Internal(apierror.DomainOutbound, "send failed").WithCause(err)
	}
	return map[string]any{
		"ok":         true,
		"channel":    target.Channel,
		"target":     target.Target,
		"files_sent": len(msg.Files),
	}, nil
}

func collectPaths(single string, multi []string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, p := range append([]string{single}, multi...) {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}
