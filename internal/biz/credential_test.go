package biz

// E 阶段凭据托管（W1a）核心断言：
//   - 明文三不进：secret 明文不进日志、不进 API 响应（业务视图无该字段）、
//     不进审计行；落库仅 AES-GCM 密文且可解密还原（执行面单点）。
//   - 策略裁决：高敏站点默认拒绝（denied_sensitivity）、无凭据拒绝
//     （denied_no_credential）、正常放行（allowed）；每次裁决（含拒绝）落审计。

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// credentialFakeRepo 是 CredentialRepo 的内存实现，供策略/断言测试使用。
type credentialFakeRepo struct {
	mu         sync.Mutex
	nextID     int64
	rows       map[int64]Credential
	secrets    map[int64]string // id -> secret_enc
	executions []CredentialExecution
}

func newCredentialFakeRepo() *credentialFakeRepo {
	return &credentialFakeRepo{rows: map[int64]Credential{}, secrets: map[int64]string{}}
}

func (r *credentialFakeRepo) ListCredentials(_ context.Context, workspaceID string, q CredentialListQuery) ([]Credential, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Credential
	for _, c := range r.rows {
		if c.WorkspaceID != workspaceID {
			continue
		}
		if q.Site != "" && c.Site != q.Site {
			continue
		}
		out = append(out, c)
	}
	return out, int64(len(out)), nil
}

func (r *credentialFakeRepo) GetCredential(_ context.Context, workspaceID string, id int64) (Credential, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.rows[id]
	if !ok || c.WorkspaceID != workspaceID {
		return Credential{}, fmt.Errorf("not found")
	}
	return c, nil
}

func (r *credentialFakeRepo) GetCredentialSecret(_ context.Context, workspaceID string, id int64) (Credential, string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.rows[id]
	if !ok || c.WorkspaceID != workspaceID {
		return Credential{}, "", fmt.Errorf("not found")
	}
	return c, r.secrets[id], nil
}

func (r *credentialFakeRepo) CreateCredential(_ context.Context, c Credential, secretEnc string) (Credential, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	c.ID = r.nextID
	r.rows[c.ID] = c
	r.secrets[c.ID] = secretEnc
	return c, nil
}

func (r *credentialFakeRepo) UpdateCredential(_ context.Context, workspaceID string, id int64, in CredentialUpdateInput, secretEnc *string) (Credential, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.rows[id]
	if !ok || c.WorkspaceID != workspaceID {
		return Credential{}, fmt.Errorf("not found")
	}
	if in.Name != nil {
		c.Name = *in.Name
	}
	if in.URL != nil {
		c.URL = *in.URL
	}
	if in.Username != nil {
		c.Username = *in.Username
	}
	if in.Sensitivity != nil {
		c.Sensitivity = *in.Sensitivity
	}
	if in.Note != nil {
		c.Note = *in.Note
	}
	r.rows[id] = c
	if secretEnc != nil {
		r.secrets[id] = *secretEnc
	}
	return c, nil
}

func (r *credentialFakeRepo) DeleteCredential(_ context.Context, workspaceID string, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.rows[id]
	if !ok || c.WorkspaceID != workspaceID {
		return fmt.Errorf("not found")
	}
	delete(r.rows, id)
	delete(r.secrets, id)
	return nil
}

func (r *credentialFakeRepo) FindCredentialBySite(_ context.Context, workspaceID, site string) (Credential, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var best Credential
	found := false
	for _, c := range r.rows {
		if c.WorkspaceID == workspaceID && c.Site == site {
			if !found || c.UpdatedAt.After(best.UpdatedAt) {
				best = c
				found = true
			}
		}
	}
	if !found {
		return Credential{}, fmt.Errorf("not found")
	}
	return best, nil
}

func (r *credentialFakeRepo) InsertCredentialExecution(_ context.Context, e CredentialExecution) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.executions = append(r.executions, e)
	return nil
}

func (r *credentialFakeRepo) ListCredentialExecutions(_ context.Context, _ string, _ CredentialExecutionQuery) ([]CredentialExecution, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := append([]CredentialExecution(nil), r.executions...)
	return out, int64(len(out)), nil
}

