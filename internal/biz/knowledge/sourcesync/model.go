// Package sourcesync —— G-38 SourceSync 外部连接器的唯一 biz 落点
// （37-knowledge.design.md §G-38 ②：Config/Item/Run/Credential/DeletionVerdict/Kind
// 类型+校验+字节级稳定枚举集中于此；Walker/Runner/Applier 实现落
// internal/knowledge/sourcesync（W2+），四表裸 SQL hand-repo 落
// internal/data/knowledge_source_sync_repo.go（迁移 20261298–20261301）。
//
// 字节级稳定纪律（§G-38 ③-1 钉）：error_code 与全部状态枚举是
// 监控/告警/UI 的跨版本契约，值集合在此一处声明、纯函数钉遍历断言
// 无路径/hash/页 ID 动态片段——新增值必须同步钉阵与 65 手册。
package sourcesync

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ──────────────────────────────── Kind ────────────────────────────────

// Kind 连接器类型枚举（设计 ③-1 CHECK 两值；新增值须同批迁移扩展 CHECK + 钉阵）。
type Kind string

const (
	KindSharedDir Kind = "shared_dir"
	KindNotion    Kind = "notion"
)

// AllKinds 声明序全集（纯函数钉遍历的唯一取值源）。
func AllKinds() []Kind { return []Kind{KindSharedDir, KindNotion} }

func (k Kind) Valid() bool {
	switch k {
	case KindSharedDir, KindNotion:
		return true
	}
	return false
}

// ParseKind 严格小写匹配（空串/大小写漂移/未知值一律拒）。
func ParseKind(s string) (Kind, error) {
	k := Kind(s)
	if !k.Valid() {
		return "", fmt.Errorf("sourcesync: invalid kind %q", s)
	}
	return k, nil
}

// ──────────────────────────────── SyncState ────────────────────────────────

// SyncState 连接器调度状态机（设计 ③-1 四值：idle|running|auth_paused|error）。
// auth_paused 只由凭据闸进入（退避分级表⑤：不重试烧配额），PUT credential 唤醒。
type SyncState string

const (
	SyncStateIdle       SyncState = "idle"
	SyncStateRunning    SyncState = "running"
	SyncStateAuthPaused SyncState = "auth_paused"
	SyncStateError      SyncState = "error"
)

func AllSyncStates() []SyncState {
	return []SyncState{SyncStateIdle, SyncStateRunning, SyncStateAuthPaused, SyncStateError}
}

func (s SyncState) Valid() bool {
	switch s {
	case SyncStateIdle, SyncStateRunning, SyncStateAuthPaused, SyncStateError:
		return true
	}
	return false
}

// ──────────────────────────────── ItemStatus ────────────────────────────────

// ItemStatus 指纹行状态（设计 ③-2 三值；删除候选拾取谓词=missing）。
type ItemStatus string

const (
	ItemStatusActive  ItemStatus = "active"
	ItemStatusMissing ItemStatus = "missing"
	ItemStatusDeleted ItemStatus = "deleted"
)

func AllItemStatuses() []ItemStatus {
	return []ItemStatus{ItemStatusActive, ItemStatusMissing, ItemStatusDeleted}
}

func (s ItemStatus) Valid() bool {
	switch s {
	case ItemStatusActive, ItemStatusMissing, ItemStatusDeleted:
		return true
	}
	return false
}

// ──────────────────────────────── RunStatus ────────────────────────────────

// RunStatus 轮次终态（设计 ③-4 四值；运行中无行——在途态由
// connectors.run_token 承载，runs 表只记 finalize 后的轮次史）。
type RunStatus string

const (
	RunStatusSuccess RunStatus = "success"
	RunStatusPartial RunStatus = "partial"
	RunStatusFailed  RunStatus = "failed"
	RunStatusSkipped RunStatus = "skipped"
)

func AllRunStatuses() []RunStatus {
	return []RunStatus{RunStatusSuccess, RunStatusPartial, RunStatusFailed, RunStatusSkipped}
}

func (s RunStatus) Valid() bool {
	switch s {
	case RunStatusSuccess, RunStatusPartial, RunStatusFailed, RunStatusSkipped:
		return true
	}
	return false
}

// ──────────────────────────────── DeletionVerdict ────────────────────────────────

