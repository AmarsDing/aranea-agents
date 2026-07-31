package runtime

import (
	"context"
	"strings"
	"sync/atomic"

	"aranea-agents/internal/event"
)

// 连接生命周期流程日志 stepID（中文标题注册见 internal/event/flow_log.go）。
const (
	flowStepConnectOpen  = "channel.connect.open"
	flowStepConnectClose = "channel.connect.close"
	flowStepConnectError = "channel.connect.error"
)

// connectFlow builds the channel-domain flow emitter for one supervised
// connector instance. Returns nil when the monitor bus is not wired, making
// all EmitConnect* helpers no-ops.
func (m *Manager) connectFlow(ctx context.Context) *event.TraceEmitter {
	if m == nil || m.bus == nil {
		return nil
	}
	return event.NewTraceEmitterForRun(event.TraceEmitterOpts{
		Ctx:    ctx,
		Domain: event.TraceDomainChannel,
		LG:     m.lg,
		Infra:  event.NewInfraFromBus(m.bus),
	})
}

// EmitConnectOpen emits 渠道连接建立（done）。accountID 为平台侧账号标识
// （如 feishu app_id、telegram bot username），为空时省略。无 emitter 时 no-op。
func EmitConnectOpen(ctx context.Context, platform, channelID, accountID, message string, extra ...event.Pair) {
	flow := event.TraceEmitterFromContext(ctx)
	if flow == nil {
		return
	}
	pairs := make([]event.Pair, 0, len(extra)+3)
	pairs = append(pairs, event.P("platform", platform), event.P("channel_id", channelID))
	if strings.TrimSpace(accountID) != "" {
		pairs = append(pairs, event.P("account_id", accountID))
	}
	pairs = append(pairs, extra...)
	flow.LogDone(flowStepConnectOpen, message, pairs...)
}

// EmitConnectClose emits 渠道连接断开（done，正常停止/远端正常关闭）。
// 无 emitter 时 no-op。
func EmitConnectClose(ctx context.Context, platform, channelID, message string, extra ...event.Pair) {
	flow := event.TraceEmitterFromContext(ctx)
	if flow == nil {
		return
	}
	pairs := make([]event.Pair, 0, len(extra)+2)
	pairs = append(pairs, event.P("platform", platform), event.P("channel_id", channelID))
	pairs = append(pairs, extra...)
	flow.LogDone(flowStepConnectClose, message, pairs...)
}

// EmitConnectError emits 渠道连接异常（error，连接失败/异常断开/重连失败）。
// 无 emitter 时 no-op。
func EmitConnectError(ctx context.Context, platform, channelID, message string, err error, extra ...event.Pair) {
	flow := event.TraceEmitterFromContext(ctx)
	if flow == nil {
		return
	}
	pairs := make([]event.Pair, 0, len(extra)+3)
	pairs = append(pairs, event.P("platform", platform), event.P("channel_id", channelID))
	if err != nil {
		pairs = append(pairs, event.P("error", err.Error()))
	}
	pairs = append(pairs, extra...)
	flow.LogError(flowStepConnectError, message, pairs...)
}

// ConnectFlowGuard 收敛 SDK 内部重连风暴期间的重复发射：
// 每个故障期只发一次 error（直到下一次 EmitOpen），每次（重新）建立发一次 open，
// 连接建立后的正常关闭只发一次 close。
type ConnectFlowGuard struct{ state atomic.Int32 }

const (
	connectFlowIdle int32 = iota
	connectFlowOpen
	connectFlowFailed
	connectFlowClosed
)

// EmitOpen runs f on every successful (re)establishment and resets the episode.
func (g *ConnectFlowGuard) EmitOpen(f func()) {
	if g == nil || f == nil {
		return
	}
	g.state.Store(connectFlowOpen)
	f()
}

// EmitError runs f at most once per outage episode (until the next EmitOpen).
func (g *ConnectFlowGuard) EmitError(f func()) {
	if g == nil || f == nil {
		return
	}
	for {
		s := g.state.Load()
		if s == connectFlowFailed || s == connectFlowClosed {
			return
		}
		if g.state.CompareAndSwap(s, connectFlowFailed) {
			f()
			return
		}
	}
}

// EmitClose runs f at most once, only when the connection was open
// （异常终态已由 EmitError 覆盖，未建立过的连接不发 close）。
func (g *ConnectFlowGuard) EmitClose(f func()) {
	if g == nil || f == nil {
		return
	}
	if g.state.CompareAndSwap(connectFlowOpen, connectFlowClosed) {
		f()
	}
}
