package server

import (
	"context"
	"errors"
	"strconv"
	"sync/atomic"
	"time"

	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// 本文件实现三项目整合 P1-4：会话吊销对 WebSocket 长连接生效。
//
// 背景：HTTP/gRPC 面逐请求比对 JWT 的 tv claim（P1-3），但 WS 是长连接，
// 握手时校验一次后不再走中间件——被踢会话、改密、离职、角色密级变更后，
// 旧连接仍会继续收发消息。故需要主动关闭：
//
//  1. **主路径（即时）**：订阅 Redis pub/sub 频道 auth:tv_bump
//     （internal/data.TokenVersionStore.Bump 发布），收到 bump 立即以
//     wsCloseCodeSessionRevoked 关闭该用户的全部连接。
//  2. **兜底（最终一致，R11）**：周期扫描在线连接，用 auth.TokenVersionIsCurrent
//     （Redis 命中 → miss 回源 DB，权威）比对握手期的 tv；不一致即关闭。
//     pub/sub 断链、重连窗口丢消息、Redis 未配置（无跨进程通道）等场景下，
//     连接最多延迟一个扫描周期被关闭，不会永久存活。
//
// fail-closed 边界（R11）：兜底扫描遇到版本源不可用（Redis + DB 同时故障）
// **不关闭连接**——此时 HTTP/gRPC 面已 fail-closed 拒绝该用户的全部请求，
// 会话实际已不可用；误杀全部在线连接会把存储抖动放大成全站断连。故障持续
// 时按节流告警，且不改变"任何真实 bump 最终都会踢掉连接"的收敛性。

const (
	// wsCloseCodeSessionRevoked 是自定义关闭码（4000–4999 为应用保留段）：
	// 客户端收到该码应清空本地 token 并跳转登录。
	wsCloseCodeSessionRevoked = 4401
	// wsRevocationSweepDefaultSecs 是兜底扫描周期（秒），可用
	// WS_REVOCATION_SWEEP_SECS 覆盖。
	wsRevocationSweepDefaultSecs = 60
	// wsRevocationResubscribeMaxBackoff 是订阅断线重连的最大退避。
	wsRevocationResubscribeMaxBackoff = 30 * time.Second
)

// TokenBumpSubscriber 订阅"会话吊销"事件流（P1-4 主路径）。实现方必须阻塞，
// 每收到一次 bump 回调 onBump，并在 ctx 取消或链路断开时返回错误。
//
// 实现见 internal/data.TokenVersionStore（分层约束：server 不得 import data，
// 端口在消费侧声明，组合根注入）。
type TokenBumpSubscriber interface {
	SubscribeSessionRevocations(ctx context.Context, onBump func(userID int64, newVersion int64)) error
}

// revocationRuntime 收纳 P1-4 的依赖与生命周期守卫，保持 WSServer 字段数平稳
// （AS-COG-01；与 outboxSweepRuntime 同一模式）。
type revocationRuntime struct {
	sub     TokenBumpSubscriber
	started atomic.Bool
}

// SetTokenBumpSubscriber 注入会话吊销事件订阅者（组合根调用）。
func (s *WSServer) SetTokenBumpSubscriber(sub TokenBumpSubscriber) {
	if s == nil {
		return
	}
	s.revocation.sub = sub
}

// startSessionRevocation 在 Start 时拉起订阅循环与兜底扫描（幂等）。
func (s *WSServer) startSessionRevocation(ctx context.Context) {
	if s == nil || !s.revocation.started.CompareAndSwap(false, true) {
		return
	}
	if !auth.TokenVersionEnforced() {
		s.lg.Warn("WS 会话吊销未启用：token version checker 未接线（组合根应在数据层就绪后调用 auth.SetTokenVersionChecker）",
			loggateway.StepID("ws.revocation_disabled"))
		return
	}
	if s.revocation.sub == nil {
		s.lg.Warn("WS 会话吊销主路径未接线（无 pub/sub 订阅者），仅靠周期兜底扫描；多进程部署必须配置 Redis",
			loggateway.StepID("ws.revocation_no_subscriber"))
	}
	if s.revocation.sub != nil {
		safego.Go(ctx, "ws-revocation-subscribe", func() { s.runTokenBumpSubscription(ctx) })
	}
	safego.Go(ctx, "ws-revocation-sweep", func() { s.runRevocationSweep(ctx) })
}

// runTokenBumpSubscription 订阅循环：断线退避重连；通道在本部署永久不可用
// （如未配置 Redis）时记录一次并退出，交给兜底扫描保证收敛。
func (s *WSServer) runTokenBumpSubscription(ctx context.Context) {
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		err := s.revocation.sub.SubscribeSessionRevocations(ctx, func(userID, _ int64) {
			s.revokeUserConnections(userID, "session_revoked")
		})
		if ctx.Err() != nil {
			return
		}
		if errors.Is(err, auth.ErrRevocationSubscriptionUnavailable) {
			s.lg.Warn("WS 会话吊销事件通道不可用（Redis 未配置），仅靠周期兜底扫描",
				loggateway.StepID("ws.revocation_subscribe_unavailable"),
				loggateway.Err(err))
			return
		}
		s.lg.Warn("WS 会话吊销订阅中断，将退避重连",
			loggateway.StepID("ws.revocation_subscribe"),
			loggateway.Duration(backoff.Milliseconds()),
			loggateway.Err(err))
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < wsRevocationResubscribeMaxBackoff {
			backoff *= 2
			if backoff > wsRevocationResubscribeMaxBackoff {
				backoff = wsRevocationResubscribeMaxBackoff
			}
		}
	}
}