// credCaptureLogger 捕获全部日志消息与字段，用于「明文不进日志」断言。
type credCaptureLogger struct {
	mu    sync.Mutex
	blobs []string
}

func (l *credCaptureLogger) record(level, msg string, fields ...loggateway.Field) {
	var b strings.Builder
	b.WriteString(level)
	b.WriteString(" ")
	b.WriteString(msg)
	for _, f := range fields {
		b.WriteString(" ")
		b.WriteString(f.Key)
		b.WriteString("=")
		b.WriteString(f.String)
		if f.Interface != nil {
			fmt.Fprintf(&b, "%v", f.Interface)
		}
	}
	l.mu.Lock()
	l.blobs = append(l.blobs, b.String())
	l.mu.Unlock()
}

func (l *credCaptureLogger) Debug(msg string, fields ...loggateway.Field) {
	l.record("debug", msg, fields...)
}
func (l *credCaptureLogger) Info(msg string, fields ...loggateway.Field) {
	l.record("info", msg, fields...)
}
func (l *credCaptureLogger) Warn(msg string, fields ...loggateway.Field) {
	l.record("warn", msg, fields...)
}
func (l *credCaptureLogger) Error(msg string, fields ...loggateway.Field) {
	l.record("error", msg, fields...)
}
func (l *credCaptureLogger) With(fields ...loggateway.Field) loggateway.Logger {
	l.record("with", "", fields...)
	return l
}

func (l *credCaptureLogger) joined() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.blobs, "\n")
}

func setupCredentialUsecase(t *testing.T) (*CredentialUsecase, *credentialFakeRepo, *credCaptureLogger, context.Context) {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	t.Setenv(envCredentialKey, hex.EncodeToString(key))
	lg := &credCaptureLogger{}
	repo := newCredentialFakeRepo()
	uc := NewCredentialUsecase(repo, NewCredentialCrypto(nil, loggateway.NewNoop()), []string{"intranet.corp"}, lg)
	return uc, repo, lg, context.Background()
}

