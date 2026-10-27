// admin.go —— G-38 W5 管理面编排：连接器 CRUD / 手动触发 / 凭据子系统 / runs+items 读径。
//
// 拍板锚点：37-knowledge.design.md §G-38 ④ 凭据模型铁律链 × ⑤ 调度（trigger 语义）
// × ⑦ API 契约（custom JSON + ADR 同 PR）。本层是 **HTTP 与仓口之间唯一编排面**：
//   - handler 薄化（P4 生效条件四）：参数解析/状态码映射/recordAudit 在 service 层，
//     业务裁决（校验/CAS 语义/凭据闸/唤醒链）全部落此，使晋升 proto 时 handler
//     可整体替换而本层零改。
//   - 凭据闸（④）：UpsertCredential 前置 RequireKeyForSecret 硬错——无 key 拒写，
//     绝不静默明文降级；`env:` 备选 scheme 是唯一免加密通道（变量名 POSIX 硬闸）。
//   - 读侧永不返明文（④ 硬约束）：CredentialView 只出 configured+masked_preview，
//     无 Reveal 方法、无 Reveal 端点；secret_ref 原文不出本层。
//   - 唤醒链（④/⑤）：PUT 凭据写成功即 WakeConnectorFromAuthPaused——仅自
//     auth_paused 收，非停轮态 no-op（重复 PUT 不报错读）。不动 next_run_at。
//   - 管理面写 CAS（W5 评审）：UpdateConnector 谓词含 run_token 恒空串守卫——在途轮原子拒
//     管理写；「先 Get 命中 + 后 Update 失配」→409（in-flight），非 404。
//
// 分层纪律：本包不 import internal/service、internal/data（仓口契约由 biz 声明、
// 由装配层注入）；审计（recordAudit）与 HTTP 映射归 service 层 W5 同批。
package sourcesync

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	bizsync "aranea-agents/internal/biz/knowledge/sourcesync"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

const (
	adminDomain     = "KNOWLEDGE_SOURCE_SYNC"
	connectorIDPfx  = "ss:conn:"
	credentialIDPfx = "ss:cred:"
)

// AdminDeps 管理面依赖（全构造注入；Crypto 为 nil 时凭据子系统 fail-closed）。
type AdminDeps struct {
	Log         loggateway.Logger     // nil→NewNoop
	Connectors  bizsync.ConnectorRepo // 必
	Credentials bizsync.CredentialRepo
	Runs        bizsync.RunRepo
	Items       bizsync.ItemRepo
	Crypto      *biz.CredentialCrypto
	Now         func() time.Time // nil→time.Now
	// NewID 幂等注入点（测试恒值；nil→crypto/rand + nano 兜底）。
	NewID func(prefix string) string
}

// AdminService 管理面编排（无内部状态——每次全量从仓口读真相，同 Runner 取向）。
type AdminService struct {
	d     AdminDeps
	log   loggateway.Logger
	now   func() time.Time
	newID func(string) string
}

// NewAdminService 依赖规范化（零值安全）。
func NewAdminService(d AdminDeps) *AdminService {
	lg := d.Log
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	now := d.Now
	if now == nil {
		now = time.Now
	}
	newID := d.NewID
	if newID == nil {
		newID = func(prefix string) string {
			var b [8]byte
			if _, err := rand.Read(b[:]); err != nil {
				return fmt.Sprintf("%s%d", prefix, time.Now().UnixNano())
			}
			return prefix + hex.EncodeToString(b[:])
		}
	}
	return &AdminService{d: d, log: lg, now: now, newID: newID}
}

// ──────────────────────────────── 连接器 CRUD ────────────────────────────────

// ConnectorInput 创建/更新共用输入（零值=未提供，由 applyDefaults 填缺省）。
// ConfigJSON 出入皆是 JSON 文本（⑦ 契约字段形状）；出库经 SanitizeConfigForExport。
type ConnectorInput struct {
	Kind                string
	Name                string
	WorkspaceID         string // 创建时由 service 从 ctx 盖章；更新时忽略（不可变）
	CollectionID        string
	ConfigJSON          string
	IntervalSeconds     int
	Enabled             *bool
	MaxItemsPerRun      int
	MaxDeletesPerRun    int
	MaxFetchBytesPerRun int64
}

