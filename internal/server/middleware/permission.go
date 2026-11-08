package middleware

import (
	"context"
	"sync"
	"time"

	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"

	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/transport"
	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
)

// 本文件实现三项目整合 P1-5 权限中间件（P0 详设 01 §6、02 §4）：
// 按 transport operation 查表执行权限码硬判。
//
// operation 键形态（kratos v2.9.2 实测）：
//   - gRPC：info.FullMethod，如 /kratos.admin.v1.AdminService/UpdateAdmin
//   - HTTP：路由 pathTemplate，如 /v1/admins/{id}（{id} 占位符原样保留）
//
// HTTP 面同一 pathTemplate 会对应多个方法（GET /v1/admins/{id} 读、
// DELETE /v1/admins/{id} 注销），故 HTTP 键额外前置大写方法：`GET /v1/admins/{id}`。
// 方法从 http.RequestFromServerContext 取，gRPC 面取不到时报文键即全方法。
//
// 登记两个键各一份；未登记的 operation 落默认策略 authenticated（Q7-A 兜底，
// 只要求登录——HTTP 面由 auth.Middleware 保证，gRPC 面由 GRPCMiddleware 保证）。
//
// 中间件必须排在认证之后：HTTP 面 auth.Middleware 是 filter，先于所有
// kratos middleware 执行；gRPC 面将本中间件注册在 auth.GRPCMiddleware 之后。

// PermissionRegistry 是 operation → 权限码的登记表。init 期登记、运行期只读；
// 默认实例由 permission_codes.go 填充（P1-6 敏感域收口），proto option
// codegen 落地后由其生成的注册文件替换（键空间不变）。
type PermissionRegistry struct {
	mu sync.RWMutex
	m  map[string]string
}

func NewPermissionRegistry() *PermissionRegistry {
	return &PermissionRegistry{m: make(map[string]string)}
}

// Require 登记单个 operation 键的权限码。键重复登记且冲突时 panic（编程错误，
// 启动即暴露）。
func (r *PermissionRegistry) Require(operation, code string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if prev, ok := r.m[operation]; ok && prev != code {
		panic("permission registry conflict on " + operation + ": " + prev + " vs " + code)
	}
	r.m[operation] = code
}

// RequireRPC 同时登记某 rpc 的 gRPC 全方法与 HTTP 绑定两个键
// （httpMethod/httpPath 为空表示该 rpc 无 HTTP 绑定，仅 gRPC 可达）。
// HTTP 键为 `<大写方法> <pathTemplate>`。
func (r *PermissionRegistry) RequireRPC(grpcFullMethod, httpMethod, httpPath, code string) {
	r.Require(grpcFullMethod, code)
	if httpPath != "" {
		if httpMethod == "" {
			panic("permission registry: http path without method for " + grpcFullMethod)
		}
		r.Require(httpMethod+" "+httpPath, code)
	}
}

// HTTPKey 返回 HTTP 请求的登记键形态 `<大写方法> <pathTemplate>`。
// 中间件与登记表共用，避免两处拼写漂移。
func HTTPKey(method, pathTemplate string) string { return method + " " + pathTemplate }

// RequireAuthenticatedRPC 显式声明某 rpc 为"仅需登录"（默认策略 authenticated）。
//
// 用途：敏感域内按设计不收严的 rpc 也登记一条空码记录——登记本身即决策留痕，
// 架构测试（permission_arch_test.go）据此要求敏感域每个 rpc 都有明确归属，
// 新增 rpc 若既不登记权限码也不声明 authenticated，测试即失败。
func (r *PermissionRegistry) RequireAuthenticatedRPC(grpcFullMethod, httpMethod, httpPath string) {
	r.Require(grpcFullMethod, "")
	if httpPath != "" {
		if httpMethod == "" {
			panic("permission registry: http path without method for " + grpcFullMethod)
		}
		r.Require(HTTPKey(httpMethod, httpPath), "")
	}
}

// CodeFor 返回 operation 需要的权限码；ok=false 表示未登记（默认策略）。
func (r *PermissionRegistry) CodeFor(operation string) (code string, ok bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	code, ok = r.m[operation]
	return
}

// CoverageSnapshot 返回登记表副本（架构测试用）。
func (r *PermissionRegistry) CoverageSnapshot() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]string, len(r.m))
	for k, v := range r.m {
		out[k] = v
	}
	return out
}

// permissionDeniedThrottle 节流 403 Warn（越权探测扫描会刷屏）：10s 窗口。
type windowThrottle struct {
	mu         sync.Mutex
	interval   time.Duration
	last       time.Time
	suppressed int
}

var permissionDeniedThrottle = &windowThrottle{interval: 10 * time.Second}

func (t *windowThrottle) allow() (bool, int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	if now.Sub(t.last) < t.interval {
		t.suppressed++
		return false, 0
	}
	sup := t.suppressed
	t.suppressed = 0
	t.last = now
	return true, sup
}

// Permission 返回按注册表硬判权限码的 kratos 中间件。
//
// 判定顺序：
//  1. operation 无登记 → 放行（Q7-A 默认策略 authenticated，登录即可）；
//  2. ctx 无 Auth：dev/test 环境的 gRPC 内网直通放行（与 GRPCMiddleware 的
//     carve-out 对齐），其余情况 403（fail-closed）；
//  3. HasPermission(code) → 放行；否则 403。
func Permission(registry *PermissionRegistry, lg loggateway.Logger) middleware.Middleware {
	if registry == nil {
		registry = NewPermissionRegistry()
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			tr, ok := transport.FromServerContext(ctx)
			if !ok {
				return handler(ctx, req)
			}
			key := tr.Operation()
			// HTTP 面：同 pathTemplate 多方法需以方法区分（GET/DELETE 同路径）。
			if hreq, ok := kratoshttp.RequestFromServerContext(ctx); ok && hreq != nil && hreq.Method != "" {
				key = HTTPKey(hreq.Method, tr.Operation())
			}
			code, required := registry.CodeFor(key)
			if !required || code == "" {
				// 未登记（默认策略 authenticated）或显式声明仅需登录。
				return handler(ctx, req)
			}
			a, ok := auth.FromContext(ctx)
			if !ok || a == nil {
				// 与 GRPCMiddleware 的 dev/test 内网直通 carve-out 对齐；
				// 生产环境的 gRPC 无凭据请求在认证中间件已被拒，到不了这里。
				if auth.DevOrTestEnv() {
					return handler(ctx, req)
				}
				lg.Warn("permission denied: no principal",
					loggateway.StepID("auth.permission_denied"),
					loggateway.Str("operation", key),
					loggateway.Str("code", code))
				return nil, auth.ErrForbidden
			}
			if a.HasPermission(code) {
				return handler(ctx, req)
			}
			if ok, suppressed := permissionDeniedThrottle.allow(); ok {
				fields := []loggateway.Field{
					loggateway.StepID("auth.permission_denied"),
					loggateway.Str("operation", key),
					loggateway.Str("code", code),
					loggateway.Int64("user_id", a.UserID),
				}
				if suppressed > 0 {
					fields = append(fields, loggateway.Int("suppressed", suppressed))
				}
				lg.Warn("permission denied", fields...)
			}
			return nil, auth.ErrForbidden
		}
	}
}
