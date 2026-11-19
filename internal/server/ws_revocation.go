package server

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"aranea-agents/internal/biz"
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
//
// 例外（评审 2026-11-09 H1）：版本查询命中的不是故障而是**账号已删**
// （ErrAdminNotFound）时，扫描主动关闭连接——删号后不会再有任何 bump 事件，
// 不主动关闭则连接会在 token 自然过期前（最长 7 天）一直存活。这与故障
// fail-open 不矛盾：删除是确定终态，不是抖动。
//
// 覆盖范围（评审 2026-11-09 MEDIUM-2）：除聊天 /v1/ws 的 connStore 外，
// 非 connStore 托管的长连接（/v1/voice 语音链路）经 RevocableConn 抽象 +
// RegisterRevocableConn 登记进本机制，共用同一订阅循环与同一兜底扫描——
// 不得复制第二套清扫循环。登记方在连接自然断开时必须配对调用
// UnregisterRevocableConn（见 VoiceWSServer.readPump defer）。

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

// RevocableConn 是"非 connStore 托管、但须随会话吊销关闭"的长连接最小抽象
// （评审 2026-11-09 MEDIUM-2：/v1/voice 语音链路）。实现方必须保证
// RevokeClose 幂等——pub/sub 主路径与兜底扫描可能并发触发同一条连接。
type RevocableConn interface {
	// RevokeUserID 返回属主 userID（十进制字符串，与 JWT sub 一致）。
	RevokeUserID() string
	// RevokeTokenVersion 返回握手期 JWT 的 tv claim（兜底扫描比对用）。
	RevokeTokenVersion() int64
	// RevokeSessionID 返回会话 ID（仅日志观测用）。
	RevokeSessionID() string
	// RevokeClose 以给定关闭码发送 close frame 并拆除连接（幂等）。
	RevokeClose(code int, reason string)
}

// revocationRuntime 收纳 P1-4 的依赖与生命周期守卫，保持 WSServer 字段数平稳
// （AS-COG-01；与 outboxSweepRuntime 同一模式）。
type revocationRuntime struct {
	sub     TokenBumpSubscriber
	started atomic.Bool
	// extra 是 connStore 之外、需随会话吊销关闭的长连接次级索引
	// （MEDIUM-2：/v1/voice）：userID → 连接集合。与 connStore.byUser 同构，
	// 清扫/pub/sub 双路径一并遍历，不另起循环。
	extraMu sync.RWMutex
	extra   map[string]map[RevocableConn]struct{}
}

// RegisterRevocableConn 把一条非 connStore 托管的长连接（如 /v1/voice）纳入
// 会话吊销覆盖（MEDIUM-2）。连接自然断开时登记方必须配对调用
// UnregisterRevocableConn；重复登记同一实例为幂等（map set 语义）。
func (s *WSServer) RegisterRevocableConn(c RevocableConn) {
	if s == nil || c == nil || c.RevokeUserID() == "" {
		return
	}
	s.revocation.extraMu.Lock()
	defer s.revocation.extraMu.Unlock()
	if s.revocation.extra == nil {
		s.revocation.extra = make(map[string]map[RevocableConn]struct{})
	}
	set := s.revocation.extra[c.RevokeUserID()]
	if set == nil {
		set = make(map[RevocableConn]struct{})
		s.revocation.extra[c.RevokeUserID()] = set
	}
	set[c] = struct{}{}
}

// UnregisterRevocableConn 从会话吊销覆盖中摘除连接（幂等）。
func (s *WSServer) UnregisterRevocableConn(c RevocableConn) {
	if s == nil || c == nil {
		return
	}
	s.revocation.extraMu.Lock()
	defer s.revocation.extraMu.Unlock()
	set := s.revocation.extra[c.RevokeUserID()]
	if set == nil {
		return
	}
	delete(set, c)
	if len(set) == 0 {
		delete(s.revocation.extra, c.RevokeUserID())
	}
}

// extraUserIDs 返回 extra 索引中全部 userID 快照（兜底扫描用）。
func (rt *revocationRuntime) extraUserIDs() []string {
	rt.extraMu.RLock()
	defer rt.extraMu.RUnlock()
	out := make([]string, 0, len(rt.extra))
	for uid := range rt.extra {
		out = append(out, uid)
	}
	return out
}