func TestNormalizeCredentialSite(t *testing.T) {
	cases := map[string]string{
		"HTTPS://GitHub.COM:8443/path/to?q=1": "github.com",
		"github.com.":                         "github.com",
		".GitHub.COM":                         "github.com",
		"  Example.com/ ":                     "example.com",
		"":                                    "",
	}
	for in, want := range cases {
		if got := NormalizeCredentialSite(in); got != want {
			t.Errorf("NormalizeCredentialSite(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMatchHighSensitivitySite(t *testing.T) {
	list := []string{"bank.com", "gov.cn"}
	must := map[string]bool{
		"bank.com":       true,
		"login.bank.com": true,
		"notbank.com":    false,
		"bank.com.evil":  false,
		"gov.cn":         true,
		"tax.gov.cn":     true,
		"example.com":    false,
	}
	for site, want := range must {
		if got := MatchHighSensitivitySite(site, list); got != want {
			t.Errorf("MatchHighSensitivitySite(%q) = %v, want %v", site, got, want)
		}
	}
}

func TestCredentialCreate_AutoMarksHighSensitivity(t *testing.T) {
	uc, repo, _, ctx := setupCredentialUsecase(t)
	c, err := uc.Create(ctx, CredentialCreateInput{
		Name: "网银", Site: "icbc.com.cn", Username: "u", SecretKind: CredentialKindPassword, Secret: "pw-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.Sensitivity != CredentialSensitivityHigh {
		t.Fatalf("expected auto high sensitivity, got %q", c.Sensitivity)
	}
	// 配置侧附加清单同样生效（只增不减）。
	c2, err := uc.Create(ctx, CredentialCreateInput{
		Name: "内网", Site: "intranet.corp", SecretKind: CredentialKindToken, Secret: "tok-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if c2.Sensitivity != CredentialSensitivityHigh {
		t.Fatalf("expected configured high sensitivity, got %q", c2.Sensitivity)
	}
	// 普通站点默认 normal。
	c3, err := uc.Create(ctx, CredentialCreateInput{
		Name: "代码站", Site: "github.com", SecretKind: CredentialKindPassword, Secret: "pw-2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if c3.Sensitivity != CredentialSensitivityNormal {
		t.Fatalf("expected normal sensitivity, got %q", c3.Sensitivity)
	}
	if len(repo.rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(repo.rows))
	}
}

func TestCredentialDecideExecution_Policy(t *testing.T) {
	uc, _, _, ctx := setupCredentialUsecase(t)
	actor := ExecutionActor{ActorType: "agent", ActorKey: "agent-1", AgentID: "a1", SessionID: "s1"}
	if _, err := uc.Create(ctx, CredentialCreateInput{
		Name: "代码站", Site: "github.com", SecretKind: CredentialKindPassword, Secret: "pw-2",
	}); err != nil {
		t.Fatal(err)
	}

	// 高敏站点：默认拒绝。
	if _, verdict, err := uc.DecideExecution(ctx, "https://www.alipay.com/login", CredentialActionLoginExecute, actor); err == nil || verdict != CredentialVerdictDeniedSensitivity {
		t.Fatalf("high-sensitivity: verdict=%q err=%v", verdict, err)
	}
	// 未知动作：拒绝。
	if _, verdict, err := uc.DecideExecution(ctx, "github.com", "delete_everything", actor); err == nil || verdict != CredentialVerdictDeniedPolicy {
		t.Fatalf("unknown action: verdict=%q err=%v", verdict, err)
	}
	// 无凭据站点：拒绝。
	if _, verdict, err := uc.DecideExecution(ctx, "gitlab.com", CredentialActionTokenIssue, actor); err == nil || verdict != CredentialVerdictDeniedNoCredential {
		t.Fatalf("no credential: verdict=%q err=%v", verdict, err)
	}
	// 正常站点：放行。
	cred, verdict, err := uc.DecideExecution(ctx, "github.com", CredentialActionLoginExecute, actor)
	if err != nil || verdict != CredentialVerdictAllowed || cred.Site != "github.com" {
		t.Fatalf("allowed: cred=%+v verdict=%q err=%v", cred, verdict, err)
	}

	// 全量审计：四次裁决（含三次拒绝）全部落账。
	execs, total, err := uc.ListExecutions(ctx, CredentialExecutionQuery{})
	if err != nil || total != 4 {
		t.Fatalf("expected 4 audit rows, got total=%d err=%v", total, err)
	}
	wantVerdicts := []string{
		CredentialVerdictDeniedSensitivity,
		CredentialVerdictDeniedPolicy,
		CredentialVerdictDeniedNoCredential,
		CredentialVerdictAllowed,
	}
	for i, w := range wantVerdicts {
		if execs[i].Verdict != w {
			t.Errorf("audit[%d].Verdict = %q, want %q", i, execs[i].Verdict, w)
		}
		if execs[i].ActorType != "agent" || execs[i].AgentID != "a1" {
			t.Errorf("audit[%d] actor lost: %+v", i, execs[i])
		}
	}
}

// 明文三不进主断言：secret 明文不进日志、不进业务视图（List/Get 无该字段）、
// 不进审计行；落库密文可经执行面单点解密还原。
func TestCredentialPlaintextNeverEscapes(t *testing.T) {
	uc, repo, lg, ctx := setupCredentialUsecase(t)
	const secret = "Sup3rS3cret-P@ssw0rd-20261109"

	created, err := uc.Create(ctx, CredentialCreateInput{
		Name: "代码站", Site: "github.com", Username: "octocat",
		SecretKind: CredentialKindPassword, Secret: secret,
	})
	if err != nil {
		t.Fatal(err)
	}

	// 1) 落库仅密文：repo 存储值不含明文，且业务视图类型根本没有 secret 字段。
	storedEnc := repo.secrets[created.ID]
	if storedEnc == "" || strings.Contains(storedEnc, secret) {
		t.Fatalf("stored secret_enc must be ciphertext without plaintext, got %q", storedEnc)
	}
	list, _, err := uc.List(ctx, CredentialListQuery{})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range list {
		if strings.Contains(fmt.Sprintf("%+v", c), secret) {
			t.Fatalf("plaintext leaked into list view: %+v", c)
		}
	}

	// 2) 裁决 + 审计全链跑一遍。
	actor := ExecutionActor{ActorType: "agent", ActorKey: "agent-9", AgentID: "a9", SessionID: "s9"}
	cred, verdict, err := uc.DecideExecution(ctx, "github.com", CredentialActionLoginExecute, actor)
	if err != nil || verdict != CredentialVerdictAllowed {
		t.Fatalf("decide: verdict=%q err=%v", verdict, err)
	}
	// 3) 执行面单点解密：返回明文供 executor 使用（这是唯一合法出口）。
	plain, err := uc.RevealSecretForExecution(ctx, cred.ID, CredentialActionLoginExecute, actor)
	if err != nil {
		t.Fatal(err)
	}
	if plain != secret {
		t.Fatalf("reveal mismatch: got %q", plain)
	}
	// 触发一条审计写失败路径（repo 正常时不走；直接断言现有日志即可）。

	// 4) 审计行不含明文。
	for _, e := range repo.executions {
		if strings.Contains(fmt.Sprintf("%+v", e), secret) {
			t.Fatalf("plaintext leaked into audit row: %+v", e)
		}
	}
	// 5) 日志不含明文（含 Create/Decide/Reveal 全程）。
	if strings.Contains(lg.joined(), secret) {
		t.Fatalf("plaintext leaked into logs:\n%s", lg.joined())
	}
}

// 无密钥时写入路径 fail-closed（不静默明文降级）。
func TestCredentialCreate_FailsClosedWithoutKey(t *testing.T) {
	_ = os.Unsetenv(envCredentialKey)
	repo := newCredentialFakeRepo()
	uc := NewCredentialUsecase(repo, NewCredentialCrypto(nil, loggateway.NewNoop()), nil, loggateway.NewNoop())
	if _, err := uc.Create(context.Background(), CredentialCreateInput{
		Name: "x", Site: "github.com", SecretKind: CredentialKindPassword, Secret: "pw",
	}); err == nil {
		t.Fatal("expected hard error when credential key missing")
	}
	if len(repo.rows) != 0 {
		t.Fatalf("no row must be written without key, got %d", len(repo.rows))
	}
}

func TestCredentialImportBitwarden(t *testing.T) {
	uc, repo, lg, ctx := setupCredentialUsecase(t)
	const vault = `{
	  "items": [
	    {"type": 1, "name": "GH", "notes": "n", "login": {"username": "octocat", "password": "bw-secret-1", "uris": [{"uri": "https://github.com/login"}]}},
	    {"type": 1, "name": "", "login": {"username": "bot", "password": "bw-secret-2", "uris": [{"uri": "gitlab.com"}]}},
	    {"type": 2, "name": "note-only"},
	    {"type": 1, "name": "no-password", "login": {"username": "x", "password": "", "uris": [{"uri": "example.com"}]}},
	    {"type": 1, "name": "no-uri", "login": {"username": "x", "password": "pw", "uris": []}}
	  ]
	}`
	res, err := uc.ImportBitwarden(ctx, vault)
	if err != nil {
		t.Fatal(err)
	}
	if res.Imported != 2 || res.Skipped != 3 {
		t.Fatalf("expected imported=2 skipped=3, got %+v", res)
	}
	if len(repo.rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(repo.rows))
	}
	// 迁入即加密：落库值不是明文；明文不进日志。
	for id, enc := range repo.secrets {
		if strings.Contains(enc, "bw-secret-") {
			t.Fatalf("row %d stored plaintext: %q", id, enc)
		}
	}
	if strings.Contains(lg.joined(), "bw-secret-1") || strings.Contains(lg.joined(), "bw-secret-2") {
		t.Fatalf("import plaintext leaked into logs:\n%s", lg.joined())
	}
}
