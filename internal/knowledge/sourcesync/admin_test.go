// admin_test.go —— G-38 W5 管理面编排钉阵（无 PG：biz 仓口契约内存镜）。
// 钉区（与设计 ④/⑤/⑦ 条款逐一对应）：
//
//	 ① 凭据闸：RequireKeyForSecret 硬错拒写（crypto nil→Unavailable；
//	    无 key→透传硬错）；加密失败绝不落明文。
//	 ② env: 备选 scheme：免加密闸原样落库；非法变量名硬拒。
//	 ③ 读侧 sanitize：view 永无 SecretRef 原文（configured+masked_preview）；
//	    ON CONFLICT 原位更新保原 ID。
//	 ④ 唤醒链：auth_paused PUT→idle+清 error/consecutive；重复 PUT no-op。
//	 ⑤ trigger 语义：next_run_at=now 入队；禁用/停轮/在途分级 409。
//	 ⑥ Update CAS：在途轮拒管理写 409；运行态字段零漂移。
//	 ⑦ IDOR 反枚举：跨租户 404；系统调用方（空 workspace）绕过。
//	 ⑧ 级联：connector 软删 ⇒ 凭据全软删（同事务语义，经 fakes 钩子缝合）。
//	 ⑨ Create：due 撒点区间钉 + id 前缀钉。
//	⑩ 幂等：软删凭据后同 key 再创=新行（partial-unique 活性仲裁）。
package sourcesync

import (
	"context"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	bizsync "aranea-agents/internal/biz/knowledge/sourcesync"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

type adminFixture struct {
	conns  *memConnectorRepo
	creds  *memCredentialRepo
	runs   *memRunRepo
	items  *memItemRepo
	crypto *biz.CredentialCrypto
	now    time.Time
	svc    *AdminService
}

func newAdminFixture(t *testing.T) *adminFixture {
	t.Helper()
	conns := newMemConnectorRepo()
	creds := newMemCredentialRepo()
	// 缝合级联钩子（镜像 repo 同事务级联语义）
	conns.onSoftDelete = creds.cascadeDelete
	fx := &adminFixture{
		conns: conns,
		creds: creds,
		runs:  &memRunRepo{},
		items: newMemItemRepo(),
		now:   time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC),
	}
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	fx.crypto = biz.NewCredentialCrypto(
		func(ctx context.Context) ([]byte, error) { return key, nil },
		loggateway.NewNoop(),
	)
	idSeq := 0
	fx.svc = NewAdminService(AdminDeps{
		Log:         loggateway.NewNoop(),
		Connectors:  conns,
		Credentials: creds,
		Runs:        fx.runs,
		Items:       fx.items,
		Crypto:      fx.crypto,
		Now:         func() time.Time { return fx.now },
		NewID: func(prefix string) string {
			idSeq++
			return prefix + strings.Repeat("0", 4-1) + string(rune('0'+idSeq)) + "abcd"
		},
	})
	return fx
}

func (f *adminFixture) seedConnector(t *testing.T, id, ws string, state bizsync.SyncState, runToken string) bizsync.Connector {
	t.Helper()
	c := bizsync.Connector{
		ID:                  id,
		Kind:                bizsync.KindSharedDir,
		Name:                "conn-" + id,
		WorkspaceID:         ws,
		CollectionID:        "col-" + id,
		ConfigJSON:          `{"root":"F:\\src"}`,
		IntervalSeconds:     900,
		Enabled:             true,
		SyncState:           state,
		MaxItemsPerRun:      500,
		MaxDeletesPerRun:    200,
		MaxFetchBytesPerRun: 512 << 20,
		RunToken:            runToken,
	}
	f.conns.seed(c)
	return c
}

func ptrBool(b bool) *bool    { return &b }
func strPtr(s string) *string { return &s }

// ── ① 凭据闸：RequireKeyForSecret 硬错，绝不静默明文降级 ──

