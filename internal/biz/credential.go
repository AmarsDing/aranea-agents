package biz

// E 阶段凭据托管模块（需求文档 §3，拍板 V-1/V-2/V-3）。
//
// 硬规则（需求概要 §4.5 与本域 proto 头注）：
//   - 明文三不进：不进日志、不进 prompt、不进任何 API 响应。本文件任何
//     loggateway 调用不得携带 secret 明文或密文；repo 层 SQL 错误向上传递
//     时也不得拼接 secret 字段。
//   - agent 侧仅两个动词（credential.login_execute / credential.token_issue），
//     经 DecideExecution 策略裁决：高敏站点默认拒绝（V-3 清单配置化），其余
//     走 approval-policy 四档语义由工具层挂接（默认关/高风险/需确认）。
//   - 全量审计：每次裁决（含拒绝）落 credential_executions。
//
// 加密：写入路径必须过 RequireKeyForSecret + EncryptSecret（信封 AES-GCM，
// 主密钥 ARANEA_CREDENTIAL_KEY 或 system settings，V-2）；无密钥硬错，
// 不静默明文降级。执行面经 RevealSecretForExecution 单点解密，仅返回给
// executor 内部使用，绝不出现在 API 响应。

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"
)

// SecretKind / Sensitivity 枚举值（与 api/kratos/credential/v1 对齐）。
const (
	CredentialKindPassword = "password"
	CredentialKindToken    = "token"
	CredentialKindCookie   = "cookie"

	CredentialSensitivityNormal = "normal"
	CredentialSensitivityHigh   = "high"
)

// 执行动作与裁决值（与 proto CredentialExecution 注释对齐）。
const (
	CredentialActionLoginExecute = "login_execute"
	CredentialActionTokenIssue   = "token_issue"

	CredentialVerdictAllowed            = "allowed"
	CredentialVerdictDeniedPolicy       = "denied_policy"
	CredentialVerdictDeniedSensitivity  = "denied_sensitivity"
	CredentialVerdictDeniedNotFound     = "denied_not_found"
	CredentialVerdictDeniedNoCredential = "denied_no_credential"
	CredentialVerdictFailed             = "failed"
)

