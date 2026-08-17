package agent

import (
	"reflect"
	"testing"

	"aranea-agents/internal/biz"
)

func TestComputeSkillVersionHash_Empty(t *testing.T) {
	if got := ComputeSkillVersionHash(nil); got != "" {
		t.Fatalf("empty refs must yield empty hash, got %q", got)
	}
}

func TestComputeSkillVersionHash_OrderIndependent(t *testing.T) {
	a := []biz.SkillEnabledRef{
		{Slug: "alpha", UpdatedAt: "2026-08-01T00:00:00Z"},
		{Slug: "beta", UpdatedAt: "2026-08-02T00:00:00Z"},
	}
	b := []biz.SkillEnabledRef{a[1], a[0]}
	if ComputeSkillVersionHash(a) != ComputeSkillVersionHash(b) {
		t.Fatal("hash must be order-independent")
	}
}

func TestComputeSkillVersionHash_ContentChangeBumpsHash(t *testing.T) {
	before := []biz.SkillEnabledRef{
		{Slug: "alpha", UpdatedAt: "2026-08-01T00:00:00Z"},
		{Slug: "beta", UpdatedAt: "2026-08-02T00:00:00Z"},
	}
	after := []biz.SkillEnabledRef{
		{Slug: "alpha", UpdatedAt: "2026-08-01T00:00:00Z"},
		{Slug: "beta", UpdatedAt: "2026-08-03T00:00:00Z"}, // content mutated
	}
	if ComputeSkillVersionHash(before) == ComputeSkillVersionHash(after) {
		t.Fatal("hash must change when a skill's content version changes")
	}
}

func TestComputeSkillVersionHash_StableWhenUnchanged(t *testing.T) {
	refs := []biz.SkillEnabledRef{
		{Slug: "alpha", UpdatedAt: "2026-08-01T00:00:00Z"},
		{Slug: "beta", UpdatedAt: "2026-08-02T00:00:00Z"},
	}
	if ComputeSkillVersionHash(refs) != ComputeSkillVersionHash(refs) {
		t.Fatal("hash must be stable across calls when nothing changed")
	}
}

// --- MCPVersionHash 守卫：P0-2B 摘除 MCP CRUD 全量失效后，本哈希是唯一的
// 惰性失效依据，所有构建期消费字段必须折入（见 ComputeMCPVersionHash 契约注释）。

func TestComputeMCPVersionHash_Empty(t *testing.T) {
	if got := ComputeMCPVersionHash(nil); got != "" {
		t.Fatalf("empty servers must yield empty hash, got %q", got)
	}
}

func TestComputeMCPVersionHash_ConfigChangeBumpsHash(t *testing.T) {
	before := []biz.EffectiveMCPServer{{ID: "s1", ServerKey: "shardprobe", ConfigJSON: `{"timeout_sec":61}`}}
	after := []biz.EffectiveMCPServer{{ID: "s1", ServerKey: "shardprobe", ConfigJSON: `{"timeout_sec":62}`}}
	if ComputeMCPVersionHash(before) == ComputeMCPVersionHash(after) {
		t.Fatal("config change must bump hash")
	}
}

func TestComputeMCPVersionHash_ServerKeyRenameBumpsHash(t *testing.T) {
	// server_key 是构建期消费字段（MCP ToolSet 名 = key，前缀全部工具运行时名），
	// 但不在 ConfigJSON 内——改名必须换哈希，否则缓存构建将永远服务旧工具名。
	before := []biz.EffectiveMCPServer{{ID: "s1", ServerKey: "old_key", ConfigJSON: `{"timeout_sec":61}`}}
	after := []biz.EffectiveMCPServer{{ID: "s1", ServerKey: "new_key", ConfigJSON: `{"timeout_sec":61}`}}
	if ComputeMCPVersionHash(before) == ComputeMCPVersionHash(after) {
		t.Fatal("server_key rename must bump hash (key is build-effective but outside ConfigJSON)")
	}
}

func TestComputeMCPVersionHash_MembershipChangeBumpsHash(t *testing.T) {
	one := []biz.EffectiveMCPServer{{ID: "s1", ServerKey: "a", ConfigJSON: `{}`}}
	two := []biz.EffectiveMCPServer{one[0], {ID: "s2", ServerKey: "b", ConfigJSON: `{}`}}
	if ComputeMCPVersionHash(one) == ComputeMCPVersionHash(two) {
		t.Fatal("server set membership change (create/delete/enable/disable) must bump hash")
	}
}

func TestComputeMCPVersionHash_OrderIndependent(t *testing.T) {
	a := []biz.EffectiveMCPServer{
		{ID: "s1", ServerKey: "a", ConfigJSON: `{"x":1}`},
		{ID: "s2", ServerKey: "b", ConfigJSON: `{"x":2}`},
	}
	b := []biz.EffectiveMCPServer{a[1], a[0]}
	if ComputeMCPVersionHash(a) != ComputeMCPVersionHash(b) {
		t.Fatal("hash must be order-independent")
	}
}

func TestEffectiveMCPServerFieldsAllHashCovered(t *testing.T) {
	// 反射守卫（沿用 settings_shard_classify 思路）：EffectiveMCPServer 新增字段时
	// 本测试强制评审——构建期消费字段必须折入 ComputeMCPVersionHash；
	// 确属调用期注入的字段（如用户凭证）才允许登记例外。
	buildEffective := map[string]bool{"ID": true, "ServerKey": true, "ConfigJSON": true}
	typ := reflect.TypeOf(biz.EffectiveMCPServer{})
	for i := 0; i < typ.NumField(); i++ {
		if name := typ.Field(i).Name; !buildEffective[name] {
			t.Fatalf("EffectiveMCPServer.%s 未在 MCPVersionHash 守卫白名单中：构建期消费字段必须折入哈希", name)
		}
	}
}
