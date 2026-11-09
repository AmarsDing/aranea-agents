package credential

// 工具层断言（E 阶段 W1a）：
//   - 双闸：vault 为 nil 不装配；ARANEA_CREDENTIAL_TOOLS 环境闸默认关。
//   - fail-closed：executor 未接线时诚实返回 unavailable，不伪造成功。
//   - 明文三不进之「不进 prompt」：工具结果（即 prompt 输入）永不含 secret；
//     secret 仅在 Call 内部直送 executor。
//   - 每次调用先经 Vault.DecideExecution 裁决（拒绝直接上抛，不进 executor）。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

type fakeVault struct {
	cred          biz.Credential
	decideErr     error
	revealErr     error
	decideCalls   int
	revealCalls   int
	lastDecideAct string
}

func (v *fakeVault) DecideExecution(_ context.Context, site, action string, _ biz.ExecutionActor) (biz.Credential, string, error) {
	v.decideCalls++
	v.lastDecideAct = action
	if v.decideErr != nil {
		return biz.Credential{}, biz.CredentialVerdictDeniedSensitivity, v.decideErr
	}
	return v.cred, biz.CredentialVerdictAllowed, nil
}

func (v *fakeVault) RevealSecretForExecution(_ context.Context, _ int64, _ string, _ biz.ExecutionActor) (string, error) {
	v.revealCalls++
	if v.revealErr != nil {
		return "", v.revealErr
	}
	return "tool-secret-plaintext-9f8e7d", nil
}

type fakeExecutor struct {
	loginCalls  int
	tokenCalls  int
	gotSecret   string
	loginResult map[string]any
}

func (e *fakeExecutor) LoginExecute(_ context.Context, site, url, username, secret string) (map[string]any, error) {
	e.loginCalls++
	e.gotSecret = secret
	if e.loginResult != nil {
		return e.loginResult, nil
	}
	return map[string]any{"session": "established", "site": site, "url": url, "username": username}, nil
}

func (e *fakeExecutor) TokenIssue(_ context.Context, site, username, secret string) (string, string, error) {
	e.tokenCalls++
	e.gotSecret = secret
	return "tmp-token-abc", "2026-11-09T12:00:00Z", nil
}

func TestNewToolset_NilVaultNotAssembled(t *testing.T) {
	if got := NewToolset(nil, nil); got != nil {
		t.Fatalf("nil vault must prune toolset, got %d tools", len(got))
	}
}

func TestEnabled_EnvGate(t *testing.T) {
	t.Setenv("ARANEA_CREDENTIAL_TOOLS", "")
	if Enabled() {
		t.Fatal("env gate must default to off")
	}
	t.Setenv("ARANEA_CREDENTIAL_TOOLS", "1")
	if !Enabled() {
		t.Fatal("env gate on with ARANEA_CREDENTIAL_TOOLS=1")
	}
	t.Setenv("ARANEA_CREDENTIAL_TOOLS", "false")
	if Enabled() {
		t.Fatal("env gate off with non-1 value")
	}
}

func TestCredentialTool_Declarations(t *testing.T) {
	v := &fakeVault{cred: biz.Credential{ID: 7, Site: "github.com", Username: "octocat"}}
	tools := NewToolset(v, nil)
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
	names := map[string]bool{}
	for _, tl := range tools {
		d := tl.Declaration()
		names[d.Name] = true
		if d.Description == "" || d.InputSchema == nil {
			t.Fatalf("tool %s missing description/schema", d.Name)
		}
	}
	if !names["credential_login_execute"] || !names["credential_token_issue"] {
		t.Fatalf("unexpected tool names: %v", names)
	}
}

func TestCredentialTool_NilExecutorHonestUnavailable(t *testing.T) {
	v := &fakeVault{cred: biz.Credential{ID: 7, Site: "github.com", Username: "octocat"}}
	tools := NewToolset(v, nil)
	res, err := tools[0].Call(context.Background(), []byte(`{"site":"github.com"}`))
	if err != nil {
		t.Fatal(err)
	}
	m, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("unexpected result type %T", res)
	}
	if m["status"] != "unavailable" {
		t.Fatalf("expected honest unavailable, got %v", m["status"])
	}
	// 裁决已走（审计落账），但未执行 → 不解密 secret。
	if v.decideCalls != 1 || v.lastDecideAct != biz.CredentialActionLoginExecute {
		t.Fatalf("decide calls=%d action=%q", v.decideCalls, v.lastDecideAct)
	}
	if v.revealCalls != 0 {
		t.Fatalf("secret must not be revealed when executor is nil (calls=%d)", v.revealCalls)
	}
	// 结果中绝无 secret 字样字段。
	if _, leaked := m["secret"]; leaked {
		t.Fatal("result must not contain secret field")
	}
}

func TestCredentialTool_ExecutorPath_NeverLeaksSecret(t *testing.T) {
	const plain = "tool-secret-plaintext-9f8e7d"
	v := &fakeVault{cred: biz.Credential{ID: 7, Site: "github.com", Username: "octocat", URL: "https://github.com/login"}}
	ex := &fakeExecutor{}
	tools := NewToolset(v, ex)

	// login_execute
	res, err := tools[0].Call(context.Background(), []byte(`{"site":"github.com"}`))
	if err != nil {
		t.Fatal(err)
	}
	if ex.loginCalls != 1 || ex.gotSecret != plain {
		t.Fatalf("executor must receive plaintext secret internally, calls=%d", ex.loginCalls)
	}
	blob, _ := json.Marshal(res)
	if contains := jsonContains(string(blob), plain); contains {
		t.Fatalf("secret leaked into login result: %s", blob)
	}

	// token_issue
	res, err = tools[1].Call(context.Background(), []byte(`{"site":"github.com"}`))
	if err != nil {
		t.Fatal(err)
	}
	if ex.tokenCalls != 1 || ex.gotSecret != plain {
		t.Fatalf("token executor calls=%d", ex.tokenCalls)
	}
	m := res.(map[string]any)
	if m["status"] != "ok" || m["token"] != "tmp-token-abc" {
		t.Fatalf("unexpected token result: %v", m)
	}
	blob, _ = json.Marshal(res)
	if jsonContains(string(blob), plain) {
		t.Fatalf("secret leaked into token result: %s", blob)
	}
}

func jsonContains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}

func TestCredentialTool_DeniedGoesNoFurther(t *testing.T) {
	v := &fakeVault{decideErr: fmt.Errorf("site is on the high-sensitivity list")}
	ex := &fakeExecutor{}
	tools := NewToolset(v, ex)
	if _, err := tools[0].Call(context.Background(), []byte(`{"site":"alipay.com"}`)); err == nil {
		t.Fatal("denied decision must surface as error")
	}
	if ex.loginCalls != 0 || v.revealCalls != 0 {
		t.Fatalf("executor/reveal must not run on deny: login=%d reveal=%d", ex.loginCalls, v.revealCalls)
	}
	// 非法参数先行拦截（不裁决）。
	if _, err := tools[0].Call(context.Background(), []byte(`{}`)); err == nil {
		t.Fatal("empty site must be rejected")
	}
	if _, err := tools[0].Call(context.Background(), []byte(`{bad json`)); err == nil {
		t.Fatal("invalid json must be rejected")
	}
}
