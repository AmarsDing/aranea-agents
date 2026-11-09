package middleware

// 本文件是 P1-6 敏感域权限码登记表的装配外壳（Q7-A 拍板：默认登录可见 +
// 敏感域逐项收严）。登记条目本身由生成器产出——permission_registry_gen.go
// 来自 api/kratos/<域>/v1/*.proto 各 rpc 的 (aranea.auth.v1.requirement)
// 选项（protoc-gen-aranea-auth，生成期强制敏感域全注解），禁手工维护
// （AGENTS.md §3）；重新生成：make api。
//
// 原始码位分配规则（P0 详设《02-存量rpc权限普查表》§2 敏感域清单 +
// 2026-11-08 全量 rpc 普查 43 service / 546 rpc）已转录为 proto 选项：
//   - admin：Login/Logout/Current public 豁免（登录前/自助路径）；管理面 user.*。
//   - computeruse / sandbox：整域 tool.exec（远控执行面，收严）。
//   - tool：部分收严——执行（TestTool）tool.exec、调用审计 audit.read，
//     其余 25 个 rpc 显式 authenticated（工具是用户自服务域，Q7-A 不加码）。
//   - monitor：ListAuditLogs → audit.read（审计面）；其余 → system.monitor。
//   - usage：读 → usage.read；写（配额/预算/录入/清理）→ usage.write；
//     导出 → usage.export。
//   - llm_provider_model：管理面 model.route；密钥揭示 → secops.policy（secadmin 专有）。
//   - ceoboard：view 面（查询）= ceoboard.view；ops 面（死信/审计）= ceoboard.ops；
//     入站 webhook 不走本域 rpc：共享密钥入口直接挂 HTTP server（基础设施层）。
//
// 登记键：gRPC 全方法 `/kratos.<域>.v1.<Svc>/<Method>` +
// HTTP 键 `<大写方法> <pathTemplate>`（同 pathTemplate 多方法，必须带方法区分）。
// 由架构测试 permission_arch_test.go 校验敏感域全覆盖 + 无死键 + public
// 豁免集与 proto 标注一致。
//
// 角色持有关系见 pkg/auth/permissions.go（P0 详设 01 §3 内置角色映射）。

var defaultPermissionRegistry = func() *PermissionRegistry {
	r := NewPermissionRegistry()
	registerGeneratedPermissions(r)
	return r
}()

// DefaultPermissionRegistry 返回进程级默认登记表（init 期填充，运行期只读）。
// HTTP/gRPC server 共用。
func DefaultPermissionRegistry() *PermissionRegistry { return defaultPermissionRegistry }

// GeneratedPublicRPCs 返回生成器登记的 public 豁免 rpc 清单副本（架构测试用，
// 与 exemptRPC 双向断言防漂移）。
func GeneratedPublicRPCs() []string {
	return append([]string(nil), generatedPublicRPCs...)
}

// requireRPC 登记辅助（生成文件的目标 API）：domain=proto 包中段（如 admin），
// svc=service 名，method=rpc 名，httpMethod/httpPath=HTTP 绑定（无绑定传空串），
// code=权限码。
func requireRPC(r *PermissionRegistry, domain, svc, rpc, httpMethod, httpPath, code string) {
	r.RequireRPC("/kratos."+domain+".v1."+svc+"/"+rpc, httpMethod, httpPath, code)
}

// requireAuthenticatedRPC 显式声明某 rpc 仅需登录（默认策略），见 RequireAuthenticatedRPC。
func requireAuthenticatedRPC(r *PermissionRegistry, domain, svc, rpc, httpMethod, httpPath string) {
	r.RequireAuthenticatedRPC("/kratos."+domain+".v1."+svc+"/"+rpc, httpMethod, httpPath)
}
