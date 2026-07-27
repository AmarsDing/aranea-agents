// Package biz_test — Skill 进化能力功能测试环境（真实 PG 隔离 schema + 真实
// repos + 可编程 fake LLM/Gate/Registrar）。
//
// 覆盖四条功能链路：
//
//	T1 自我进化：健康度触发 → CuratorFlow → LLM draft → 审批 → Reload 新版本
//	T2 对话内容创建：observations → patterns → proposal → 注册 SKILL.md
//	T3 去重：patternHash 去重 / SkillExists 预检 / DetectDuplicateGroups
//	T4 融合提升：rule_fuse 融合草稿 + 真实 merge 两 skill 合一
package biz_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/platformskill"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// ── 环境 fixture ─────────────────────────────────────────────────────────────

type skillFuncEnv struct {
	d      *data.Data
	client *ent.Client
	lg     loggateway.Logger
}

// newSkillFuncEnv 构建隔离 PG schema，应用 Ent auto-migration + 非 Ent 表
// （learning_loop / unified_evolution）的 DDL 迁移，返回真实 *Data。
func newSkillFuncEnv(t *testing.T) *skillFuncEnv {
	t.Helper()
	client, db := testhelper.SetupTestPG(t)
	d := &data.Data{}
	d.SetEntClientForTest(client, db, loggateway.NewNoop())
	ctx := context.Background()
	if err := data.EnsureLearningLoopSchema(ctx, client); err != nil {
		t.Fatalf("ensure learning loop schema: %v", err)
	}
	if err := data.EnsureUnifiedEvolutionSchema(ctx, client); err != nil {
		t.Fatalf("ensure unified evolution schema: %v", err)
	}
	return &skillFuncEnv{d: d, client: client, lg: loggateway.NewNoop()}
}

func nowRFC3339() string { return time.Now().UTC().Format(time.RFC3339) }

// ── 种子数据 helpers ─────────────────────────────────────────────────────────

func seedAgent(t *testing.T, env *skillFuncEnv, id string) {
	t.Helper()
	_, err := env.client.Agent.Create().
		SetID(id).
		SetAgentKey("key_" + id).
		SetDisplayName("FuncTest Agent " + id).
		SetProvider("fake").
		SetModel("fake-model").
		SetCreatedAt(nowRFC3339()).
		SetUpdatedAt(nowRFC3339()).
		Save(context.Background())
	if err != nil {
		t.Fatalf("seed agent %s: %v", id, err)
	}
}

// seedSkill 写入 platform_skill（enabled, active）+ 一条 published skill_version。
func seedSkill(t *testing.T, env *skillFuncEnv, id, key, name, desc, body string, tags []string) (skillID, versionID string) {
	t.Helper()
	tagObjs := make([]map[string]string, 0, len(tags))
	for _, tag := range tags {
		tagObjs = append(tagObjs, map[string]string{"name": tag, "source": "user"})
	}
	metaJSON, _ := json.Marshal(map[string]any{"tags": tagObjs})
	if _, err := env.client.PlatformSkill.Create().
		SetID(id).
		SetSkillKey(key).
		SetName(name).
		SetDescription(desc).
		SetStatus("active").
		SetEnabled(true).
		SetConfigJSON("{}").
		SetMetadataJSON(string(metaJSON)).
		SetCreatedAt(nowRFC3339()).
		SetUpdatedAt(nowRFC3339()).
		Save(context.Background()); err != nil {
		t.Fatalf("seed skill %s: %v", id, err)
	}
	versionID = fmt.Sprintf("ver_%s", id)
	// 已发布版本理应早于后续进化/融合产生的新版本；created_at 为秒级 TEXT，
	// 回拨 2 分钟避免与新版本同秒并列。
	oldAt := time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339)
	if _, err := env.client.SkillVersion.Create().
		SetID(versionID).
		SetSkillID(id).
		SetVersion("1.0.0").
		SetContentMarkdown(body).
		SetStatus("published").
		SetCreatedAt(oldAt).
		SetUpdatedAt(oldAt).
		Save(context.Background()); err != nil {
		t.Fatalf("seed skill version %s: %v", id, err)
	}
	return id, versionID
}