func TestAdminUpsertCredential_KeyGateHardRejects(t *testing.T) {
	fx := newAdminFixture(t)
	fx.seedConnector(t, "c1", "ws-1", bizsync.SyncStateIdle, "")

	// crypto=nil → Unavailable（装配 fail-closed）
	noCrypto := NewAdminService(AdminDeps{
		Connectors:  fx.conns,
		Credentials: fx.creds,
		Log:         loggateway.NewNoop(),
		Now:         func() time.Time { return fx.now },
	})
	_, err := noCrypto.UpsertCredential(context.Background(), "c1", "ws-1",
		CredentialInput{CredentialKey: "bot_token", Secret: "plain-secret"})
	if err == nil || !apierror.IsCode(err, apierror.CodeUnavailable) {
		t.Fatalf("crypto nil must Unavailable: %v", err)
	}

	// 加密器无 key（resolver 返回 nil）→ 透传硬错，且 creds 仓口零写入
	noKey := biz.NewCredentialCrypto(
		func(ctx context.Context) ([]byte, error) { return nil, nil },
		loggateway.NewNoop(),
	)
	fxNoKey := NewAdminService(AdminDeps{
		Connectors:  fx.conns,
		Credentials: fx.creds,
		Crypto:      noKey,
		Log:         loggateway.NewNoop(),
		Now:         func() time.Time { return fx.now },
	})
	_, err = fxNoKey.UpsertCredential(context.Background(), "c1", "ws-1",
		CredentialInput{CredentialKey: "bot_token", Secret: "plain-secret"})
	if err == nil {
		t.Fatal("no-key must hard-fail")
	}
	if _, found, _ := fx.creds.GetCredential(context.Background(), "c1", "bot_token"); found {
		t.Fatal("hard-fail must not leave any row")
	}

	// 加密器 resolver 故障 → 透传硬错
	resolverDown := biz.NewCredentialCrypto(
		func(ctx context.Context) ([]byte, error) {
			return nil, apierror.Internal("CREDENTIAL_KEY", "resolver down")
		},
		loggateway.NewNoop(),
	)
	fxDown := NewAdminService(AdminDeps{
		Connectors:  fx.conns,
		Credentials: fx.creds,
		Crypto:      resolverDown,
		Log:         loggateway.NewNoop(),
		Now:         func() time.Time { return fx.now },
	})
	_, err = fxDown.UpsertCredential(context.Background(), "c1", "ws-1",
		CredentialInput{CredentialKey: "bot_token", Secret: "plain-secret"})
	if err == nil || !strings.Contains(err.Error(), "resolver down") {
		t.Fatalf("resolver error must propagate: %v", err)
	}
}

// ── ② env: 备选 scheme：免加密闸原样落库 + 非法变量名硬拒 ──

func TestAdminUpsertCredential_EnvScheme(t *testing.T) {
	fx := newAdminFixture(t)
	fx.seedConnector(t, "c1", "ws-1", bizsync.SyncStateIdle, "")

	// env: 合法 → 免加密闸原样落库（即便 crypto nil 也通）
	noCrypto := NewAdminService(AdminDeps{
		Connectors:  fx.conns,
		Credentials: fx.creds,
		Log:         loggateway.NewNoop(),
		Now:         func() time.Time { return fx.now },
	})
	v, err := noCrypto.UpsertCredential(context.Background(), "c1", "ws-1",
		CredentialInput{CredentialKey: "bot_token", Secret: "env:NOTION_TOKEN"})
	if err != nil {
		t.Fatalf("env scheme must bypass crypto gate: %v", err)
	}
	if !v.Configured {
		t.Fatal("env scheme is configured")
	}
	stored, found, _ := fx.creds.GetCredential(context.Background(), "c1", "bot_token")
	if !found || stored.SecretRef != "env:NOTION_TOKEN" {
		t.Fatalf("env ref stored verbatim: %+v", stored)
	}

	// 非法变量名 → BadRequest，且不落库
	for _, bad := range []string{"env:", "env:1BAD", "env:HAS SPACE", "env:A/B"} {
		if _, err := noCrypto.UpsertCredential(context.Background(), "c1", "ws-1",
			CredentialInput{CredentialKey: "k2", Secret: bad}); err == nil {
			t.Fatalf("illegal env ref %q must reject", bad)
		}
	}
	if _, found, _ := fx.creds.GetCredential(context.Background(), "c1", "k2"); found {
		t.Fatal("rejected env ref must not leave row")
	}
}