// Credential 是 credentials 表一行的业务视图（不含 secret）。
type Credential struct {
	ID          int64
	WorkspaceID string
	Name        string
	Site        string
	URL         string
	Username    string
	SecretKind  string
	Sensitivity string
	Note        string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// CredentialCreateInput 是新建凭据的输入（Secret 为一次性明文，写后即弃）。
type CredentialCreateInput struct {
	Name        string
	Site        string
	URL         string
	Username    string
	SecretKind  string
	Sensitivity string
	Note        string
	Secret      string
}

// CredentialUpdateInput 是部分更新输入；指针为 nil 表示不动该列。
// Secret 非 nil 且非空 = 轮换 secret。
type CredentialUpdateInput struct {
	Name        *string
	URL         *string
	Username    *string
	Sensitivity *string
	Note        *string
	Secret      *string
}

// CredentialListQuery 是管理面分页查询。
type CredentialListQuery struct {
	Site     string
	Page     int // 1-based；<=0 按 1
	PageSize int // <=0 默认 50；>200 截 200
}

// CredentialExecution 是执行审计一行。
type CredentialExecution struct {
	ID           int64
	CredentialID int64
	Site         string
	Action       string
	ActorType    string
	ActorKey     string
	AgentID      string
	SessionID    string
	Verdict      string
	Reason       string
	CreatedAt    time.Time
}

// CredentialExecutionQuery 是审计分页查询。
type CredentialExecutionQuery struct {
	CredentialID int64
	Site         string
	Page         int
	PageSize     int
}

// CredentialRepo 抽象 credentials / credential_executions 持久化。
type CredentialRepo interface {
	ListCredentials(ctx context.Context, workspaceID string, q CredentialListQuery) ([]Credential, int64, error)
	GetCredential(ctx context.Context, workspaceID string, id int64) (Credential, error)
	// GetCredentialSecret 仅执行面使用：返回 (元数据, 密文)。
	GetCredentialSecret(ctx context.Context, workspaceID string, id int64) (Credential, string, error)
	CreateCredential(ctx context.Context, c Credential, secretEnc string) (Credential, error)
	UpdateCredential(ctx context.Context, workspaceID string, id int64, in CredentialUpdateInput, secretEnc *string) (Credential, error)
	DeleteCredential(ctx context.Context, workspaceID string, id int64) error
	// FindCredentialBySite 按站点定位凭据（执行面；同站点多条取最近更新）。
	FindCredentialBySite(ctx context.Context, workspaceID, site string) (Credential, error)
	InsertCredentialExecution(ctx context.Context, e CredentialExecution) error
	ListCredentialExecutions(ctx context.Context, workspaceID string, q CredentialExecutionQuery) ([]CredentialExecution, int64, error)
}

// DefaultHighSensitivitySites 是内置高敏站点清单（V-3）。与
// configs credential.high_sensitivity_sites 取并集；配置侧只能加、不能减
// （安全向单调）。匹配规则：规范化 host（小写、去端口、去首尾点）后精确
// 相等或后缀匹配（"bank.com" 命中 "login.bank.com"，不命中 "notbank.com"）。
func DefaultHighSensitivitySites() []string {
	return []string{
		// 金融/支付：登录态可直接动钱，agent 永不自动执行。
		"alipay.com", "paypal.com", "stripe.com", "unionpay.com",
		"icbc.com.cn", "ccb.com", "boc.cn", "abcchina.com", "cmbchina.com",
		// 交易所/券商：与资金操作直连。
		"binance.com", "okx.com", "coinbase.com", "huobi.com",
		// 政务/社保/税务。
		"gov.cn", "12366.gov.cn", "si.12333.gov.cn",
	}
}

// NormalizeCredentialSite 规范化站点标识：小写、去 scheme、去路径、去端口、
// 去首尾点。空串原样返回（调用方校验必填）。
func NormalizeCredentialSite(site string) string {
	s := strings.ToLower(strings.TrimSpace(site))
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndex(s, ":"); i >= 0 && !strings.Contains(s[i+1:], "]") {
		s = s[:i]
	}
	return strings.Trim(s, ".")
}

// MatchHighSensitivitySite 报告 site 是否命中清单（精确或后缀）。
func MatchHighSensitivitySite(site string, list []string) bool {
	host := NormalizeCredentialSite(site)
	if host == "" {
		return false
	}
	for _, item := range list {
		h := NormalizeCredentialSite(item)
		if h == "" {
			continue
		}
		if host == h || strings.HasSuffix(host, "."+h) {
			return true
		}
	}
	return false
}

// CredentialUsecase 实现凭据管理面与执行面裁决。
type CredentialUsecase struct {
	repo                 CredentialRepo
	crypto               *CredentialCrypto
	highSensitivitySites []string
	lg                   loggateway.Logger
}

// NewCredentialUsecase 构造用例。highSites 为配置侧附加高敏清单（与内置
// 清单并集，可为空）。crypto 为 nil 时写入路径全部硬错（fail-closed）。
func NewCredentialUsecase(repo CredentialRepo, crypto *CredentialCrypto, highSites []string, lg loggateway.Logger) *CredentialUsecase {
	merged := append([]string(nil), DefaultHighSensitivitySites()...)
	seen := map[string]bool{}
	for _, s := range merged {
		seen[NormalizeCredentialSite(s)] = true
	}
	for _, s := range highSites {
		n := NormalizeCredentialSite(s)
		if n != "" && !seen[n] {
			seen[n] = true
			merged = append(merged, n)
		}
	}
	return &CredentialUsecase{repo: repo, crypto: crypto, highSensitivitySites: merged, lg: lg}
}

func normalizeCredentialPage(q *CredentialListQuery) {
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.PageSize <= 0 {
		q.PageSize = 50
	}
	if q.PageSize > 200 {
		q.PageSize = 200
	}
}

func validateCredentialKind(kind string) error {
	switch kind {
	case CredentialKindPassword, CredentialKindToken, CredentialKindCookie:
		return nil
	default:
		return apierror.BadRequest("CREDENTIAL", "secret_kind must be one of password|token|cookie")
	}
}

func validateCredentialSensitivity(s string) error {
	switch s {
	case "", CredentialSensitivityNormal, CredentialSensitivityHigh:
		return nil
	default:
		return apierror.BadRequest("CREDENTIAL", "sensitivity must be one of normal|high")
	}
}

func (u *CredentialUsecase) workspaceOf(ctx context.Context) string {
	return workspace.IDFromContext(ctx)
}

func (u *CredentialUsecase) requireCrypto() error {
	if u.crypto == nil {
		return apierror.Internal("CREDENTIAL_KEY", credentialKeyRequiredMsg)
	}
	return nil
}

// List 返回管理面分页清单（元数据，永不含 secret）。
func (u *CredentialUsecase) List(ctx context.Context, q CredentialListQuery) ([]Credential, int64, error) {
	normalizeCredentialPage(&q)
	return u.repo.ListCredentials(ctx, u.workspaceOf(ctx), q)
}

// Get 返回单条元数据。
func (u *CredentialUsecase) Get(ctx context.Context, id int64) (Credential, error) {
	return u.repo.GetCredential(ctx, u.workspaceOf(ctx), id)
}

// Create 加密落库一条凭据（E-1：迁入即加密）。
func (u *CredentialUsecase) Create(ctx context.Context, in CredentialCreateInput) (Credential, error) {
	if err := u.requireCrypto(); err != nil {
		return Credential{}, err
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Site = NormalizeCredentialSite(in.Site)
	if in.Name == "" || in.Site == "" {
		return Credential{}, apierror.BadRequest("CREDENTIAL", "name and site are required")
	}
	if err := validateCredentialKind(in.SecretKind); err != nil {
		return Credential{}, err
	}
	if err := validateCredentialSensitivity(in.Sensitivity); err != nil {
		return Credential{}, err
	}
	if in.Sensitivity == "" && MatchHighSensitivitySite(in.Site, u.highSensitivitySites) {
		in.Sensitivity = CredentialSensitivityHigh
	}
	if in.Sensitivity == "" {
		in.Sensitivity = CredentialSensitivityNormal
	}
	enc, err := u.crypto.EncryptSecret(ctx, in.Secret)
	if err != nil {
		return Credential{}, err
	}
	now := time.Now()
	return u.repo.CreateCredential(ctx, Credential{
		WorkspaceID: u.workspaceOf(ctx),
		Name:        in.Name,
		Site:        in.Site,
		URL:         strings.TrimSpace(in.URL),
		Username:    strings.TrimSpace(in.Username),
		SecretKind:  in.SecretKind,
		Sensitivity: in.Sensitivity,
		Note:        strings.TrimSpace(in.Note),
		CreatedAt:   now,
		UpdatedAt:   now,
	}, enc)
}

// Update 部分更新；Secret 非空即轮换（重新加密）。
func (u *CredentialUsecase) Update(ctx context.Context, id int64, in CredentialUpdateInput) (Credential, error) {
	if in.Sensitivity != nil {
		if err := validateCredentialSensitivity(*in.Sensitivity); err != nil {
			return Credential{}, err
		}
	}
	var enc *string
	if in.Secret != nil && strings.TrimSpace(*in.Secret) != "" {
		if err := u.requireCrypto(); err != nil {
			return Credential{}, err
		}
		v, err := u.crypto.EncryptSecret(ctx, *in.Secret)
		if err != nil {
			return Credential{}, err
		}
		enc = &v
	}
	return u.repo.UpdateCredential(ctx, u.workspaceOf(ctx), id, in, enc)
}

// Delete 删除一条凭据。
func (u *CredentialUsecase) Delete(ctx context.Context, id int64) error {
	return u.repo.DeleteCredential(ctx, u.workspaceOf(ctx), id)
}

// ListExecutions 返回执行审计分页（audit.read）。
func (u *CredentialUsecase) ListExecutions(ctx context.Context, q CredentialExecutionQuery) ([]CredentialExecution, int64, error) {
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.PageSize <= 0 {
		q.PageSize = 50
	}
	if q.PageSize > 200 {
		q.PageSize = 200
	}
	return u.repo.ListCredentialExecutions(ctx, u.workspaceOf(ctx), q)
}

// IsHighSensitivitySite 报告站点是否命中高敏清单（供工具层/前端提示用）。
func (u *CredentialUsecase) IsHighSensitivitySite(site string) bool {
	return MatchHighSensitivitySite(site, u.highSensitivitySites)
}

// ExecutionActor 描述一次执行的发起者。
type ExecutionActor struct {
	ActorType string // user | agent | system
	ActorKey  string
	AgentID   string
	SessionID string
}

// ExecutionActorFromContext 从 JWT 组装 user 侧 actor；agent 侧由工具层显式传入。
func ExecutionActorFromContext(ctx context.Context) ExecutionActor {
	if a, ok := auth.FromContext(ctx); ok && a != nil {
		return ExecutionActor{
			ActorType: "user",
			ActorKey:  strconv.FormatInt(a.UserID, 10),
		}
	}
	return ExecutionActor{ActorType: "system"}
}

// RecordExecution 落一条执行审计（拒绝也落——全量审计口径）。审计写入失败
// 仅记日志不上抛，避免审计库故障放大为业务故障；审计明细本身永不含 secret。
func (u *CredentialUsecase) RecordExecution(ctx context.Context, e CredentialExecution) {
	e.CreatedAt = time.Now()
	if err := u.repo.InsertCredentialExecution(ctx, e); err != nil && u.lg != nil {
		u.lg.Warn("credential execution audit insert failed",
			loggateway.StepID("credential.audit"),
			loggateway.Str("site", e.Site),
			loggateway.Str("action", e.Action),
			loggateway.Str("verdict", e.Verdict),
			loggateway.Err(err))
	}
}

// DecideExecution 是执行面的策略裁决单点（V-3）：
//  1. site 命中高敏清单 → denied_sensitivity（高敏默认拒绝，不经四档流转）。
//  2. 站点无凭据 → denied_no_credential。
//  3. 其余 → allowed（approval-policy 四档挂接在工具层：默认关/高风险/需确认，
//     工具被禁用时工具层先拒，不会走到这里）。
//
// 每次调用（含拒绝）落审计。返回凭据元数据供后续 Reveal 使用。
func (u *CredentialUsecase) DecideExecution(ctx context.Context, site, action string, actor ExecutionActor) (Credential, string, error) {
	norm := NormalizeCredentialSite(site)
	audit := func(verdict, reason string) {
		u.RecordExecution(ctx, CredentialExecution{
			Site:      norm,
			Action:    action,
			ActorType: actorTypeOrDefault(actor.ActorType),
			ActorKey:  actor.ActorKey,
			AgentID:   actor.AgentID,
			SessionID: actor.SessionID,
			Verdict:   verdict,
			Reason:    reason,
		})
	}
	if action != CredentialActionLoginExecute && action != CredentialActionTokenIssue {
		audit(CredentialVerdictDeniedPolicy, "unknown action")
		return Credential{}, CredentialVerdictDeniedPolicy, apierror.BadRequest("CREDENTIAL", "unknown credential action: %s", action)
	}
	if norm == "" {
		audit(CredentialVerdictDeniedPolicy, "site is required")
		return Credential{}, CredentialVerdictDeniedPolicy, apierror.BadRequest("CREDENTIAL", "site is required")
	}
	if u.IsHighSensitivitySite(norm) {
		audit(CredentialVerdictDeniedSensitivity, "site matches high-sensitivity list")
		return Credential{}, CredentialVerdictDeniedSensitivity, apierror.Forbidden("CREDENTIAL", "site is on the high-sensitivity list; execution denied by policy")
	}
	cred, err := u.repo.FindCredentialBySite(ctx, u.workspaceOf(ctx), norm)
	if err != nil {
		audit(CredentialVerdictDeniedNoCredential, "no credential for site")
		return Credential{}, CredentialVerdictDeniedNoCredential, apierror.NotFound("CREDENTIAL", "no credential for site")
	}
	audit(CredentialVerdictAllowed, "")
	return cred, CredentialVerdictAllowed, nil
}

func actorTypeOrDefault(t string) string {
	if t == "" {
		return "system"
	}
	return t
}

// RevealSecretForExecution 解密指定凭据的 secret，仅供执行面（executor）在
// DecideExecution 放行后调用。返回值绝不出现在日志/prompt/API 响应；
// 调用方用完应立即丢弃。失败时落 failed 审计并 fail-closed。
func (u *CredentialUsecase) RevealSecretForExecution(ctx context.Context, id int64, action string, actor ExecutionActor) (string, error) {
	if err := u.requireCrypto(); err != nil {
		return "", err
	}
	cred, enc, err := u.repo.GetCredentialSecret(ctx, u.workspaceOf(ctx), id)
	if err != nil {
		return "", err
	}
	plain, err := u.crypto.DecryptSecret(ctx, enc)
	if err != nil {
		u.RecordExecution(ctx, CredentialExecution{
			CredentialID: id,
			Site:         cred.Site,
			Action:       action,
			ActorType:    actorTypeOrDefault(actor.ActorType),
			ActorKey:     actor.ActorKey,
			AgentID:      actor.AgentID,
			SessionID:    actor.SessionID,
			Verdict:      CredentialVerdictFailed,
			Reason:       "decrypt failed",
		})
		return "", err
	}
	return plain, nil
}

// ============================================================================
// Bitwarden 导入（E-1：迁入即加密；解析出的明文只在内存中过手一次）
// ============================================================================

// bitwardenVault 是 Bitwarden 导出 JSON 的最小结构（仅取 login 段）。
type bitwardenVault struct {
	Items []struct {
		Type  int    `json:"type"` // 1 = login
		Name  string `json:"name"`
		Notes string `json:"notes"`
		Login *struct {
			Username string `json:"username"`
			Password string `json:"password"`
			TOTP     string `json:"totp"`
			URIs     []struct {
				URI string `json:"uri"`
			} `json:"uris"`
		} `json:"login"`
	} `json:"items"`
}

// ImportBitwardenResult 汇报导入结果。
type ImportBitwardenResult struct {
	Imported int
	Skipped  int
}

// ImportBitwarden 解析 Bitwarden vault JSON 并逐条加密落库。跳过：非 login
// 条目、缺密码、site 无法从 URI 推导。导入失败单条不阻断整体（计数进 Skipped
// 之外的错误才上抛——如加密密钥缺失属全局硬错）。
func (u *CredentialUsecase) ImportBitwarden(ctx context.Context, vaultJSON string) (ImportBitwardenResult, error) {
	if err := u.requireCrypto(); err != nil {
		return ImportBitwardenResult{}, err
	}
	var vault bitwardenVault
	if err := json.Unmarshal([]byte(vaultJSON), &vault); err != nil {
		return ImportBitwardenResult{}, apierror.BadRequest("CREDENTIAL", "vault_json must be valid Bitwarden export JSON")
	}
	var res ImportBitwardenResult
	for _, item := range vault.Items {
		if item.Type != 1 || item.Login == nil || strings.TrimSpace(item.Login.Password) == "" {
			res.Skipped++
			continue
		}
		url := ""
		if len(item.Login.URIs) > 0 {
			url = strings.TrimSpace(item.Login.URIs[0].URI)
		}
		site := NormalizeCredentialSite(url)
		if site == "" {
			res.Skipped++
			continue
		}
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = site
		}
		_, err := u.Create(ctx, CredentialCreateInput{
			Name:       name,
			Site:       site,
			URL:        url,
			Username:   strings.TrimSpace(item.Login.Username),
			SecretKind: CredentialKindPassword,
			Note:       strings.TrimSpace(item.Notes),
			Secret:     item.Login.Password,
		})
		if err != nil {
			// 密钥缺失等全局错误直接上抛；单条校验错误计跳过。
			if apierror.IsCode(err, apierror.CodeInternal) {
				return res, err
			}
			res.Skipped++
			continue
		}
		res.Imported++
	}
	return res, nil
}