func seedInvocation(t *testing.T, env *skillFuncEnv, id, skillID, outcome, status string, durationMS int, at time.Time) {
	t.Helper()
	atStr := at.UTC().Format(time.RFC3339)
	if _, err := env.client.SkillInvocation.Create().
		SetID(id).
		SetSkillID(skillID).
		SetOutcome(outcome).
		SetStatus(status).
		SetDurationMs(durationMS).
		SetCreatedAt(atStr).
		SetStartedAt(atStr).
		Save(context.Background()); err != nil {
		t.Fatalf("seed invocation %s: %v", id, err)
	}
}

// seedFailingInvocations 写入 n 条最近失败的调用记录（触发健康度进化）。
func seedFailingInvocations(t *testing.T, env *skillFuncEnv, skillID string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		seedInvocation(t, env, fmt.Sprintf("inv_%s_%d", skillID, i), skillID,
			"failure", "failed", 120+i, time.Now().UTC().Add(-time.Duration(i)*time.Minute))
	}
}

// ── 可编程 fakes ─────────────────────────────────────────────────────────────

// fakeLLMCaller 实现 biz.LLMCaller：记录全部请求，按 handler 返回编程响应。
type fakeLLMCaller struct {
	mu       sync.Mutex
	requests []biz.LLMCallRequest
	handler  func(req biz.LLMCallRequest) (string, error)
}

func (f *fakeLLMCaller) Call(_ context.Context, req biz.LLMCallRequest) (string, int, error) {
	f.mu.Lock()
	f.requests = append(f.requests, req)
	f.mu.Unlock()
	if f.handler == nil {
		return "", 0, fmt.Errorf("fakeLLMCaller: no handler configured")
	}
	text, err := f.handler(req)
	return text, len(text) / 4, err
}

func (f *fakeLLMCaller) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

// passGate 实现 biz.SkillGateVerifier：全部检查通过。
type passGate struct{}

func (passGate) Verify(_ context.Context, _ string, _ string, _ *biz.EvolutionObservationReport) (*biz.GateVerificationResult, error) {
	return &biz.GateVerificationResult{
		Passed: true,
		Checks: []biz.GateCheckResult{{Name: "fake_gate", Passed: true, Reason: "functional test pass-through"}},
	}, nil
}

// dbSkillRegistrar 实现 biz.SkillRegistrationPort，镜像
// service.skillsButlerRegistrationAdapter 语义（slug=name，注册即创建
// platform_skill + published version），但直接落真实 DB。
type dbSkillRegistrar struct {
	client *ent.Client
}

func (r *dbSkillRegistrar) RegisterSkill(_ context.Context, _ string, name string, skillMD string) error {
	ctx := context.Background()
	id := fmt.Sprintf("skill_%d", time.Now().UTC().UnixNano())
	if _, err := r.client.PlatformSkill.Create().
		SetID(id).
		SetSkillKey(name).
		SetName(name).
		SetStatus("active").
		SetEnabled(true).
		SetConfigJSON("{}").
		SetMetadataJSON("{}").
		SetCreatedAt(nowRFC3339()).
		SetUpdatedAt(nowRFC3339()).
		Save(ctx); err != nil {
		return err
	}
	_, err := r.client.SkillVersion.Create().
		SetID(fmt.Sprintf("skillver_%d", time.Now().UTC().UnixNano())).
		SetSkillID(id).
		SetVersion("1.0.0").
		SetContentMarkdown(skillMD).
		SetStatus("published").
		SetCreatedAt(nowRFC3339()).
		SetUpdatedAt(nowRFC3339()).
		Save(ctx)
	return err
}

func (r *dbSkillRegistrar) SkillExists(_ context.Context, _ string, name string) (bool, error) {
	return r.client.PlatformSkill.Query().
		Where(platformskill.SkillKeyEQ(name), platformskill.DeletedAtEQ("")).
		Exist(context.Background())
}