// ── ③ 读侧 sanitize：view 无明文 + ON CONFLICT 原位更新保原 ID ──

func TestAdminUpsertCredential_SanitizeAndConflictKeepsID(t *testing.T) {
	fx := newAdminFixture(t)
	fx.seedConnector(t, "c1", "ws-1", bizsync.SyncStateIdle, "")

	v1, err := fx.svc.UpsertCredential(context.Background(), "c1", "ws-1",
		CredentialInput{CredentialKey: "bot_token", Secret: "super-secret-value"})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if !v1.Configured {
		t.Fatal("configured")
	}
	if strings.Contains(v1.MaskedPreview, "super-secret-value") {
		t.Fatalf("masked preview must never contain secret: %q", v1.MaskedPreview)
	}
	if !strings.HasPrefix(v1.MaskedPreview, "enc:") || !strings.Contains(v1.MaskedPreview, "...") {
		t.Fatalf("masked preview shape: %q", v1.MaskedPreview)
	}
	firstID := v1.ID

	// 第二次 upsert 同 key → ON CONFLICT 原位更新，ID 保原值（库行为准）
	v2, err := fx.svc.UpsertCredential(context.Background(), "c1", "ws-1",
		CredentialInput{CredentialKey: "bot_token", Secret: "rotated-secret"})
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if v2.ID != firstID {
		t.Fatalf("ON CONFLICT must keep original row id: %q vs %q", v2.ID, firstID)
	}
	stored, _, _ := fx.creds.GetCredential(context.Background(), "c1", "bot_token")
	if strings.Contains(stored.SecretRef, "rotated-secret") {
		t.Fatal("secret_ref must be encrypted envelope, never plaintext")
	}
	if !strings.HasPrefix(stored.SecretRef, "enc:") {
		t.Fatalf("secret_ref must be enc: envelope: %q", stored.SecretRef)
	}

	// List 也只返 view（④）
	views, err := fx.svc.ListCredentials(context.Background(), "c1", "ws-1")
	if err != nil || len(views) != 1 {
		t.Fatalf("list: n=%d err=%v", len(views), err)
	}
	if views[0].MaskedPreview == "" {
		t.Fatal("list view must carry masked preview")
	}
}

// ── ④ 唤醒链：auth_paused PUT → idle；非停轮态 no-op 不报错 ──

func TestAdminUpsertCredential_WakesAuthPausedOnly(t *testing.T) {
	fx := newAdminFixture(t)
	fx.seedConnector(t, "c1", "ws-1", bizsync.SyncStateAuthPaused, "")
	// 把停轮现场写实（mem seed 默认零值——经二次写）：
	c := fx.conns.get("c1")
	c.ErrorCode = bizsync.ErrCodeAuthFailed
	c.ConsecutiveFailures = 3
	_, _ = fx.conns.UpdateConnector(context.Background(), c)

	_, err := fx.svc.UpsertCredential(context.Background(), "c1", "ws-1",
		CredentialInput{CredentialKey: "bot_token", Secret: "fresh-secret"})
	if err != nil {
		t.Fatalf("upsert during auth_paused: %v", err)
	}
	after := fx.conns.get("c1")
	if after.SyncState != bizsync.SyncStateIdle {
		t.Fatalf("woken state: %q", after.SyncState)
	}
	if after.ErrorCode != bizsync.ErrCodeNone || after.ConsecutiveFailures != 0 {
		t.Fatalf("woken clears error/consecutive: %+v", after)
	}

	// 重复 PUT（现 idle）→ no-op 不报错，状态零漂移
	_, err = fx.svc.UpsertCredential(context.Background(), "c1", "ws-1",
		CredentialInput{CredentialKey: "bot_token", Secret: "another-rotation"})
	if err != nil {
		t.Fatalf("repeat PUT on idle must not error: %v", err)
	}
	after2 := fx.conns.get("c1")
	if after2.SyncState != bizsync.SyncStateIdle {
		t.Fatalf("repeat PUT must not re-wake: %q", after2.SyncState)
	}
}

