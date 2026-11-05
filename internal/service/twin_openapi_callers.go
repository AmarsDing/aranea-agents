// Package service — twin_openapi_callers.go
//
// 机器面多调用方身份（P0 底座安全前置，2026-11-02，三仓联动）。
// 原单一 ARANEA_TWINOPENAPI_TOKEN 无调用方维度：TwinMonitor / HotCraftFlow
// AGT / NovaTrader harness 共用一把钥匙，审计无法归因、租户无法隔离。
// 本文件引入 caller 注册表：token → {name, workspace_id}，
// guard 认证后把 caller 放入请求上下文，各 handler 据此做租户门禁。
//
// 配置形状（两个环境变量可叠加）：
//
//	ARANEA_TWINOPENAPI_TOKEN    存量单 token（兼容面）→ caller "twinmonitor"，
//	                            workspace_id 为空 = 系统级（全量可见，行为
//	                            与引入 caller 前字节级一致）。
//	ARANEA_TWINOPENAPI_CALLERS  JSON 数组，元素
//	                            {"name":"hotcraftflow","token":"...","workspace_id":"..."}
//	                            name 唯一、token 必填、workspace_id 可空
//	                            （空 = 系统级可信调用方）。
//
// 门禁语义（对齐 P2-B 租户模型）：
//   - 系统级 caller（workspace_id 空）：全量可见、全部可写；
//   - 租户 caller：读面 = 共享（workspace_id 空）+ 自有；写面（graph 更新、
//     run 提交/取消/恢复、agent 更新）= 仅自有；创建面强制归属自有。
package service

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"os"
	"strings"

	"aranea-agents/pkg/loggateway"
)

// TwinOpenAPICallersEnv 多调用方 token 注册表的环境变量名（JSON 数组）。
const TwinOpenAPICallersEnv = "ARANEA_TWINOPENAPI_CALLERS"

// twinLegacyCallerName 存量单 token（ARANEA_TWINOPENAPI_TOKEN）的 caller 名。
// 沿用 "twinmonitor" 使记忆 SourceKind、审计字段与历史数据一致。
const twinLegacyCallerName = "twinmonitor"

// twinCaller 机器面调用方身份。
type twinCaller struct {
	// Name 审计名（日志、记忆 SourceKind、幂等键空间）。
	Name string `json:"name"`
	// Token 机器 Bearer（常量时间比对）。
	Token string `json:"token"`
	// WorkspaceID 归属租户；空 = 系统级（全量可见，兼容存量 twinmonitor）。
	WorkspaceID string `json:"workspace_id"`
}

// isSystem 报告是否系统级调用方（无租户过滤）。
func (c *twinCaller) isSystem() bool { return c == nil || c.WorkspaceID == "" }

// loadTwinCallers 从环境加载 caller 注册表：CALLERS JSON + 存量单 token。
// 解析失败的条目记 warn 跳过（部分可用优于整体拒绝）；重名/空 token 条目同。
func loadTwinCallers(lg loggateway.Logger) []*twinCaller {
	var out []*twinCaller
	seen := map[string]bool{}
	if raw := strings.TrimSpace(os.Getenv(TwinOpenAPICallersEnv)); raw != "" {
		var list []twinCaller
		if err := json.Unmarshal([]byte(raw), &list); err != nil {
			lg.Warn("twinopenapi callers JSON 解析失败，整段忽略",
				loggateway.StepID("twinopenapi.callers_parse_fail"), loggateway.Err(err))
		}
		for i := range list {
			c := &list[i]
			c.Name = strings.TrimSpace(c.Name)
			c.Token = strings.TrimSpace(c.Token)
			c.WorkspaceID = strings.TrimSpace(c.WorkspaceID)
			if c.Name == "" || c.Token == "" || seen[c.Name] {
				lg.Warn("twinopenapi callers 条目非法（空 name/token 或重名），跳过",
					loggateway.StepID("twinopenapi.callers_skip"),
					loggateway.Str("name", c.Name))
				continue
			}
			seen[c.Name] = true
			out = append(out, c)
		}
	}
	// 存量单 token 兼容面：未显式定义同名 caller 时注册为系统级。
	if tok := strings.TrimSpace(os.Getenv(TwinOpenAPITokenEnv)); tok != "" && !seen[twinLegacyCallerName] {
		out = append(out, &twinCaller{Name: twinLegacyCallerName, Token: tok})
	}
	return out
}

// authenticate 以常量时间比对在注册表内匹配 token；未命中返回 nil。
// 全表扫描（不提前 break），避免 token 位置侧信道。
func (s *TwinOpenAPICompatService) authenticate(token string) *twinCaller {
	var matched *twinCaller
	for _, c := range s.callerList() {
		if subtle.ConstantTimeCompare([]byte(token), []byte(c.Token)) == 1 {
			matched = c
		}
	}
	return matched
}

// twinCallerCtxKey 请求上下文内的 caller 键。
type twinCallerCtxKey struct{}

// withTwinCaller 把认证后的 caller 写入请求上下文。
func withTwinCaller(ctx context.Context, c *twinCaller) context.Context {
	return context.WithValue(ctx, twinCallerCtxKey{}, c)
}

// twinCallerFrom 取请求 caller；guard 保证非空（兜底按系统级处理，不 panic）。
func twinCallerFrom(ctx context.Context) *twinCaller {
	if c, ok := ctx.Value(twinCallerCtxKey{}).(*twinCaller); ok && c != nil {
		return c
	}
	return &twinCaller{Name: "unknown"}
}
