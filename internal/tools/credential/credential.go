// Package credential implements the two agent-side credential verbs
// (E 阶段凭据托管，需求文档 §3，拍板 V-1)：
//
//   - credential_login_execute — 在目的站点执行登录（computer-use 执行面）。
//   - credential_token_issue   — 为目标操作签发一次性临时令牌。
//
// 硬规则：
//   - agent 永不读取密码明文作为工具结果：secret 只在服务端内部从 vault
//     解密后直送 executor，工具返回中绝不出现 secret 或其派生物（明文三不进
//     之「不进 prompt」——工具结果即进 prompt）。
//   - 每次调用先经 Vault.DecideExecution 策略裁决并落审计（含拒绝）。
//   - executor 未接线（computeruse 接线前）时诚实返回 unavailable——不伪造
//     成功、不静默跳过（fail-closed）。
//
// 装配：默认关闭（tools 表种子 enabled=false + 高风险 + 需确认；另需
// ARANEA_CREDENTIAL_TOOLS=1 环境闸），经 builder deps 注入 Vault/Executor。
package credential

import (
	"context"
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/decision"
	"aranea-agents/pkg/apierror"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// Vault 是凭据库裁决/解密封口（由 biz.CredentialUsecase 满足）。
type Vault interface {
	// DecideExecution 策略裁决并落审计；allowed 时返回凭据元数据。
	DecideExecution(ctx context.Context, site, action string, actor biz.ExecutionActor) (biz.Credential, string, error)
	// RevealSecretForExecution 解密 secret 仅供 executor 使用。
	RevealSecretForExecution(ctx context.Context, id int64, action string, actor biz.ExecutionActor) (string, error)
}

// Executor 是执行面接口（computer-use / browser 接线后由运行时提供）。
// 实现方约束：不得记录/外传 secret；返回结果不得包含 secret 或会话凭据明文。
type Executor interface {
	// LoginExecute 在目的站点执行登录动作。
	LoginExecute(ctx context.Context, site, url, username, secret string) (map[string]any, error)
	// TokenIssue 用凭据为站点签发一次性临时令牌；返回令牌与过期时刻（RFC3339）。
	TokenIssue(ctx context.Context, site, username, secret string) (token string, expiresAt string, err error)
}

// Enabled 是环境闸（默认关）：ARANEA_CREDENTIAL_TOOLS=1/true 才装配。
// 与 tools 表种子 enabled=false 双闸：环境闸管装配，表种子管 agent 侧可见性。
func Enabled() bool {
	switch strings.ToLower(strings.TrimSpace(lookupEnv("ARANEA_CREDENTIAL_TOOLS"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// NewToolset 构造两动词工具集。vault 为 nil 返回 nil（未装配凭据模块时不暴露）。
// executor 可为 nil——工具仍可调用并完成策略裁决/审计，执行步诚实返回
// unavailable（fail-closed）。
func NewToolset(vault Vault, executor Executor) []trpctool.CallableTool {
	if vault == nil {
		return nil
	}
	return []trpctool.CallableTool{
		&tool{
			name:  "credential_login_execute",
			desc:  "Execute a login action on a target site using a vault-managed credential. The secret is never exposed to the agent; policy engine decides per site (high-sensitivity sites are denied). Returns session status only.",
			vault: vault, executor: executor, action: biz.CredentialActionLoginExecute,
			schema: loginSchema,
		},
		&tool{
			name:  "credential_token_issue",
			desc:  "Issue a short-lived temporary token for a target site operation using a vault-managed credential. The underlying secret is never exposed; returns the temporary token with expiry. Policy engine decides per site.",
			vault: vault, executor: executor, action: biz.CredentialActionTokenIssue,
			schema: tokenSchema,
		},
	}
}

type tool struct {
	name     string
	desc     string
	vault    Vault
	executor Executor
	action   string
	schema   *trpctool.Schema
}

func (t *tool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{Name: t.name, Description: t.desc, InputSchema: t.schema}
}

func (t *tool) Call(ctx context.Context, args []byte) (any, error) {
	var in struct {
		Site string `json:"site"`
		URL  string `json:"url"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, apierror.BadRequest(apierror.DomainTool, t.name+": invalid args: "+err.Error())
	}
	site := strings.TrimSpace(in.Site)
	if site == "" {
		return nil, apierror.BadRequest(apierror.DomainTool, t.name+": site is required")
	}
	actor := biz.ExecutionActor{
		ActorType: "agent",
		SessionID: decision.GateSessionIDFromContext(ctx),
	}
	// 1) 策略裁决 + 审计（拒绝也在此落审计并直接上抛错误）。
	cred, _, err := t.vault.DecideExecution(ctx, site, t.action, actor)
	if err != nil {
		return nil, err
	}
	// 2) executor 未接线：诚实降级（裁决已放行且已审计 allowed；执行步失败
	// 由 executor 侧在接线后自审计，这里不落 failed——未执行不算执行失败）。
	if t.executor == nil {
		return map[string]any{
			"status":        "unavailable",
			"site":          cred.Site,
			"action":        t.action,
			"verdict":       biz.CredentialVerdictAllowed,
			"detail":        "credential executor is not wired yet (computer-use integration pending); policy check passed and the attempt was audited, but no action was performed",
			"audited":       true,
			"credential_id": cred.ID,
		}, nil
	}
	// 3) 解密 secret 直送 executor（绝不出现在返回中）。
	secret, err := t.vault.RevealSecretForExecution(ctx, cred.ID, t.action, actor)
	if err != nil {
		return nil, err
	}
	if t.action == biz.CredentialActionTokenIssue {
		tok, expiresAt, err := t.executor.TokenIssue(ctx, cred.Site, cred.Username, secret)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"status":     "ok",
			"site":       cred.Site,
			"action":     t.action,
			"token":      tok,
			"expires_at": expiresAt,
		}, nil
	}
	res, err := t.executor.LoginExecute(ctx, cred.Site, firstNonEmpty(in.URL, cred.URL), cred.Username, secret)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"status": "ok",
		"site":   cred.Site,
		"action": t.action,
		"result": res,
	}, nil
}

func (t *tool) StreamableCall(ctx context.Context, _ []byte) (*trpctool.StreamReader, error) {
	return nil, apierror.BadRequest(apierror.DomainTool, t.name+": credential tools are not streamable")
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return strings.TrimSpace(a)
	}
	return strings.TrimSpace(b)
}

var loginSchema = &trpctool.Schema{
	Type:        "object",
	Description: "Execute login on a target site with a vault credential",
	Properties: map[string]*trpctool.Schema{
		"site": {Type: "string", Description: "Target site key, e.g. github.com"},
		"url":  {Type: "string", Description: "Optional login URL override; defaults to the credential's stored URL"},
	},
	Required: []string{"site"},
}

var tokenSchema = &trpctool.Schema{
	Type:        "object",
	Description: "Issue a temporary token for a target site operation",
	Properties: map[string]*trpctool.Schema{
		"site": {Type: "string", Description: "Target site key, e.g. github.com"},
	},
	Required: []string{"site"},
}