// ── ⑤ trigger 语义：入队 now + 在途/禁用/停轮分级 409 ──

func TestAdminTriggerSync_Grading(t *testing.T) {
	fx := newAdminFixture(t)

	// 正常 idle → next_run_at=now
	fx.seedConnector(t, "ok", "ws-1", bizsync.SyncStateIdle, "")
	cOK := fx.conns.get("ok")
	cOK.NextRunAt = fx.now.Add(time.Hour)
	_, _ = fx.conns.UpdateConnector(context.Background(), cOK)
	if err := fx.svc.TriggerSync(context.Background(), "ok", "ws-1"); err != nil {
		t.Fatalf("trigger idle: %v", err)
	}
	if got := fx.conns.get("ok").NextRunAt; !got.Equal(fx.now) {
		t.Fatalf("enqueue semantics next_run_at=now: got %v", got)
	}

	// 在途（run_token 非空）→ CAS 失配 409
	fx.seedConnector(t, "inflight", "ws-1", bizsync.SyncStateRunning, "ss:tok:active")
	if err := fx.svc.TriggerSync(context.Background(), "inflight", "ws-1"); err == nil ||
		!apierror.IsCode(err, apierror.CodeConflict) {
		t.Fatalf("in-flight must 409: %v", err)
	}

	// 禁用 → 409（先于 CAS 判定的分级）
	fx.seedConnector(t, "disabled", "ws-1", bizsync.SyncStateIdle, "")
	cd := fx.conns.get("disabled")
	cd.Enabled = false
	_, _ = fx.conns.UpdateConnector(context.Background(), cd)
	if err := fx.svc.TriggerSync(context.Background(), "disabled", "ws-1"); err == nil ||
		!apierror.IsCode(err, apierror.CodeConflict) {
		t.Fatalf("disabled must 409: %v", err)
	}

	// auth_paused → 409（引导用户走凭据唤醒）
	fx.seedConnector(t, "paused", "ws-1", bizsync.SyncStateAuthPaused, "")
	if err := fx.svc.TriggerSync(context.Background(), "paused", "ws-1"); err == nil ||
		!apierror.IsCode(err, apierror.CodeConflict) {
		t.Fatalf("auth_paused must 409: %v", err)
	}

	// 不存在 → 404
	if err := fx.svc.TriggerSync(context.Background(), "ghost", "ws-1"); err == nil ||
		!apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("ghost must 404: %v", err)
	}
}

// ── ⑥ Update CAS：在途轮拒管理写 409；运行态字段零漂移 ──

