// Package channel holds platform adapters (Feishu/Lark, …) for bridging external
// IM surfaces into Aranea. Interface shape follows the split used in trpc-agent-go
// openclaw/channel: a stable identity, a long-running surface when needed, and
// optional outbound text delivery (see EXTENDING.md in pkg/trpc-agent-go/openclaw).
package channel

import "context"

// Identified is a stable channel kind for logs, metrics, and config (e.g. "feishu").
// Parity: openclaw/channel.Channel.ID.
type Identified interface {
	ID() string
}

// Runner blocks until ctx is done or an unrecoverable error (polling, WebSocket bots).
// Parity: openclaw/channel.Channel.Run.
//
// Webhook-only adapters do not use this; ingress stays on Kratos HTTP handlers.
type Runner interface {
	Identified

	Run(ctx context.Context) error
}

// OutboundText delivers plain text to a transport-specific recipient key
// (e.g. Feishu user open_id). Parity: openclaw/channel.TextSender outbound half.
type OutboundText interface {
	Identified

	SendText(ctx context.Context, recipient string, text string) error
}
