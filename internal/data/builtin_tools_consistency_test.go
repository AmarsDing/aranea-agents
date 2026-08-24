package data

import (
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

// 工具元数据一致性 fitness 守卫（2026-08-24 P1-2②）。
//
// 工具元数据分置两包、各自演进，漂移即静默故障：
//   - biz 侧策略表：toolGroups / toolProfiles / registryOptInOnlyKeys（agent_tool_policy.go）
//   - data 侧目录种子：builtinPlatformToolSeeds / builtinCLIAdminToolSeeds
//
// 历史事故模式：
//   - cli_admin_* 种子 enabled=false 但未入 opt-in 表 → applyRegistryAdminDenials
//     全员硬 deny，__system_admin__ 都装不上（M74 修复）；
//   - read_image 被组表引用但无 factory/种子，空挂至 2026-08-24 P5 才清除。
//
// 本测试把跨表契约编码为断言：任何一侧增删工具而另一侧未同步，测试即失败，
// 强迫显式决策而非静默漂移。

// builtinSeedKeySet 汇总全部内建工具种子键（平台种子 + cli_admin 种子），
// 作为跨表一致性断言的目录侧基准。
func builtinSeedKeySet() map[string]bool {
	keys := make(map[string]bool, len(builtinPlatformToolSeeds)+len(builtinCLIAdminToolSeeds))
	for _, s := range builtinPlatformToolSeeds {
		if keys[s.key] {
			continue
		}
		keys[s.key] = true
	}
	for _, s := range builtinCLIAdminToolSeeds {
		keys[s.key] = true
	}
	return keys
}

// I1：静态组表每个成员必须有对应种子行——否则 profile 经 group 授予的是
// 不存在的工具，effective tools 静默缺失（组表打错字/种子漏加都落此列）。
func TestToolMetadataConsistency_GroupMembersSeeded(t *testing.T) {
	seeds := builtinSeedKeySet()
	tables := biz.ExportToolPolicyStaticTables()
	if len(tables.Groups) == 0 {
		t.Fatal("policy static groups must not be empty")
	}
	for group, members := range tables.Groups {
		if len(members) == 0 {
			t.Errorf("group %q is empty", group)
		}
		for _, m := range members {
			if !seeds[m] {
				t.Errorf("group %q member %q has no builtin seed row — profiles granting group:%s reference a nonexistent tool", group, m, group)
			}
		}
	}
}

// I2：profile 显式命名的工具必须有种子行；group: 引用必须指向已知组。
// profile 条目不做 NormalizeToolPolicyKey 归一（profileAllowSet 原始串查表），
// 必须与种子 key 逐字符一致。
func TestToolMetadataConsistency_ProfileEntriesSeeded(t *testing.T) {
	seeds := builtinSeedKeySet()
	tables := biz.ExportToolPolicyStaticTables()
	if len(tables.Profiles) == 0 {
		t.Fatal("policy profiles must not be empty")
	}
	for profile, entries := range tables.Profiles {
		for _, e := range entries {
			if strings.HasPrefix(e, "group:") {
				gn := strings.TrimPrefix(e, "group:")
				if _, ok := tables.Groups[gn]; !ok && gn != "cli_admin" {
					t.Errorf("profile %q references unknown group %q", profile, gn)
				}
				continue
			}
			if !seeds[e] {
				t.Errorf("profile %q names tool %q with no builtin seed row — grant is silently void", profile, e)
			}
		}
	}
}

// I3：opt-in 表每个键必须有种子行——无种子的 opt-in 条目是死配置，
// 且通常意味着种子漏加（工具永远装配不上）。
func TestToolMetadataConsistency_OptInKeysSeeded(t *testing.T) {
	seeds := builtinSeedKeySet()
	for key := range biz.ExportToolPolicyStaticTables().OptInOnly {
		if !seeds[key] {
			t.Errorf("registryOptInOnlyKeys entry %q has no builtin seed row — dead opt-in, tool can never be assembled", key)
		}
	}
}

// overrideOnlyDisabledSeeds 登记「种子 enabled=false 且刻意不入 opt-in 表」的工具：
// 其启用路径是 per-agent override（tool_agent_overrides mode=allow），而非
// profile/allow JSON。新增 enabled=false 种子必须二选一（入 opt-in 表 / 登记本表
// 并注明设计依据），否则 applyRegistryAdminDenials 对全员硬 deny、工具永不装配。
var overrideOnlyDisabledSeeds = map[string]string{
	"knowledge_reflect":    "跨库反思：集成面默认收敛，按 agent 逐个 override 授予",
	"client_open_app":      "74-voice-companion §6：桌面客户端桥，per-agent override 授予",
	"client_open_url":      "74-voice-companion §6：同上",
	"coding_dispatch_task": "76-coding-agent-bridge §13：外部编程 CLI 桥，per-agent override 授予",
	"coding_check_task":    "76-coding-agent-bridge §13：同上",
	"coding_cancel_task":   "76-coding-agent-bridge §13：同上",
}

// I4：enabled=false 的种子必须存在至少一条启用路径（opt-in 表 或 登记的
// override-only 豁免）；豁免表条目也必须是真实存在的种子，防止豁免表腐化。
func TestToolMetadataConsistency_DisabledSeedsHaveEnablePath(t *testing.T) {
	seeds := builtinSeedKeySet()
	optIn := biz.ExportToolPolicyStaticTables().OptInOnly

	check := func(key string, enabled bool) {
		if enabled || optIn[key] {
			return
		}
		if _, ok := overrideOnlyDisabledSeeds[key]; ok {
			return
		}
		t.Errorf("seed %q is enabled=false but in neither registryOptInOnlyKeys nor overrideOnlyDisabledSeeds — hard-denied for every agent, no enable path", key)
	}
	for _, s := range builtinPlatformToolSeeds {
		check(s.key, s.enabled)
	}
	// cli_admin 种子统一 enabled=FALSE（SeedBuiltinCLIAdminTools INSERT 字面量）。
	for _, s := range builtinCLIAdminToolSeeds {
		check(s.key, false)
	}
	for key := range overrideOnlyDisabledSeeds {
		if !seeds[key] {
			t.Errorf("overrideOnlyDisabledSeeds entry %q is not a builtin seed — stale exemption", key)
		}
	}
}