func TestAdminUpdateConnector_RunStateZeroDrift(t *testing.T) {
	fx := newAdminFixture(t)
	fx.seedConnector(t, "c1", "ws-1", bizsync.SyncStateError, "")
	c := fx.conns.get("c1")
	c.Cursor = "cursor-v3"
	c.NextRunAt = fx.now.Add(30 * time.Minute)
	c.ConsecutiveFailures = 4
	c.ErrorCode = bizsync.ErrCodeWalkFailed
	c.ErrorMessage = "walk exploded"
	c.RunToken = ""
	_, _ = fx.conns.UpdateConnector(context.Background(), c)

	updated, err := fx.svc.UpdateConnector(context.Background(), "c1", "ws-1", ConnectorInput{
		Kind:         "shared_dir",
		Name:         "renamed",
		CollectionID: "col-c1",
		ConfigJSON:   `{"root":"F:\\src2"}`,
		Enabled:      ptrBool(false),
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "renamed" || updated.Enabled {
		t.Fatalf("declarative fields applied: %+v", updated)
	}
	// 运行态零漂移（管理面不可改写）
	if updated.Cursor != "cursor-v3" || updated.ConsecutiveFailures != 4 ||
		updated.ErrorCode != bizsync.ErrCodeWalkFailed || updated.ErrorMessage != "walk exploded" {
		t.Fatalf("run state must not drift: %+v", updated)
	}
	if !updated.NextRunAt.Equal(c.NextRunAt) {
		t.Fatalf("next_run_at must not drift: %v vs %v", updated.NextRunAt, c.NextRunAt)
	}

	// 在途轮 → 409（读出后并发 claim 竞态）
	fx.seedConnector(t, "c2", "ws-1", bizsync.SyncStateRunning, "ss:tok:active")
	_, err = fx.svc.UpdateConnector(context.Background(), "c2", "ws-1", ConnectorInput{
		Kind: "shared_dir", Name: "n2", CollectionID: "col-c2",
	})
	if err == nil || !apierror.IsCode(err, apierror.CodeConflict) {
		t.Fatalf("in-flight update must 409: %v", err)
	}

	// workspace_id 不可变
	_, err = fx.svc.UpdateConnector(context.Background(), "c1", "ws-1", ConnectorInput{
		Kind: "shared_dir", Name: "n", CollectionID: "col-c1", WorkspaceID: "ws-other",
	})
	if err == nil || !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("workspace immutable must BadRequest: %v", err)
	}
}

// ── ⑦ IDOR 反枚举 + 系统调用方绕过 ──

func TestAdminRequireConnector_IDOR(t *testing.T) {
	fx := newAdminFixture(t)
	fx.seedConnector(t, "c1", "ws-1", bizsync.SyncStateIdle, "")

	// 跨租户 → 404（不区分不存在/越权——反枚举）
	if _, err := fx.svc.GetConnector(context.Background(), "c1", "ws-other"); err == nil ||
		!apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("cross-tenant must 404: %v", err)
	}
	// 同租户 → 200
	if _, err := fx.svc.GetConnector(context.Background(), "c1", "ws-1"); err != nil {
		t.Fatalf("same workspace must pass: %v", err)
	}
	// 系统调用方（空 workspace）→ 绕过隔离
	if _, err := fx.svc.GetConnector(context.Background(), "c1", ""); err != nil {
		t.Fatalf("system caller bypass: %v", err)
	}
}

// ── ⑧ 级联：connector 软删 ⇒ 凭据全软删（同事务语义） ──

func TestAdminDeleteConnector_CascadesCredentials(t *testing.T) {
	fx := newAdminFixture(t)
	fx.seedConnector(t, "c1", "ws-1", bizsync.SyncStateIdle, "")
	if _, err := fx.svc.UpsertCredential(context.Background(), "c1", "ws-1",
		CredentialInput{CredentialKey: "bot_token", Secret: "s1"}); err != nil {
		t.Fatalf("seed cred: %v", err)
	}
	if _, err := fx.svc.UpsertCredential(context.Background(), "c1", "ws-1",
		CredentialInput{CredentialKey: "refresh_token", Secret: "s2"}); err != nil {
		t.Fatalf("seed cred2: %v", err)
	}

	if err := fx.svc.DeleteConnector(context.Background(), "c1", "ws-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	// 删除后 require → 404
	if _, err := fx.svc.GetConnector(context.Background(), "c1", "ws-1"); err == nil {
		t.Fatal("deleted connector must 404")
	}
	// 全部凭据级联软删（直查仓口——connector 已软删，service 读径必 404 是对的）
	for _, key := range []string{"bot_token", "refresh_token"} {
		if _, found, _ := fx.creds.GetCredential(context.Background(), "c1", key); found {
			t.Fatalf("credential %q must cascade-delete with connector", key)
		}
	}
	// 重复删除（幂等删除后 get 先 404）
	if err := fx.svc.DeleteConnector(context.Background(), "c1", "ws-1"); !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("double delete must 404: %v", err)
	}
}

// ── ⑨ Create：due 撒点区间钉 + id 前缀钉 + 缺省填充 ──

