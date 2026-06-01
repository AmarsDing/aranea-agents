package outbound

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	ch "aranea-agents/internal/channel"
	"aranea-agents/pkg/loggateway"
)

type DeliveryTarget struct {
	Channel string `json:"channel,omitempty"`
	Target  string `json:"target,omitempty"`
}

type OutboundFile struct {
	Path    string
	Name    string
	AsVoice bool
}

type OutboundMessage struct {
	Text  string
	Files []OutboundFile
}

type Router struct {
	mu             sync.RWMutex
	textSenders    map[string]TextSender
	messageSenders map[string]MessageSender
	lg             loggateway.Logger
}

func NewRouter(lg loggateway.Logger) *Router {
	return &Router{
		textSenders:    make(map[string]TextSender),
		messageSenders: make(map[string]MessageSender),
		lg:             lg,
	}
}

func (r *Router) RegisterOutboundText(sender ch.OutboundText) {
	if sender == nil {
		return
	}
	r.RegisterTextSender(WrapOutboundText(sender))
}

func (r *Router) RegisterTextSender(sender TextSender) {
	if r == nil || sender == nil {
		return
	}
	id := strings.TrimSpace(sender.ID())
	if id == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.textSenders[id] = sender
}

func (r *Router) RegisterMessageSender(sender MessageSender) {
	if r == nil || sender == nil {
		return
	}
	id := strings.TrimSpace(sender.ID())
	if id == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messageSenders[id] = sender
}

func (r *Router) Channels() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	index := make(map[string]struct{})
	for id := range r.textSenders {
		index[id] = struct{}{}
	}
	for id := range r.messageSenders {
		index[id] = struct{}{}
	}
	out := make([]string, 0, len(index))
	for id := range index {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func (r *Router) SendText(ctx context.Context, target DeliveryTarget, text string) error {
	return r.SendMessage(ctx, target, OutboundMessage{Text: text})
}

func (r *Router) SendMessage(ctx context.Context, target DeliveryTarget, msg OutboundMessage) error {
	if r == nil {
		return fmt.Errorf("outbound: nil router")
	}
	channelID := strings.TrimSpace(target.Channel)
	if channelID == "" {
		return fmt.Errorf("outbound: empty channel")
	}

	r.lg.Info("Outbound send message", loggateway.StepID("outbound.send"), loggateway.Str("channel", channelID), loggateway.Str("target", target.Target))

	r.mu.RLock()
	messageSender := r.messageSenders[channelID]
	textSender := r.textSenders[channelID]
	r.mu.RUnlock()
	if messageSender != nil {
		if err := messageSender.SendMessage(ctx, target.Target, msg); err != nil {
			r.lg.Error("Outbound message send failed", loggateway.StepID("outbound.send.fail"), loggateway.Str("channel", channelID), loggateway.Str("target", target.Target), loggateway.Err(err))
			return err
		}
		return nil
	}
	if textSender == nil {
		r.lg.Warn("Outbound unsupported channel", loggateway.StepID("outbound.send.unsupported"), loggateway.Str("channel", channelID))
		return fmt.Errorf("outbound: unsupported channel: %s", channelID)
	}
	if len(msg.Files) > 0 {
		return fmt.Errorf("outbound: channel does not support file delivery: %s", channelID)
	}
	if err := textSender.SendText(ctx, target.Target, msg.Text); err != nil {
		r.lg.Error("Outbound text send failed", loggateway.StepID("outbound.send.fail"), loggateway.Str("channel", channelID), loggateway.Str("target", target.Target), loggateway.Err(err))
		return err
	}
	return nil
}
