package auth

import (
	"context"
	"errors"
	"sync"
	"time"

	"aranea-agents/pkg/loggateway"
)

// 本文件实现三项目整合 P1-3 会话吊销（P0 详设 01 §5）：
// JWT claim tv 与服务端当前 token_version 逐请求比对；不一致即拒绝。
// 版本源为 Redis write-through 缓存（不靠 TTL），miss 回源 DB。
//
// 分层约束：pkg/auth 不得 import internal/*，故校验器实现
// （internal/data.TokenVersionStore）经 SetTokenVersionChecker 在组合根
// （cmd/admin newApp）注入，与 SetLogger / SetOnGRPCUnauthenticated 同模式。

// ErrRevocationSubscriptionUnavailable 表示令牌版本递增事件通道在本部署中
// 永久不可用（如未配置 Redis）。订阅方（P1-4 WS 主动关闭）据此停止重试——
// 继续退避重连只是无用功；最终一致性由周期兜底扫描（TokenVersionIsCurrent）
// 保证，多进程部署必须配置 Redis 才有即时踢连接。
var ErrRevocationSubscriptionUnavailable = errors.New("auth: revocation subscription unavailable")

// TokenVersionChecker reports the current session-revocation version of a user.
// 实现必须并发安全；实现侧负责 Redis 不可用时的 DB 回源。
type TokenVersionChecker interface {
	// CurrentVersion returns admins.token_version for userID.
	// 用户不存在视同版本 0（旧 token 仍可被判 tv=0 通过一次，
	// 但任何真实 bump 后旧 token 立即失效）。
	CurrentVersion(ctx context.Context, userID int64) (int64, error)
}

var (
	tvCheckerMu sync.RWMutex
	tvChecker   TokenVersionChecker
)

// SetTokenVersionChecker installs the process-wide checker. Called once from
// the composition root after the data layer is ready; nil disables enforcement
// （单元测试 / 工具进程）。
func SetTokenVersionChecker(c TokenVersionChecker) {
	tvCheckerMu.Lock()
	defer tvCheckerMu.Unlock()
	tvChecker = c
}

// tokenVersionCheckTimeout 上限校验额外时延：Redis 命中亚毫秒；miss 回源 DB
// 走正常连接池。超时即 fail-closed（R11）。
const tokenVersionCheckTimeout = 800 * time.Millisecond

// tokenRevokedThrottle 节流「会话已吊销」Warn（踢人/改密后客户端常带旧 token
// 重试风暴）；与 tokenInvalidThrottle 同 10s 窗口。
var tokenRevokedThrottle = newLogThrottle(10 * time.Second)

// TokenVersionEnforced reports whether a token-version checker is installed
// （即本进程是否执行会话吊销校验）。供长连接侧（P1-4 WS）在启动时判定
// 吊销能力是否可用，避免用一次真实查询去试探。
func TokenVersionEnforced() bool {
	tvCheckerMu.RLock()
	defer tvCheckerMu.RUnlock()
	return tvChecker != nil
}

// TokenVersionIsCurrent 比对 userID 的服务端当前版本与 tokenVersion。
//
// 返回值：
//   - enforced=false：checker 未接线（单元测试 / 工具进程），调用方按旧行为处理；
//   - err != nil：校验出错（Redis 与 DB 均不可用），调用方按各自 R11 策略处理；
//   - current=true：版本一致，token 仍有效。
//
// 单请求路径与 WS 长连接兜底扫描（internal/server/ws_revocation.go）共用此判定，
// 避免两处比对语义漂移。
func TokenVersionIsCurrent(ctx context.Context, userID, tokenVersion int64) (current, enforced bool, err error) {
	tvCheckerMu.RLock()
	checker := tvChecker
	tvCheckerMu.RUnlock()
	if checker == nil {
		return false, false, nil
	}
	cctx, cancel := context.WithTimeout(ctx, tokenVersionCheckTimeout)
	defer cancel()
	v, err := checker.CurrentVersion(cctx, userID)
	if err != nil {
		return false, true, err
	}
	return v == tokenVersion, true, nil
}

// enforceTokenVersion 比对 claims.tv 与服务端当前版本。
//   - checker 未接线（测试/工具进程）：跳过，保持旧行为。
//   - 校验出错（Redis+DB 均不可用）：fail-closed 拒绝（R11：安全场景不 fail-open）。
//   - 版本不一致：拒绝（踢会话/改密/离职/角色密级变更后旧 token 立即失效）。
func enforceTokenVersion(ctx context.Context, a *Auth, lg loggateway.Logger, step string) error {
	if a == nil {
		return nil
	}
	current, enforced, err := TokenVersionIsCurrent(ctx, a.UserID, a.TokenVersion)
	if !enforced {
		return nil
	}
	if err != nil {
		if lg == nil {
			lg = getLogger()
		}
		if ok, suppressed := tokenRevokedThrottle.allow(); ok {
			fields := []loggateway.Field{
				loggateway.StepID(step),
				loggateway.Str("reason", "version_check_error"),
				loggateway.Err(err),
			}
			if suppressed > 0 {
				fields = append(fields, loggateway.Int("suppressed", suppressed))
			}
			lg.Warn("auth rejected: token version check failed (fail-closed)", fields...)
		}
		return ErrUnauthorized
	}
	if !current {
		if lg == nil {
			lg = getLogger()
		}
		if ok, suppressed := tokenRevokedThrottle.allow(); ok {
			fields := []loggateway.Field{
				loggateway.StepID(step),
				loggateway.Str("reason", "session_revoked"),
				loggateway.Int64("user_id", a.UserID),
			}
			if suppressed > 0 {
				fields = append(fields, loggateway.Int("suppressed", suppressed))
			}
			lg.Warn("auth rejected: session revoked (token_version mismatch)", fields...)
		}
		return ErrUnauthorized
	}
	return nil
}