func TestAdminCreateConnector_DueScatterAndDefaults(t *testing.T) {
	fx := newAdminFixture(t)
	c, err := fx.svc.CreateConnector(context.Background(), ConnectorInput{
		Kind:            "shared_dir",
		Name:            "n1",
		WorkspaceID:     "ws-1",
		CollectionID:    "col-1",
		ConfigJSON:      `{"root":"F:\\src"}`,
		IntervalSeconds: 900,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !strings.HasPrefix(c.ID, "ss:conn:") {
		t.Fatalf("id prefix: %q", c.ID)
	}
	// due ∈ [now, now+interval)
	if c.NextRunAt.Before(fx.now) || !c.NextRunAt.Before(fx.now.Add(900*time.Second)) {
		t.Fatalf("due scatter window: %v", c.NextRunAt)
	}
	if c.MaxItemsPerRun != 500 || c.MaxDeletesPerRun != 200 || c.MaxFetchBytesPerRun != 512<<20 {
		t.Fatalf("budget defaults: %+v", c)
	}
	if c.SyncState != bizsync.SyncStateIdle {
		t.Fatalf("fresh state idle: %q", c.SyncState)
	}

	// 校验闸：interval 下限
	if _, err := fx.svc.CreateConnector(context.Background(), ConnectorInput{
		Kind: "shared_dir", Name: "n2", WorkspaceID: "ws-1", CollectionID: "col-2",
		ConfigJSON: `{"root":"F:\\src"}`, IntervalSeconds: 60,
	}); err == nil || !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("interval floor must BadRequest: %v", err)
	}
	// 校验闸：shared_dir 配置形状（敏感键黑名单经 ParseSharedDirConfig）
	if _, err := fx.svc.CreateConnector(context.Background(), ConnectorInput{
		Kind: "shared_dir", Name: "n3", WorkspaceID: "ws-1", CollectionID: "col-3",
		ConfigJSON: `{"root":"F:\\src","api_key":"leak"}`, IntervalSeconds: 900,
	}); err == nil {
		t.Fatal("secret-ish config key must reject")
	}
}

// ── ⑩ 软删墓碑行不仲裁：同 key 再创=新行（partial-unique 活性） ──

func TestAdminCredential_TombstoneExemptsResurrection(t *testing.T) {
	fx := newAdminFixture(t)
	fx.seedConnector(t, "c1", "ws-1", bizsync.SyncStateIdle, "")

	v1, err := fx.svc.UpsertCredential(context.Background(), "c1", "ws-1",
		CredentialInput{CredentialKey: "bot_token", Secret: "s1"})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := fx.svc.DeleteCredential(context.Background(), "c1", "ws-1", "bot_token"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	// 软删后同 key 再创=新行（活性仲裁豁免墓碑行）
	v2, err := fx.svc.UpsertCredential(context.Background(), "c1", "ws-1",
		CredentialInput{CredentialKey: "bot_token", Secret: "s2"})
	if err != nil {
		t.Fatalf("resurrect: %v", err)
	}
	if v2.ID == v1.ID {
		t.Fatalf("resurrection must mint fresh row: %q vs %q", v2.ID, v1.ID)
	}
	// 删除不存在凭据 → 幂等 nil
	if err := fx.svc.DeleteCredential(context.Background(), "c1", "ws-1", "never_existed"); err != nil {
		t.Fatalf("delete missing must be idempotent nil: %v", err)
	}
}

// ── 附：凭证 key 形状硬闸（经 admin 入参通道） ──

func TestAdminUpsertCredential_KeyShapeGate(t *testing.T) {
	fx := newAdminFixture(t)
	fx.seedConnector(t, "c1", "ws-1", bizsync.SyncStateIdle, "")
	for _, bad := range []string{"", "  ", " pad", "has space", strings.Repeat("k", 129)} {
		if _, err := fx.svc.UpsertCredential(context.Background(), "c1", "ws-1",
			CredentialInput{CredentialKey: bad, Secret: "env:OK_VAR"}); err == nil {
			t.Fatalf("bad key %q must reject", bad)
		}
	}
}
