// Package sourcesync —— G-38 SourceSync 域内编排层（Walker/Applier 抽象 + Runner 八步序）。
// 拍板锚点：37-knowledge.design.md §G-38 ② Walker/Applier 签名 × ⑤ 调度/租约 ×
// ⑥ 删除八步序 × ⑧ WS 帧缝；biz 类型/枚举/常量唯一落点
// internal/biz/knowledge/sourcesync（本包别名 bizsync 引用，不重复声明）。
//
// 分层纪律：本包不 import internal/service、internal/data——跨副本排他锁契约
// 本地镜像（与 service.KnowledgeJobLocker 同构、由 W4 cron 装配面注入 PG 实现）。
// 实现落位：Walker/Applier 按 kind 注册（W3 shared_dir、W6 notion），
// Runner 的 ⑥ 八步序为本包唯一编排真理。
package sourcesync

import (
	"context"
	"errors"
	"fmt"
	"io"

	bizsync "aranea-agents/internal/biz/knowledge/sourcesync"
)

// ──────────────────────────────── Walk 载荷类型 ────────────────────────────────

// CursorState 游标（设计 ②：prev 为空串=首扫；emit 出错即中止本轮、游标不动——
// 与 vault runner 不存 prev 的契约对齐）。
type CursorState = string

// Snapshot 单条远端快照（walk 出席事实）。ExternalID 是项寻址真相（③-1）；
// DisplayPath 仅进 error_message/UI（禁入 error_code 动态片段——字节稳定纪律①）。
type Snapshot struct {
	ExternalID  string
	Fingerprint string // 远端 rev（Notion last_edited_time / 本地 mtime+size 摘要）
	DisplayPath string
}

// Payload 抽取载荷（设计 ②：显式 MIME 交 ExtractorRegistry 归一化）。
// 所有权：walker 创建 Body，Applier 消费后负责 Close（runner 不介入）。
type Payload struct {
	Body      io.ReadCloser
	MIME      string
	SizeBytes int64
}

// ApplyOutcome 单条提交回执（⑥ 级联唯一正道所需的寻址三件套）。
type ApplyOutcome struct {
	DocumentID   string
	OwnLifecycle bool   // adopt=false（同库同 hash 红线→接管豁免）；新建=true
	ContentHash  string // 提交成功才回填=checkpoint 推进点
}

// ──────────────────────────────── Walker / Applier 抽象 ────────────────────────────────

// Walker 源侧走查器（每 kind 一实现，W3/W6 落）。契约铁律（设计 ②）：
//   - Walk 必须串行调用 emit（runner 统计与 item 写入无锁设计）；
//     emit 返回 error（含 ErrBudgetExhausted 哨兵）即中止本轮、游标不动
//     （实现须返回 prev，不得返回推进值）。
//   - Walk 自体 error=当轮 abort——runner 跳过候选圈定与删除，已出席标记保留
//     （出席是事实，优于全有/全无）。
//   - 凭据/401/403 一律包装 AuthError 返回（含 emit 内 Fetch 失败）。
type Walker interface {
	Kind() bizsync.Kind
	Walk(ctx context.Context, cfg bizsync.Connector, prev CursorState, emit func(Snapshot) error) (CursorState, error)
	Fetch(ctx context.Context, cfg bizsync.Connector, externalID string) (*Payload, error)
	// ConfirmDeleted 三态（P5 拍板 a）：Unknown 保留 missing 不降级删除；
	// shared_dir 以文件系统直查为真相（非 walk 缓冲），Notion 走 pages.retrieve。
	ConfirmDeleted(ctx context.Context, cfg bizsync.Connector, externalID string) bizsync.DeletionVerdict
}

// Applier 知识域提交面（薄封装 vault_sync 提交序列：幂等短路 → 建档/adopt →
// indexing CAS → 提交 chunk/meta；fetch/远端 5xx 不进 embed 熔断计数）。
// Upsert 失败不阻断 walk（runner 记 fetch_failed 桶、item 病态行，下轮重试）。
// Delete 只在 OwnLifecycle=true 应调（级联删文档）；false 时 runner 不调用。
type Applier interface {
	Upsert(ctx context.Context, cfg bizsync.Connector, snap Snapshot, payload *Payload) (ApplyOutcome, error)
	Delete(ctx context.Context, cfg bizsync.Connector, item bizsync.Item) error
}

// ──────────────────────────────── 依赖缝 ────────────────────────────────

// JobLocker 跨副本排他锁面（⑤ fail-closed：PG 抖动宁停不重——外部 API 有配额
// 副作用；与 cron lease fail-open 取向相反且理由已写明）。
// 实现由 W4 cron 装配注入（PG advisory lock）；本包不 import internal/service。
type JobLocker interface {
	TryAcquire(ctx context.Context, jobKey string) (release func(), ok bool, err error)
}