// runRevocationSweep 周期兜底扫描（见文件头 fail-closed 边界说明）。
func (s *WSServer) runRevocationSweep(ctx context.Context) {
	interval := time.Duration(envInt("WS_REVOCATION_SWEEP_SECS", wsRevocationSweepDefaultSecs)) * time.Second
	if interval <= 0 {
		interval = time.Duration(wsRevocationSweepDefaultSecs) * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sweepRevokedConnections(ctx)
		}
	}
}

// sweepRevokedConnections 扫描在线连接，关闭 tv 已过期的连接。
func (s *WSServer) sweepRevokedConnections(ctx context.Context) {
	closed, checkErrors := 0, 0
	for _, uid := range s.store.userIDs() {
		userID, err := strconv.ParseInt(uid, 10, 64)
		if err != nil {
			continue
		}
		s.store.forEachConnForUser(uid, func(wc *wsConn) {
			current, enforced, err := auth.TokenVersionIsCurrent(ctx, userID, wc.tokenVersion)
			if !enforced {
				return
			}
			if err != nil {
				checkErrors++
				return
			}
			if !current {
				s.revokeConn(wc, "session_revoked_sweep")
				closed++
			}
		})
	}
	if closed > 0 || checkErrors > 0 {
		s.lg.Warn("WS 会话吊销兜底扫描完成",
			loggateway.StepID("ws.revocation_sweep"),
			loggateway.Int("closed", closed),
			loggateway.Int("check_errors", checkErrors))
	}
}

// revokeUserConnections 关闭某用户的全部在线连接（pub/sub 主路径）。
func (s *WSServer) revokeUserConnections(userID int64, reason string) {
	uid := strconv.FormatInt(userID, 10)
	s.store.forEachConnForUser(uid, func(wc *wsConn) {
		s.revokeConn(wc, reason)
	})
}

// revokeConn 摘除并关闭单条连接。先摘索引再关连接：关闭会触发 readPump 的
// 清理路径，重复摘除是幂等的（connStore.remove 找不到即无操作）。
func (s *WSServer) revokeConn(wc *wsConn, reason string) {
	if wc == nil {
		return
	}
	s.removeConn(wc)
	wc.closeWithCode(wsCloseCodeSessionRevoked, reason)
	s.lg.Warn("WS 连接因会话吊销被主动关闭",
		loggateway.StepID("ws.session_revoked"),
		loggateway.SessionID(wc.sessionID),
		loggateway.Str("user_id", wc.userID),
		loggateway.Str("reason", reason))
}