// DeletionVerdict 删除确认三态（设计 ⑥-5/P5 拍板 a：Unknown 保留 missing
// 不降级删除；Notion retrieve 404/archived=Confirmed，余=Unknown；
// shared_dir 文件系统 walk-absent 即 Confirmed）。
type DeletionVerdict int8

const (
	VerdictUnknown     DeletionVerdict = 0
	VerdictConfirmed   DeletionVerdict = 1
	VerdictStillExists DeletionVerdict = 2
)

func (v DeletionVerdict) Valid() bool {
	switch v {
	case VerdictUnknown, VerdictConfirmed, VerdictStillExists:
		return true
	}
	return false
}

func (v DeletionVerdict) String() string {
	switch v {
	case VerdictUnknown:
		return "unknown"
	case VerdictConfirmed:
		return "confirmed"
	case VerdictStillExists:
		return "still_exists"
	}
	return fmt.Sprintf("deletion_verdict(%d)", int(v))
}

// ──────────────────────────────── ErrorCode ────────────────────────────────

// ErrorCode 字节级稳定错误码（设计 ③-1：纯函数钉遍历断言无动态片段；
// 归一化后写入 connectors.error_code / runs.error_code，绝不拼接实例信息
// ——doc id/path/hash 等动态信息只进 error_message（≤512 截断））。
type ErrorCode string

const (
	ErrCodeNone                    ErrorCode = ""
	ErrCodeAuthFailed              ErrorCode = "auth_failed"
	ErrCodeCredentialDecryptFailed ErrorCode = "credential_decrypt_failed"
	ErrCodeCredentialMissing       ErrorCode = "credential_missing"
	ErrCodeWalkFailed              ErrorCode = "walk_failed"
	ErrCodeFetchFailed             ErrorCode = "fetch_failed"
	ErrCodeConfirmFailed           ErrorCode = "confirm_failed"
	ErrCodeWalkEmptyAbort          ErrorCode = "walk_empty_abort"
)

// AllErrorCodes 非空错误码全集（声明序；钉阵按此遍历）。
func AllErrorCodes() []ErrorCode {
	return []ErrorCode{
		ErrCodeAuthFailed,
		ErrCodeCredentialDecryptFailed,
		ErrCodeCredentialMissing,
		ErrCodeWalkFailed,
		ErrCodeFetchFailed,
		ErrCodeConfirmFailed,
		ErrCodeWalkEmptyAbort,
	}
}

// errorCodeShape 蛇形小写字面量形状守卫（钉阵复用；禁分隔符外数字段、
// 禁路径斜杠、禁模板花括——三者分别拦截「带实例号段」「带路径」「带动态插值」）。
var errorCodeShape = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)

// ErrorCodeByteStable 字节稳定判定（钉阵+写侧统一闸）：形状合法且在全集内。
func (c ErrorCode) ByteStable() bool {
	if c == ErrCodeNone {
		return true
	}
	if !errorCodeShape.MatchString(string(c)) {
		return false
	}
	for _, v := range AllErrorCodes() {
		if c == v {
			return true
		}
	}
	return false
}

// ──────────────────────────────── 常量（拍板项落值） ────────────────────────────────

const (
	// MinIntervalSeconds 调度间隔下限（设计 ③-1/⑤，创建/PATCH 校验闸）。
	MinIntervalSeconds = 900
	// DefaultMaxItemsPerRun 默认轮内新增/更新预算（abuse 爆炸半径截断）。
	DefaultMaxItemsPerRun = 500
	// DefaultMaxDeletesPerRun 默认轮内删除预算（误删爆炸半径截断，超预算留 miss 下轮）。
	DefaultMaxDeletesPerRun = 200
	// DefaultMaxFetchBytesPerRun 默认轮内拉取字节预算（设计未给初值，按
	// 单文件 32MB 白名单上限 × 约一轮新增批的经验上界取 512MiB，W1 登记
	// 可调参）。不改变行为面曲线，仅防单轮失控。
	DefaultMaxFetchBytesPerRun int64 = 512 << 20
	// RunKeep 轮次保留数（P10 拍板 a 常量钉死：finalize 同事务 DELETE 保
	// 每 connector 最近 100 轮）。
	RunKeep = 100
	// ItemRetention missing/deleted 指纹行保留窗（P10 姊妹条款：同事务
	// 第二 DELETE，超窗才硬清）。
	ItemRetention = 90 * 24 * time.Hour

	// DefaultRunLeaseTTL 轮次租约默认 TTL（设计未给初值，W2 登记可调参：
	// 30min——远超单轮预算内典型耗时；认领时间戳复用 updated_at 承载，
	// 长轮由 Runner 心跳续期，崩溃回收误伤窗口≈心跳间隔+TTL）。
	DefaultRunLeaseTTL = 30 * time.Minute

	// maxErrorMessageRunes error_message 截断上限（③-1 ≤512 截断，按 rune 防
	// 多字节截半）。
	maxErrorMessageRunes = 512
)