// ListConnectors 工作区活性列表（含 auth_paused/error；UI 管理面读径）。
// config_json 出库前 sanitize（⑦ 密文键剥离）。
func (s *AdminService) ListConnectors(ctx context.Context, workspaceID string) ([]bizsync.Connector, error) {
	if s.d.Connectors == nil {
		return nil, apierror.Unavailable(adminDomain, "connector repository not configured")
	}
	items, err := s.d.Connectors.ListConnectorsByWorkspace(ctx, strings.TrimSpace(workspaceID))
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].ConfigJSON = bizsync.SanitizeConfigForExport(items[i].ConfigJSON)
	}
	return items, nil
}

// GetConnector 单行取址 + 工作区隔离闸。跨租户/不存在一律 NotFound（404 反
// IDOR 枚举——知识域 assertCollectionAccess 同宗口径）。
func (s *AdminService) GetConnector(ctx context.Context, id, callerWorkspace string) (bizsync.Connector, error) {
	c, err := s.requireConnector(ctx, id, callerWorkspace)
	if err != nil {
		return bizsync.Connector{}, err
	}
	c.ConfigJSON = bizsync.SanitizeConfigForExport(c.ConfigJSON)
	return c, nil
}

// CreateConnector 落新行：校验 → 填缺省 → id/due 撒点 → 仓口冲突即 409。
// workspace_id 由调用方（service 层）从 ctx 盖章，本层只做归属校验形态。
func (s *AdminService) CreateConnector(ctx context.Context, in ConnectorInput) (bizsync.Connector, error) {
	if s.d.Connectors == nil {
		return bizsync.Connector{}, apierror.Unavailable(adminDomain, "connector repository not configured")
	}
	c, err := s.buildConnector(in, "")
	if err != nil {
		return bizsync.Connector{}, err
	}
	c.ID = s.newID(connectorIDPfx)
	// due 初值 id-hash 撒点（③-1）：全量 connector 同刻建批时 due 波峰拍平到
	// [now, now+interval) 均匀位——用最终 id 计算（确定性幂等）。
	c.NextRunAt = bizsync.NextRunFirstDue(c.ID, c.IntervalSeconds, s.now())
	if err := s.d.Connectors.CreateConnector(ctx, c); err != nil {
		// 软删感知部分唯一（同 collection 活跃唯一）冲突由 data 侧扭 409。
		return bizsync.Connector{}, err
	}
	c.ConfigJSON = bizsync.SanitizeConfigForExport(c.ConfigJSON)
	return c, nil
}