// FrameSink ⑧ WS 帧发布缝（W8 接 SystemNoticeEvent/knowledge_source_sync
// event_type；≤1 started + ≤1 done，auth_paused 状态迁移必发）。Runner 只在
// 事件点触发，节流纪律（≥15s 或 >20% 进度）由实现侧承担。nil→noop。
type FrameSink interface {
	OnRunStarted(ctx context.Context, ev RunEvent)
	OnRunFinished(ctx context.Context, ev RunEvent)
}

// RunEvent 帧载荷（done 帧最终计数即 routes 面板数据面）。
type RunEvent struct {
	ConnectorID string
	Kind        bizsync.Kind
	RunID       string
	RunToken    string
	Stats       RunStats
	Status      bizsync.RunStatus
	ErrorCode   bizsync.ErrorCode
}

// ──────────────────────────────── 错误语义 ────────────────────────────────

// ErrBudgetExhausted 预算哨兵（max_items_per_run/max_fetch_bytes_per_run 触顶
// 时 walker 从 emit 收到并原样返回）——W2 登记裁决：预算只闸提交工作量，
// 不闸 walk 完整性之外的删除安全——命中即中止 walk、游标不动、本轮
// partial 收轮且跳过候选圈定与删除（部分 walk 非缺席权威，与
// walk_empty_abort 同源安全理由）；下轮无记忆重扫自然续传。
var ErrBudgetExhausted = errors.New("sourcesync: per-run budget exhausted")

// AuthError 凭据类失败包装（设计 ⑥-4 整轮闸：任一 401/403 → auth_failed +
// 跳过删除 + auth_paused 立即生效，不进退避序列、不动 next_run_at）。
// Code 细粒度字节稳定码（auth_failed/credential_missing/
// credential_decrypt_failed 之一；其他值或不合法值一律归一 auth_failed）。
type AuthError struct {
	Code bizsync.ErrorCode
	Err  error
}

func (e *AuthError) Error() string { return "sourcesync auth: " + e.Err.Error() }
func (e *AuthError) Unwrap() error { return e.Err }

// NewAuthError 包装入口（walker 侧 401/403/凭据装配失败统一走此；
// code 传空=默认 auth_failed）。
func NewAuthError(code bizsync.ErrorCode, err error) error {
	if err == nil {
		return nil
	}
	return &AuthError{Code: code, Err: err}
}

// AuthCodeOf 提取归一错误码（非 AuthError→false）。
func AuthCodeOf(err error) (bizsync.ErrorCode, bool) {
	var ae *AuthError
	if !errors.As(err, &ae) {
		return "", false
	}
	switch ae.Code {
	case bizsync.ErrCodeCredentialMissing, bizsync.ErrCodeCredentialDecryptFailed:
		return ae.Code, true
	default:
		return bizsync.ErrCodeAuthFailed, true
	}
}

// IsAuthError 判定（⑥-4 闸输入）。
func IsAuthError(err error) bool {
	_, ok := AuthCodeOf(err)
	return ok
}

// ──────────────────────────────── Runner 输出类型 ────────────────────────────────

// RunStats 轮级计数（与 runs 表列一一对应；fetch_failed=拉取+提交合并失败桶
// ——W2 登记口径：applier.Upsert 失败同桶；miss_unknown=Unknown/StillExists/
// 确认出错三条保留路径合并计数）。
type RunStats struct {
	PrevActive  int
	Seen        int
	Created     int
	Updated     int
	Unchanged   int
	FetchFailed int
	Deleted     int
	MissUnknown int
	BytesPulled int64
}

// RunOutcome 轮级终局（skipped 与真实轮次同一出口；run 行只在真实执行过
// 后插入——jobSkipped 不写 runs 表）。
type RunOutcome struct {
	ConnectorID string
	RunID       string
	RunToken    string
	Skipped     bool
	SkipReason  string // 小写蛇形内部枚举：not_found/disabled/locked/claim_lost
	Stats       RunStats
	Status      bizsync.RunStatus
	ErrorCode   bizsync.ErrorCode // 归一后的字节稳定码
	Error       error             // 原始错误（仅内存载体，不入库）
	Finalized   bool              // connector finalize CAS 成功
	ClaimLost   bool              // 收轮/心跳时失地（被回收/软删/管理冲突）
	SyncState   bizsync.SyncState // 收轮回写的目标状态（skipped 时为空）
}

func (o RunOutcome) String() string {
	if o.Skipped {
		return fmt.Sprintf("skipped(%s)", o.SkipReason)
	}
	return fmt.Sprintf("status=%s code=%s seen=%d created=%d updated=%d unchanged=%d fetch_failed=%d deleted=%d miss_unknown=%d bytes=%d",
		o.Status, o.ErrorCode, o.Stats.Seen, o.Stats.Created, o.Stats.Updated, o.Stats.Unchanged,
		o.Stats.FetchFailed, o.Stats.Deleted, o.Stats.MissUnknown, o.Stats.BytesPulled)
}