// BackoffMinutes 失败退避（③-1 铁律：next_run_at=now()+2^consecutive_failures
// 分钟，cap 60；memory_fact sweeper 先例对齐）。
func BackoffMinutes(consecutiveFailures int) int {
	if consecutiveFailures < 0 {
		consecutiveFailures = 0
	}
	if consecutiveFailures >= 6 {
		return 60
	}
	m := 1 << consecutiveFailures
	if m > 60 {
		return 60
	}
	return m
}

// BackoffDuration 时间量形态（2^n 分钟 cap 60）。
func BackoffDuration(consecutiveFailures int) time.Duration {
	return time.Duration(BackoffMinutes(consecutiveFailures)) * time.Minute
}

// TruncateErrorMessage error_message ≤512 rune 截断（③-1；UTF-8 安全）。
func TruncateErrorMessage(s string) string {
	r := []rune(s)
	if len(r) <= maxErrorMessageRunes {
		return s
	}
	return string(r[:maxErrorMessageRunes])
}

// ──────────────────────────────── 实体 ────────────────────────────────

// Connector 连接器主表行投影（知识域专用：workspace 间隔离闸在 service
// 读写断言，本层只做字段投影；字段序=迁移 20261298 列序）。
type Connector struct {
	ID                  string
	Kind                Kind
	Name                string
	WorkspaceID         string
	CollectionID        string
	ConfigJSON          string // 仅非密配置（③-1；凭据一律走 credential 行表）
	IntervalSeconds     int
	Enabled             bool
	SyncState           SyncState
	Cursor              string
	NextRunAt           time.Time
	LastRunAt           time.Time
	LastSuccessAt       time.Time
	ConsecutiveFailures int
	ErrorCode           ErrorCode
	ErrorMessage        string
	MaxItemsPerRun      int
	MaxDeletesPerRun    int
	MaxFetchBytesPerRun int64
	RunToken            string // 当前轮认领 CAS（W2）
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// Item 指纹行投影（items 表唯一寻址真相，P3 拍板 a：
// (connector_id, external_id)→document_id 是更新/删除唯一寻址与管辖权边界）。
type Item struct {
	ConnectorID   string
	ExternalID    string
	DocumentID    string
	Fingerprint   string // 远端 rev（Notion last_edited_time / 本地 mtime+size 摘要）
	ContentHash   string // 提交成功才回填=checkpoint 推进点（对齐 vault 落库即 checkpoint）
	OwnLifecycle  bool   // adopt=false（同库同 hash 建档红线→接管豁免）
	Status        ItemStatus
	MissCount     int
	LastSeenRunID string
	ErrorMessage  string // 出席即清（Upsert 语义钉③-2）
	FirstSeenAt   time.Time
	LastSeenAt    time.Time
	DeletedAt     time.Time
}

// Credential 凭据行投影（P1 拍板 A：channel_credential 同构行表；
// secret_ref 仅存 enc: 信封，Sensitive——recordAudit Summary 禁记、
// 读侧 API 只返 configured+masked_preview，永不返明文、不建 Reveal 端点）。
type Credential struct {
	ID            string
	ConnectorID   string
	CredentialKey string
	Status        string
	SecretRef     string
	MetadataJSON  string // workspace_id/bot_id 等非密元数据+OAuth refresh 预留落点
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Run 轮次行投影（finalize 终态行；插入同事务 P10 双保留 DELETE 由 repo 承担）。
type Run struct {
	ID           string
	ConnectorID  string
	StartedAt    time.Time
	FinishedAt   time.Time
	Status       RunStatus
	Created      int
	Updated      int
	Unchanged    int
	FetchFailed  int
	Deleted      int
	MissUnknown  int
	BytesPulled  int64
	ErrorCode    ErrorCode
	ErrorMessage string
}

// ValidateConfig 创建/PATCH 共用校验闸（⑤ 接入面与 W5 HTTP 层共用，
// 落 biz 唯一处）：kind/name/collection 非空 + interval≥900 + 三预算>0。
// 铁律：error_code 若带值必须 ByteStable（写侧一致性硬闸）。
func ValidateConfig(c Connector) error {
	if !c.Kind.Valid() {
		return fmt.Errorf("sourcesync: invalid kind %q", c.Kind)
	}
	if strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("sourcesync: name required")
	}
	if strings.TrimSpace(c.CollectionID) == "" {
		return fmt.Errorf("sourcesync: collection_id required")
	}
	if c.IntervalSeconds < MinIntervalSeconds {
		return fmt.Errorf("sourcesync: interval_seconds %d below minimum %d", c.IntervalSeconds, MinIntervalSeconds)
	}
	if c.MaxItemsPerRun <= 0 || c.MaxDeletesPerRun <= 0 || c.MaxFetchBytesPerRun <= 0 {
		return fmt.Errorf("sourcesync: budgets must be positive")
	}
	if c.ErrorCode != ErrCodeNone && !c.ErrorCode.ByteStable() {
		return fmt.Errorf("sourcesync: error_code %q not byte-stable", c.ErrorCode)
	}
	return nil
}

// ──────────────────────────────── 分 kind 配置（W3；config_json 仅非密） ────────────────────────────────

// SharedDirConfig shared_dir 连接器配置投影（config_json 唯一合法形状；
// 凭据/密钥键的出现即拒——凭据一律走 credential 行表③-3）。
type SharedDirConfig struct {
	Root string `json:"root"` // 监视根目录（绝对路径；walker 侧 resolve+stat 校验存在性）
}

// 敏感键黑名单（拒写入 config_json 的明文秘密形态；enc: 信封走行表不经此）。
var sharedDirForbiddenKeys = []string{"token", "secret", "password", "credential", "api_key"}

// ParseSharedDirConfig 解析+校验 config_json（② 签名级契约：walker/W5 共用唯一口径）。
// 空 root/非法 JSON/未知字段/敏感键一律报错——配置缺陷归 walk_failed（非 auth 家族）。
// 顺序裁决（W3 评审修 #2/#4）：probe 先解 → 敏感键黑名单扫（保证可触达——typed decode
// 的 DisallowUnknownFields 会先一步吞掉 secret-ish 键，后置即死代码）→ 严格 decode
// （未知字段拒）→ root Trim+非空+绝对路径硬闸（评审修 #1/#W3-F4：相对 root 随进程
// CWD 漂移是配置事故，声明形态即绝对路径，相对值宁拒不歧义）。
func ParseSharedDirConfig(raw string) (SharedDirConfig, error) {
	var cfg SharedDirConfig
	if strings.TrimSpace(raw) == "" {
		return cfg, fmt.Errorf("sourcesync: shared_dir config_json required")
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		return cfg, fmt.Errorf("sourcesync: shared_dir config_json invalid: %w", err)
	}
	for k := range probe {
		lk := strings.ToLower(k)
		for _, bad := range sharedDirForbiddenKeys {
			if strings.Contains(lk, bad) {
				return cfg, fmt.Errorf("sourcesync: config_json forbids secret-ish key %q (use credential rows)", k)
			}
		}
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return cfg, fmt.Errorf("sourcesync: shared_dir config_json invalid: %w", err)
	}
	cfg.Root = strings.TrimSpace(cfg.Root)
	if cfg.Root == "" {
		return cfg, fmt.Errorf("sourcesync: shared_dir root required")
	}
	if !filepath.IsAbs(cfg.Root) {
		return cfg, fmt.Errorf("sourcesync: shared_dir root must be absolute, got %q", cfg.Root)
	}
	return cfg, nil
}

// ──────────────────────────────── 仓口（P6 拍板 a：hand-repo 裸 SQL） ────────────────────────────────

// 仓口布尔三元组语义统一：(值, 命中, 错误)——biz 不依赖 apierror 分级；
// data 侧把 not-found 收窄为 found=false（board_run_lease InMem 对齐口径）。

// ConnectorRepo 连接器主表 CRUD。时间戳由 data 侧时钟落值（lease 先例）。
type ConnectorRepo interface {
	// CreateConnector 落新行（调用方先过 ValidateConfig + 生成 id/due 初值
	// id-hash 撒点）；软删感知部分唯一（同 collection 活跃唯一）冲突即错误返回。
	CreateConnector(ctx context.Context, c Connector) error
	// GetConnector 活性行取址（deleted_at=''）。
	GetConnector(ctx context.Context, id string) (Connector, bool, error)
	// UpdateConnector 整体 replace（updated_at 翻钟）；未命中/已软删 found=false。
	UpdateConnector(ctx context.Context, c Connector) (bool, error)
	// SoftDeleteConnector 软删 connector + 级联软删本 connector 全部凭据
	//（⑦ DELETE 默认级联同笔事务；recordAudit 由 service 层 W5 承担）。
	SoftDeleteConnector(ctx context.Context, id string) (bool, error)
	// ListConnectorsByWorkspace 工作区活性列表（管理面读径；含 auth_paused/error）。
	ListConnectorsByWorkspace(ctx context.Context, workspaceID string) ([]Connector, error)
	// ListDueConnectors due 拾取（⑤ 部分索引谓词+残余：enabled ∧ 未软删
	// ∧ next_run_at<=now ∧ sync_state NOT IN (running, auth_paused)——
	// running 防双跑、auth_paused 防烧配额；ORDER BY next_run_at ASC LIMIT n）。
	ListDueConnectors(ctx context.Context, now time.Time, limit int) ([]Connector, error)

	// ── W2 认领/收轮（claim CAS + finalize CAS guard） ──

	// TryClaimRun 轮次认领 CAS（⑤ 唯一入口）：原子门＝enabled ∧ 未软删 ∧
	// （sync_state ∈ {idle, error}）∨ （running ∧ updated_at < now-leaseTTL——
	// 窘迫尸窃取，claim 时间戳复用 updated_at 承载，免新迁移列）。前半守新轮
	// 认旧轮，后半允许稳定回收。Returns：
	//   - (true, nil) 本调用赢得本轮 token（连接行接管）；
	//   - (false, nil) 已有活跃持有者（含同一 run_token 的并发重复调用也 false——
	//     成功路径只返回一次 true）。
	// 副作用：run_token=新 token、sync_state='running'、updated_at=now
	// （updated_at 自此承担租约心跳语义：长轮由 Runner 周期续期，防误回收）。
	TryClaimRun(ctx context.Context, id, runToken string, leaseTTL time.Duration, now time.Time) (bool, error)

	// HeartbeatClaim 租约续期（长轮保活，防 ReclaimZombieRuns/TryClaimRun 窃取）：
	// WHERE id AND run_token AND sync_state='running' 三重守卫 → updated_at=now。
	// 失地（found=false）即本轮已被回收/软删，Runner 据此早退（收轮 CAS 仍兜底）。
	HeartbeatClaim(ctx context.Context, id, runToken string, now time.Time) (bool, error)

	// FinalizeRunWithClaim 收轮 CAS 守卫：只有自己签发的 run_token 可 finalize；
	// WHERE id AND run_token = expectedToken 双条件——失配即 found=false
	//（本轮已被他人接管 / connector 已 soft-delete）。副作用：run_token=''，
	// 其余字段由调用方组装（sync_state/next_run_at/last_run_at/
	// last_success_at/error_code/error_message/consecutive_failures/cursor/updated_at）。
	// 状态机定值规则：
	//   success       sync_state='idle',   error_code='',   consecutive=0, next_run_at=now+interval, last_success_at=now
	//   partial       同 success 但 fetch_failed>0（终端 enable 保留）
	//   failed        sync_state='error', consecutive++, next_run_at=now+2^f min cap 60
	//   auth_paused   sync_state='auth_paused', 不动 next_run_at（等唤醒），consecutive 不变
	//   skipped       内部 no-op finalize（供空轮/aborted 尾调用）
	FinalizeRunWithClaim(ctx context.Context, c Connector, expectedRunToken string) (bool, error)

	// ReclaimZombieRuns 崩溃回收扫描（⑤ 崩溃回收=lease_ttl 过期置 idle；对
	// 调度 startup 与周期 tick 皆可调用）：重置过期 running 行为 idle，让下轮
	// due 拾取可再认领。谓词：sync_state='running' ∧ updated_at < now-leaseTTL
	//（updated_at=认领时间戳，Runner 心跳续期后的长轮不被误收）。
	// 副作用：sync_state='idle'、run_token=''、updated_at=now、
	// consecutive_failures+=1（僵死非瞬态轻罚）；不动 error_code/last_run_at
	//（保留下次错误上下文可见）。Returns 回收数。
	// 方案选择：TryClaimRun 内嵌僵死窃取是本方法的主路径；本扫描是观测面 + 抢救
	//（MetricsSource 挂监控），claim 已窃取则本方法返回 0 行。
	ReclaimZombieRuns(ctx context.Context, now time.Time, leaseTTL time.Duration) (int, error)
}

// ItemRepo 指纹行 CRUD（Upsert 语义钉③-2：出席即 status=active、
// miss_count=0、error_message 清空——flap 边界不触发删除）。
type ItemRepo interface {
	// UpsertItemSeen 出席上报：插入或原位复位（fingerprint/last_seen_* 翻页，
	// miss_count 归零、error_message 清空、status 翻 active、deleted_at 清位
	// ——源复活=生命周期重开；document_id/content_hash/own_lifecycle 不被
	// 出席覆盖——绑定另走 Set* 方法）。
	UpsertItemSeen(ctx context.Context, connectorID, externalID, documentID, fingerprint string, ownLifecycle bool, lastSeenRunID string, seenAt time.Time) error
	// GetItem 单行取址。
	GetItem(ctx context.Context, connectorID, externalID string) (Item, bool, error)
	// SetItemDocument 绑定 document_id/own_lifecycle（adopt 登记/创建落档后回填）。
	SetItemDocument(ctx context.Context, connectorID, externalID, documentID string, ownLifecycle bool) (bool, error)
	// SetItemContentHash 提交成功才回填（checkpoint 推进点）。
	SetItemContentHash(ctx context.Context, connectorID, externalID, contentHash string) (bool, error)
	// SetItemError 逐条记账（fetch 失败等 per-item 错误；出席自动清空）。
	SetItemError(ctx context.Context, connectorID, externalID, message string) (bool, error)
	// ListItemsByStatus 删除候选拾取径（WHERE status=?，missing 支撑部分索引）。
	ListItemsByStatus(ctx context.Context, connectorID string, status ItemStatus, limit int) ([]Item, error)
	// CountActiveItems walk_empty_abort 闸上轮 active 计数（⑥-4 通用闸）。
	CountActiveItems(ctx context.Context, connectorID string) (int, error)
	// MarkAbsentItemsMissing ⑥-3 候选圈定：active ∧ last_seen_run_id<>runToken
	// → status=missing、miss_count+=1（返回圈定数）。
	MarkAbsentItemsMissing(ctx context.Context, connectorID, runToken string) (int, error)
	// MarkItemDeleted ⑥-8 收尾：status=deleted + deleted_at 落钟。
	MarkItemDeleted(ctx context.Context, connectorID, externalID string, when time.Time) (bool, error)
}

// CredentialRepo 凭据行表 CRUD（P1：secret_ref 仅存 enc: 信封；软删重建
// 受 partial-unique 活性唯一保护，冲突=409 级错误）。
type CredentialRepo interface {
	// UpsertCredential ON CONFLICT partial-arbiter (connector_id, credential_key)
	// WHERE deleted_at='' DO UPDATE——软删墓碑行不参与仲裁（先行软删再创=新行）。
	UpsertCredential(ctx context.Context, c Credential) error
	// GetCredential 活性行取址。
	GetCredential(ctx context.Context, connectorID, credentialKey string) (Credential, bool, error)
	// SoftDeleteCredential 单行软删（recordAudit 由 service 层 W5 承担）。
	SoftDeleteCredential(ctx context.Context, connectorID, credentialKey string) (bool, error)
	// ListCredentialsByConnector 活性凭据列表（管理面；secret_ref 出库前
	// 由 service sanitize，永不原文上 API）。
	ListCredentialsByConnector(ctx context.Context, connectorID string) ([]Credential, error)
}

// RunRepo 轮次史 CRUD。
type RunRepo interface {
	// InsertRunWithRetention finalize 唯一入库口：同事务 INSERT run 行
	// + DELETE 超 RunKeep 旧轮（P10 常量钉死，idx_src_runs_conn 支撑）
	// + DELETE 超 ItemRetention 的 missing/deleted 指纹行（姊妹条款第二 DELETE）。
	InsertRunWithRetention(ctx context.Context, run Run) error
	// ListRuns 逆向分页（idx_src_runs_conn (connector_id, started_at DESC) 支撑）。
	ListRuns(ctx context.Context, connectorID string, limit int) ([]Run, error)
}