// UpdateConnector 整体 replace（PATCH 语义在 service 层做字段合并；本层收
// 合并后的完整形状）。在途轮拒写：先 Get 命中则失配=409，未命中=404。
func (s *AdminService) UpdateConnector(ctx context.Context, id, callerWorkspace string, in ConnectorInput) (bizsync.Connector, error) {
	if s.d.Connectors == nil {
		return bizsync.Connector{}, apierror.Unavailable(adminDomain, "connector repository not configured")
	}
	existing, err := s.requireConnector(ctx, id, callerWorkspace)
	if err != nil {
		return bizsync.Connector{}, err
	}
	// id（撒点确定性）与运行态字段不可经管理面改写：workspace/collection 由入参
	// 决定 collection；其余运行态（cursor/next_run_at/last_*/consecutive/error/
	// run_token）一律沿用 existing——管理面只改声明面。
	//
	// PATCH 语义（未提供=沿用）：interval_seconds≤0 / config_json 空串 视为调用方
	// 未提供，继承既有值——否则「只改 name」的 PATCH 会被 ValidateConfig 的
	// interval 下限硬闸误伤（校验闸必须看到合并后的完整形状）。
	if in.IntervalSeconds <= 0 {
		in.IntervalSeconds = existing.IntervalSeconds
	}
	if strings.TrimSpace(in.ConfigJSON) == "" {
		in.ConfigJSON = existing.ConfigJSON
	}
	if strings.TrimSpace(in.CollectionID) == "" {
		in.CollectionID = existing.CollectionID
	}
	merged, err := s.buildConnector(in, existing.ID)
	if err != nil {
		return bizsync.Connector{}, err
	}
	merged.ID = existing.ID
	merged.WorkspaceID = existing.WorkspaceID
	merged.Cursor = existing.Cursor
	merged.NextRunAt = existing.NextRunAt
	merged.LastRunAt = existing.LastRunAt
	merged.LastSuccessAt = existing.LastSuccessAt
	merged.ConsecutiveFailures = existing.ConsecutiveFailures
	merged.ErrorCode = existing.ErrorCode
	merged.ErrorMessage = existing.ErrorMessage
	merged.RunToken = existing.RunToken
	merged.CreatedAt = existing.CreatedAt
	if in.WorkspaceID != "" && in.WorkspaceID != existing.WorkspaceID {
		return bizsync.Connector{}, apierror.BadRequest(adminDomain, "workspace_id is immutable")
	}
	ok, err := s.d.Connectors.UpdateConnector(ctx, merged)
	if err != nil {
		return bizsync.Connector{}, err
	}
	if !ok {
		// 已证存在（requireConnector 过）而 CAS 失配 ⇒ 在途轮持有 run_token。
		return bizsync.Connector{}, apierror.Conflict(adminDomain, "connector sync in progress; retry after current run finishes")
	}
	merged.ConfigJSON = bizsync.SanitizeConfigForExport(merged.ConfigJSON)
	return merged, nil
}

// DeleteConnector 软删（⑦：级联软删 credential 由 repo 同笔事务承担；保留已
// 摄取文档）。purge=1 异步入队归 W7 删除墓碑级联，本层不承接。
func (s *AdminService) DeleteConnector(ctx context.Context, id, callerWorkspace string) error {
	if s.d.Connectors == nil {
		return apierror.Unavailable(adminDomain, "connector repository not configured")
	}
	if _, err := s.requireConnector(ctx, id, callerWorkspace); err != nil {
		return err
	}
	ok, err := s.d.Connectors.SoftDeleteConnector(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		// 读后竞态（并发软删）——幂等语义：第二次删除视为已不存在。
		return apierror.NotFound(adminDomain, "connector not found")
	}
	return nil
}

// TriggerSync 手动触发入队（⑤：next_run_at=now() 入队语义＋在途 409；
// run_token 由 scheduler 认领时签发，本层不碰）。
func (s *AdminService) TriggerSync(ctx context.Context, id, callerWorkspace string) error {
	if s.d.Connectors == nil {
		return apierror.Unavailable(adminDomain, "connector repository not configured")
	}
	c, err := s.requireConnector(ctx, id, callerWorkspace)
	if err != nil {
		return err
	}
	if !c.Enabled {
		return apierror.Conflict(adminDomain, "connector disabled")
	}
	if c.SyncState == bizsync.SyncStateAuthPaused {
		return apierror.Conflict(adminDomain, "connector paused on auth failure; update credential to resume")
	}
	accepted, err := s.d.Connectors.TriggerConnectorSync(ctx, id, s.now())
	if err != nil {
		return err
	}
	if !accepted {
		// 读后竞态：另一轮已认领（run_token 非空）或状态漂移到停轮/禁用。
		return apierror.Conflict(adminDomain, "connector sync already in progress")
	}
	return nil
}

// ListRuns 轮次史（逆向分页，idx_src_runs_conn 支撑）。
func (s *AdminService) ListRuns(ctx context.Context, id, callerWorkspace string, limit int) ([]bizsync.Run, error) {
	if s.d.Runs == nil {
		return nil, apierror.Unavailable(adminDomain, "run repository not configured")
	}
	if _, err := s.requireConnector(ctx, id, callerWorkspace); err != nil {
		return nil, err
	}
	return s.d.Runs.ListRuns(ctx, id, clampLimit(limit, 50, 200))
}

