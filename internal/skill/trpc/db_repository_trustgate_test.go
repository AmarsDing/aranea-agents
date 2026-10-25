package trpc

// D4-c Get 坐席钉板（就绪不翻）：信任门禁 on 档在正文加载坐席死拒低信
// 任技能——候选集不剔、L1 概览保留、skillNotFoundError 语义保持框架上
// 游既有 notfound 处理路径；门禁未安装（off/shadow 缺省）恒放行（零行
// 为变化）。坐席钉在 body 快取命中之前：防翻牌后存量快取旁路新增真拒。

import (
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
)

const (
	tgBlockedSlug = "zip-demo"
	tgPassSlug    = "builtin-demo"
)

// newTrustGateAdapter 构造无需 DB 的坐席夹具：快照预置两条目（zip_import
// 默认 T4、builtin 默认 T3），loaded 置新使 refreshIfStale 不触发后台
// reload，body 快取预置使放行路径零查询返回。
func newTrustGateAdapter(t *testing.T, withEnforcer bool) *DBRepositoryAdapter {
	t.Helper()
	uc := newStubUsecase(&stubSkillRepo{})
	if withEnforcer {
		uc.SetTrustGateEnforcer(biz.NewTrustGateEnforcer(
			loggateway.NewNoop(),
			biz.TrustGateModeOn,
			biz.SkillLoadLayerL2Instruction,
			biz.RequiredTrustForLayer(biz.SkillLoadLayerL2Instruction),
		))
	}
	r := NewDBRepositoryAdapter(uc, time.Hour, loggateway.NewNoop())
	r.entries = []dbSkillEntry{
		{summary: trpcskill.Summary{Name: tgBlockedSlug}, slug: tgBlockedSlug, skillID: "skill_zip", source: "zip_import"},
		{summary: trpcskill.Summary{Name: tgPassSlug}, slug: tgPassSlug, skillID: "skill_builtin", source: "builtin"},
	}
	r.index = map[string]int{tgBlockedSlug: 0, tgPassSlug: 1}
	r.loaded = time.Now()
	r.caches.bodies.Store(tgBlockedSlug, &trpcskill.Skill{Summary: trpcskill.Summary{Name: tgBlockedSlug}, Body: "zip body"})
	r.caches.bodies.Store(tgPassSlug, &trpcskill.Skill{Summary: trpcskill.Summary{Name: tgPassSlug}, Body: "builtin body"})
	return r
}

// TestGet_TrustGateBlocksUntrustedEvenWhenCached：on 档在快取命中之前死拒
// zip 默认 T4——即使 body 快取已备好（翻牌前门缓存的 body 不得旁路新拒）。
func TestGet_TrustGateBlocksUntrustedEvenWhenCached(t *testing.T) {
	r := newTrustGateAdapter(t, true)
	sk, err := r.Get(tgBlockedSlug)
	if err == nil || sk != nil {
		t.Fatalf("blocked skill must not load, got sk=%v err=%v", sk, err)
	}
	if _, ok := err.(*skillNotFoundError); !ok {
		t.Fatalf("blocked path must surface skillNotFoundError semantics, got %T: %v", err, err)
	}
}

// TestGet_TrustGatePassesTrustedFromCache：on 档对达标候选照常走快取
// 放行——快取语义对受信技能零回归。
func TestGet_TrustGatePassesTrustedFromCache(t *testing.T) {
	r := newTrustGateAdapter(t, true)
	sk, err := r.Get(tgPassSlug)
	if err != nil || sk == nil || sk.Body != "builtin body" {
		t.Fatalf("trusted skill must pass through cache, got sk=%+v err=%v", sk, err)
	}
}

// TestGet_TrustGateNotInstalledIsZeroChange：门禁未安装（off/shadow 缺
// 省，enforcer 从未 Set）对低信任候选恒放行——就绪不翻的零行为变化钉。
func TestGet_TrustGateNotInstalledIsZeroChange(t *testing.T) {
	r := newTrustGateAdapter(t, false)
	sk, err := r.Get(tgBlockedSlug)
	if err != nil || sk == nil || sk.Body != "zip body" {
		t.Fatalf("gate-not-installed must serve cached body unchanged, got sk=%+v err=%v", sk, err)
	}
}

// TestSummaries_TrustGateKeepsL1Untouched：候选集不剔、L1 概览保留 T4
// （on 档也不藏低信任技能——可见性归 visibility 过滤，门禁只守正文坐席）。
func TestSummaries_TrustGateKeepsL1Untouched(t *testing.T) {
	r := newTrustGateAdapter(t, true)
	sums := r.Summaries()
	if len(sums) != 2 {
		t.Fatalf("L1 overview must keep both entries, got %d", len(sums))
	}
	found := false
	for _, s := range sums {
		if s.Name == tgBlockedSlug {
			found = true
		}
	}
	if !found {
		t.Fatalf("blocked skill must stay visible in L1 summaries, got %+v", sums)
	}
}

// TestGet_UnknownSlugStillNotFound：非快照 slug 在坐席之前就按既有
// notfound 语义返回，门禁不动常规路径。
func TestGet_UnknownSlugStillNotFound(t *testing.T) {
	r := newTrustGateAdapter(t, true)
	if _, err := r.Get("never-existed"); err == nil {
		t.Fatal("unknown slug must stay not-found")
	} else if _, ok := err.(*skillNotFoundError); !ok {
		t.Fatalf("unknown slug error type = %T, want *skillNotFoundError", err)
	}
}