// forEachExtraForUser 遍历某用户的全部外部登记连接（快照后在锁外回调，
// 与 connStore.forEachConnForUser 同一自死锁防护：回调会关闭连接，进而触发
// 对端 readPump → UnregisterRevocableConn → extraMu 写锁）。
func (rt *revocationRuntime) forEachExtraForUser(userID string, fn func(RevocableConn)) {
	if userID == "" {
		return
	}
	rt.extraMu.RLock()
	snapshot := make([]RevocableConn, 0, len(rt.extra[userID]))
	for c := range rt.extra[userID] {
		snapshot = append(snapshot, c)
	}
	rt.extraMu.RUnlock()
	for _, c := range snapshot {
		fn(c)
	}
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

// revocationSweepTarget 统一聊天 connStore 与外部登记连接（MEDIUM-2 voice）
// 的兜底扫描目标：判定逻辑（tv 比对/销户/故障边界）共享一份，不随连接
// 类型分叉。
type revocationSweepTarget struct {
	userID       int64
	tokenVersion int64
	close        func(reason string)
}

// collectRevocationTargets 汇总 connStore（/v1/ws）与 extra 索引（/v1/voice）
// 的在线连接为统一扫描目标。
func (s *WSServer) collectRevocationTargets() []revocationSweepTarget {
	var out []revocationSweepTarget
	for _, uid := range s.store.userIDs() {
		userID, err := strconv.ParseInt(uid, 10, 64)
		if err != nil {
			continue
		}
		s.store.forEachConnForUser(uid, func(wc *wsConn) {
			out = append(out, revocationSweepTarget{
				userID:       userID,
				tokenVersion: wc.tokenVersion,
				close:        func(reason string) { s.revokeConn(wc, reason) },
			})
		})
	}
	for _, uid := range s.revocation.extraUserIDs() {
		userID, err := strconv.ParseInt(uid, 10, 64)
		if err != nil {
			continue
		}
		s.revocation.forEachExtraForUser(uid, func(c RevocableConn) {
			out = append(out, revocationSweepTarget{
				userID:       userID,
				tokenVersion: c.RevokeTokenVersion(),
				close:        func(reason string) { s.revokeExtraConn(c, reason) },
			})
		})
	}
	return out
}

// sweepRevokedConnections 扫描在线连接，关闭 tv 已过期的连接。
func (s *WSServer) sweepRevokedConnections(ctx context.Context) {
	closed, checkErrors := 0, 0
	for _, t := range s.collectRevocationTargets() {
		current, enforced, err := auth.TokenVersionIsCurrent(ctx, t.userID, t.tokenVersion)
		if !enforced {
			continue
		}
		if err != nil {
			// 账号已删（评审 H1）：确定终态，主动关闭——删号后不会再有
			// bump 事件，skip 会让连接存活至 token 自然过期（最长 7 天）。
			// 其他错误（版本源故障）保持 fail-open 边界（见文件头 R11）。
			if errors.Is(err, biz.ErrAdminNotFound) {
				t.close("account_deleted")
				closed++
				continue
			}
			checkErrors++
			continue
		}
		if !current {
			t.close("session_revoked_sweep")
			closed++
		}
	}
	if closed > 0 || checkErrors > 0 {
		s.lg.Warn("WS 会话吊销兜底扫描完成",
			loggateway.StepID("ws.revocation_sweep"),
			loggateway.Int("closed", closed),
			loggateway.Int("check_errors", checkErrors))
	}
}

// revokeUserConnections 关闭某用户的全部在线连接（pub/sub 主路径）：
// connStore（/v1/ws）与外部登记的 /v1/voice 连接一并关闭（MEDIUM-2）。
func (s *WSServer) revokeUserConnections(userID int64, reason string) {
	uid := strconv.FormatInt(userID, 10)
	s.store.forEachConnForUser(uid, func(wc *wsConn) {
		s.revokeConn(wc, reason)
	})
	s.revocation.forEachExtraForUser(uid, func(c RevocableConn) {
		s.revokeExtraConn(c, reason)
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

// revokeExtraConn 摘除并关闭一条外部登记连接（MEDIUM-2：/v1/voice），与
// revokeConn 同语义：先摘索引再关闭——RevokeClose 触发的对端 readPump 清理
// 会再次 UnregisterRevocableConn，map 删除幂等。
func (s *WSServer) revokeExtraConn(c RevocableConn, reason string) {
	if c == nil {
		return
	}
	s.UnregisterRevocableConn(c)
	c.RevokeClose(wsCloseCodeSessionRevoked, reason)
	s.lg.Warn("WS 连接因会话吊销被主动关闭",
		loggateway.StepID("ws.session_revoked"),
		loggateway.SessionID(c.RevokeSessionID()),
		loggateway.Str("user_id", c.RevokeUserID()),
		loggateway.Str("reason", reason))
}