// ListItems 指纹行读径（status 过滤支撑 missing 排查；⑦ ?status=missing）。
func (s *AdminService) ListItems(ctx context.Context, id, callerWorkspace, status string, limit int) ([]bizsync.Item, error) {
	if s.d.Items == nil {
		return nil, apierror.Unavailable(adminDomain, "item repository not configured")
	}
	if _, err := s.requireConnector(ctx, id, callerWorkspace); err != nil {
		return nil, err
	}
	st := bizsync.ItemStatus(strings.TrimSpace(status))
	if st == "" {
		st = bizsync.ItemStatusActive
	}
	if !st.Valid() {
		return nil, apierror.BadRequest(adminDomain, "invalid status filter")
	}
	return s.d.Items.ListItemsByStatus(ctx, id, st, clampLimit(limit, 100, 500))
}

// ──────────────────────────────── 凭据子系统（④） ────────────────────────────────

// CredentialInput PUT 凭据入参。Secret 为明文（enc: 加密后落库）或 `env:NAME`
// 备选 scheme（原样落 secret_ref，运行时解析注密）。
type CredentialInput struct {
	CredentialKey string
	Secret        string
	Status        string
	MetadataJSON  string
}

// CredentialView 读侧投影（④ 硬约束：只出 configured+masked_preview，永不返明文）。
type CredentialView struct {
	ID            string    `json:"id"`
	ConnectorID   string    `json:"connector_id"`
	CredentialKey string    `json:"credential_key"`
	Status        string    `json:"status"`
	Configured    bool      `json:"configured"`
	MaskedPreview string    `json:"masked_preview"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// UpsertCredential 凭据写入（④ 铁律链）：校验键形 → 加密闸（env: 除外）→ 落库
// → 唤醒 auth_paused。加密失败即硬错，绝不落明文降级。
func (s *AdminService) UpsertCredential(ctx context.Context, connectorID, callerWorkspace string, in CredentialInput) (CredentialView, error) {
	if s.d.Credentials == nil {
		return CredentialView{}, apierror.Unavailable(adminDomain, "credential repository not configured")
	}
	c, err := s.requireConnector(ctx, connectorID, callerWorkspace)
	if err != nil {
		return CredentialView{}, err
	}
	key := in.CredentialKey // 不 Trim——形状闸必须校验原始值（尾随空白即拒，诚实闸）
	if err := bizsync.ValidateCredentialKey(key); err != nil {
		return CredentialView{}, apierror.BadRequest(adminDomain, err.Error())
	}
	secret := strings.TrimSpace(in.Secret)
	if secret == "" {
		return CredentialView{}, apierror.BadRequest(adminDomain, "secret is required")
	}
	var secretRef string
	if bizsync.IsEnvSecretRef(secret) {
		// 备选 scheme（P1）：env:NAME 原样落库，不经加密闸（值不是秘密本体）。
		if err := bizsync.ValidateEnvSecretRef(secret); err != nil {
			return CredentialView{}, apierror.BadRequest(adminDomain, err.Error())
		}
		secretRef = secret
	} else {
		if s.d.Crypto == nil {
			return CredentialView{}, apierror.Unavailable(adminDomain, "credential crypto not configured")
		}
		// 无 key 拒写硬错（不静默明文降级）。
		if err := s.d.Crypto.RequireKeyForSecret(ctx); err != nil {
			return CredentialView{}, err
		}
		secretRef, err = s.d.Crypto.EncryptChannelSecretRef(ctx, secret)
		if err != nil {
			return CredentialView{}, err
		}
		if secretRef == "" {
			return CredentialView{}, apierror.Internal(adminDomain, "credential encryption produced empty secret_ref")
		}
	}
	status := strings.TrimSpace(in.Status)
	if status == "" {
		status = "active"
	}
	metadata := strings.TrimSpace(in.MetadataJSON)
	if metadata == "" {
		metadata = "{}"
	}
	if !json.Valid([]byte(metadata)) {
		return CredentialView{}, apierror.BadRequest(adminDomain, "metadata_json must be valid JSON")
	}
	row := bizsync.Credential{
		ID:            s.newID(credentialIDPfx),
		ConnectorID:   connectorID,
		CredentialKey: key,
		Status:        status,
		SecretRef:     secretRef,
		MetadataJSON:  metadata,
	}
	if err := s.d.Credentials.UpsertCredential(ctx, row); err != nil {
		return CredentialView{}, err
	}
	// 唤醒链（④）：写成功即自动清 auth_paused/consecutive_failures。仅命中
	// auth_paused 才收——重复 PUT/分类漂移静默 no-op（不报错误读）。
	if c.SyncState == bizsync.SyncStateAuthPaused {
		if _, err := s.d.Connectors.WakeConnectorFromAuthPaused(ctx, connectorID, s.now()); err != nil {
			// 唤醒失败不回滚凭据（写入已成功，下轮 sweep 自然拾取）；
			// 记 Warn 供观测，不劫持 200。
			s.log.Warn("sourcesync credential wake failed",
				loggateway.Str("component", "sourcesync.admin"),
				loggateway.Str("connector_id", connectorID),
				loggateway.Err(err))
		}
	}
	// 回读活的现状（repo 可能原位更新既有行——ID/CreatedAt 以库为准）。
	stored, found, err := s.d.Credentials.GetCredential(ctx, connectorID, key)
	if err != nil {
		return CredentialView{}, err
	}
	if found {
		return credentialToView(stored), nil
	}
	return credentialToView(row), nil
}

// ListCredentials 活性凭据列表（secret_ref 出库前 sanitize，④）。
func (s *AdminService) ListCredentials(ctx context.Context, connectorID, callerWorkspace string) ([]CredentialView, error) {
	if s.d.Credentials == nil {
		return nil, apierror.Unavailable(adminDomain, "credential repository not configured")
	}
	if _, err := s.requireConnector(ctx, connectorID, callerWorkspace); err != nil {
		return nil, err
	}
	rows, err := s.d.Credentials.ListCredentialsByConnector(ctx, connectorID)
	if err != nil {
		return nil, err
	}
	out := make([]CredentialView, 0, len(rows))
	for _, r := range rows {
		out = append(out, credentialToView(r))
	}
	return out, nil
}

// DeleteCredential 单行软删（幂等：不存在视为成功，④ 无 Reveal 面）。
func (s *AdminService) DeleteCredential(ctx context.Context, connectorID, callerWorkspace, credentialKey string) error {
	if s.d.Credentials == nil {
		return apierror.Unavailable(adminDomain, "credential repository not configured")
	}
	if _, err := s.requireConnector(ctx, connectorID, callerWorkspace); err != nil {
		return err
	}
	key := credentialKey // 不 Trim（同 Upsert：形状闸校验原始值）
	if err := bizsync.ValidateCredentialKey(key); err != nil {
		return apierror.BadRequest(adminDomain, err.Error())
	}
	if _, err := s.d.Credentials.SoftDeleteCredential(ctx, connectorID, key); err != nil {
		return err
	}
	return nil
}

// ──────────────────────────────── 内部 ────────────────────────────────

// requireConnector 取址 + 租户隔离（跨租户/不存在 → 404 反 IDOR 枚举）。
func (s *AdminService) requireConnector(ctx context.Context, id, callerWorkspace string) (bizsync.Connector, error) {
	if s.d.Connectors == nil {
		return bizsync.Connector{}, apierror.Unavailable(adminDomain, "connector repository not configured")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return bizsync.Connector{}, apierror.BadRequest(adminDomain, "connector id is required")
	}
	c, found, err := s.d.Connectors.GetConnector(ctx, id)
	if err != nil {
		return bizsync.Connector{}, err
	}
	if !found {
		return bizsync.Connector{}, apierror.NotFound(adminDomain, "connector not found")
	}
	// callerWorkspace 空=系统调用方（cron/admin 后台）——绕过隔离；非空则必须
	// 与行同工作区（共享库 connector 由 system ctx 建，workspace_id=''）。
	if callerWorkspace != "" && c.WorkspaceID != callerWorkspace {
		return bizsync.Connector{}, apierror.NotFound(adminDomain, "connector not found")
	}
	return c, nil
}

// buildConnector 输入 → 实体（含缺省填充）+ 全量校验闸（kind 分派 config 解析）。
// id 由调用方随后赋值（due 撒点依赖最终 id）。
func (s *AdminService) buildConnector(in ConnectorInput, currentID string) (bizsync.Connector, error) {
	kind, err := bizsync.ParseKind(strings.TrimSpace(in.Kind))
	if err != nil {
		return bizsync.Connector{}, apierror.BadRequest(adminDomain, "kind must be shared_dir or notion")
	}
	name := strings.TrimSpace(in.Name)
	collectionID := strings.TrimSpace(in.CollectionID)
	if name == "" {
		return bizsync.Connector{}, apierror.BadRequest(adminDomain, "name is required")
	}
	if collectionID == "" {
		return bizsync.Connector{}, apierror.BadRequest(adminDomain, "collection_id is required")
	}
	config := strings.TrimSpace(in.ConfigJSON)
	if config == "" {
		config = "{}"
	}
	// kind 分派配置校验（②：walker 与 W5 共用唯一口径；notion W6 前只做 JSON 形状）。
	switch kind {
	case bizsync.KindSharedDir:
		if _, err := bizsync.ParseSharedDirConfig(config); err != nil {
			return bizsync.Connector{}, apierror.BadRequest(adminDomain, err.Error())
		}
	default:
		if !json.Valid([]byte(config)) {
			return bizsync.Connector{}, apierror.BadRequest(adminDomain, "config_json must be valid JSON")
		}
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	c := bizsync.Connector{
		ID:                  currentID,
		Kind:                kind,
		Name:                name,
		WorkspaceID:         strings.TrimSpace(in.WorkspaceID),
		CollectionID:        collectionID,
		ConfigJSON:          config,
		IntervalSeconds:     in.IntervalSeconds,
		Enabled:             enabled,
		SyncState:           bizsync.SyncStateIdle,
		MaxItemsPerRun:      in.MaxItemsPerRun,
		MaxDeletesPerRun:    in.MaxDeletesPerRun,
		MaxFetchBytesPerRun: in.MaxFetchBytesPerRun,
	}
	// 缺省填充（③-1 拍板值：预算三列 500/200/512MiB）。
	if c.MaxItemsPerRun <= 0 {
		c.MaxItemsPerRun = bizsync.DefaultMaxItemsPerRun
	}
	if c.MaxDeletesPerRun <= 0 {
		c.MaxDeletesPerRun = bizsync.DefaultMaxDeletesPerRun
	}
	if c.MaxFetchBytesPerRun <= 0 {
		c.MaxFetchBytesPerRun = bizsync.DefaultMaxFetchBytesPerRun
	}
	if err := bizsync.ValidateConfig(c); err != nil {
		return bizsync.Connector{}, apierror.BadRequest(adminDomain, err.Error())
	}
	return c, nil
}

func credentialToView(c bizsync.Credential) CredentialView {
	ref := strings.TrimSpace(c.SecretRef)
	return CredentialView{
		ID:            c.ID,
		ConnectorID:   c.ConnectorID,
		CredentialKey: c.CredentialKey,
		Status:        c.Status,
		Configured:    ref != "",
		MaskedPreview: biz.MaskSecretRef(ref),
		UpdatedAt:     c.UpdatedAt,
	}
}

func clampLimit(v, def, max int) int {
	if v <= 0 {
		return def
	}
	if v > max {
		return max
	}
	return v
}
