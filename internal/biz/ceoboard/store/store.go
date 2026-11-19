// Package store 提供 ceoboard 的持久化底座(PG/Ent 版,自 ceo-dashboard SQLite 移植)。
// 表设计(Ent schema 见 internal/data/ent/schema/ceoboard_*.go):
//
//	ceoboard_events             — 归一化事件(NATS/webhook 统一落库);event_uid 主键幂等
//	ceoboard_webhook_deliveries — webhook 投递流水(存储主键 = source:幂等键 复合串,(source,key) 维度幂等,重启后去重语义不丢)
//	ceoboard_dead_letters       — 死信(接收失败/处理失败),支持重放;同键未重放幂等由库层部分唯一索引兜底
//	ceoboard_audit              — 高危操作审计(前门语义由 biz 层执行,fail-closed)
//	ceoboard_latency_samples    — latency 物化样本((metric_id,b_uid) 唯一,B 侧幂等)
//
// 移植保真要点(11 条风险点):
//
//	① event_uid 主键幂等 + "重复投递时 payload_complete 0→1 单调补齐"(完整重投可升级截断行,完整行首写不变);
//	③ UTC+8 闭日归档 + 迟到保护 + 热数据删除 RowsAffected==1 守卫(archive.go);
//	④ spool 崩溃恢复(.importing 认领、撕裂尾隔离、逐行 sha256)(spool.go);
//	⑦ AllowedSubjects 字面值白名单(不解释 NATS 通配符)(events.go);
//	⑨ 死信同 (kind,source,ref_key) 未重放幂等返回已有 id(进程内 dlMu + 库层部分唯一索引双保险,评审 2026-11-09 M2);
//	⑪ payload TEXT 保真 + 审计截断口径 128/64/256/1024。
//
// 评审 2026-11-09 MEDIUM-7:投递幂等维度化为 (source,key)——原全局幂等键主键在
// 不同源产生相同键(同 event_id / 同实体 id / 同 body 摘要回退)时跨源冲突,后到的
// 投递被静默去重(事件丢失)。存储主键改为 source:幂等键 复合串(拼接收口在本包
// deliveryStorageID,与 ceoboard_events 的 event_uid = 平台:键 同构;读侧剥前缀还原,
// 外部一律只见裸幂等键);存量裸键行由迁移 20261311 回填,未回填残留读取恒等兼容。
//
// 时钟口径:occurred_at 取自事件信封(NULL=未带,读侧退 received_at),received_at 取本机;
// PG timestamptz 取代原 unix 纳秒 int64(同一瞬间,语义不变)。
//
// sessions 表废弃不迁:aranea 已 JWT 无状态 + token_version 吊销(P1)。
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	entsql "entgo.io/ent/dialect/sql"

	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/ceoboardaudit"
	"aranea-agents/internal/data/ent/ceoboarddeadletter"
	"aranea-agents/internal/data/ent/ceoboardevent"
	"aranea-agents/internal/data/ent/ceoboardlatencysample"
	"aranea-agents/internal/data/ent/ceoboardwebhookdelivery"
)

// Store 包装 Ent 客户端(PG);rawDB 供容量类指标直查(pg_database_size),可空。
type Store struct {
	client *ent.Client
	rawDB  *sql.DB

	eventWriteMu     sync.Mutex // 归档删除与补齐升级互斥(降低 RowsAffected 守卫流产率)
	dlMu             sync.Mutex // 死信"查重再插"两步骤串行化(原单 SQL 原子语义的进程内等价)
	spool            *spool
	archiveDir       string
	retentionDays    int
	dbCapacityBytes  int64
	archDedup        *archiveDedupIndex // 归档后重投查重的惰性闭日索引(L6,见 archive.go)
	spoolWriteFailed atomic.Uint64
	auditWriteFailed atomic.Uint64
}

// New 构造存储;client 由装配层注入(共享 Ent 客户端,本包不负责关闭)。
// 默认:retention 90 天,容量告警基线 10GiB(ConfigureArchive 可改)。
func New(client *ent.Client, rawDB *sql.DB) *Store {
	return &Store{client: client, rawDB: rawDB, retentionDays: 90, dbCapacityBytes: 10 * 1024 * 1024 * 1024, archDedup: newArchiveDedupIndex()}
}

// Client 返回共享 Ent 客户端，供只读经营聚合使用；调用方不得关闭。
func (s *Store) Client() *ent.Client {
	if s == nil {
		return nil
	}
	return s.client
}

// eventToDTO 把 Ent 实体映射为 biz DTO(occurred_at NULL → 零值)。
func eventToDTO(e *ent.CeoboardEvent) Event {
	ev := Event{
		UID:               e.ID,
		Platform:          e.Platform,
		Subject:           e.Subject,
		SubjectKey:        e.SubjectKey,
		EventID:           e.EventID,
		EntityID:          e.EntityID,
		ReceivedAt:        e.ReceivedAt.UTC(),
		Payload:           e.Payload,
		ImportedFromSpool: e.ImportedFromSpool,
		PayloadComplete:   e.PayloadComplete,
	}
	if e.OccurredAt != nil {
		ev.OccurredAt = e.OccurredAt.UTC()
	}
	return ev
}

// InsertEvent 幂等落库一条事件;返回 true 表示新插入(false = 重复投递被去重)。
func (s *Store) InsertEvent(ctx context.Context, ev *Event) (bool, error) {
	return s.insertEvent(ctx, ev, false)
}

func (s *Store) insertEvent(ctx context.Context, ev *Event, fromSpool bool) (bool, error) {
	// 评审 2026-11-09 L6:热库按 event_uid 主键去重,归档硬删除会关闭该窗口;
	// 老信封事件先查归档,命中即判重(走与主键冲突相同的"非新插入"出口),
	// 重放不再重复计数。RestoreArchive 直调 insertEventWithFlags,天然旁路。
	if s.archivedEventExists(ev) {
		return false, nil
	}
	return s.insertEventWithFlags(ctx, ev, fromSpool, true)
}

// sanitizeText 消毒写入 PG TEXT 列的自由文本(评审 2026-11-09 H3):非法
// UTF-8 序列与 NUL 一律替换为 U+FFFD——PG 对二者直接拒收(SQLSTATE 22021),
// 消毒前一条毒消息即可让死信/事件写入永久失败并引发无限重投。其余内容
// 原样保留(可检索、可定位)。合法输入零分配直返。
func sanitizeText(s string) string {
	s = strings.ToValidUTF8(s, "�")
	if strings.IndexByte(s, 0) >= 0 {
		s = strings.ReplaceAll(s, "\x00", "�")
	}
	return s
}

func (s *Store) insertEventWithFlags(ctx context.Context, ev *Event, fromSpool, complete bool) (bool, error) {
	s.eventWriteMu.Lock()
	defer s.eventWriteMu.Unlock()

	// 消毒一切源自消息字节的自由文本列(H3;PG TEXT 拒收非法 UTF-8/NUL)——含
	// 主键:同一原始 uid 恒映射到同一消毒 uid,幂等语义不变。
	uid := sanitizeText(ev.UID)
	payload := sanitizeText(ev.Payload)
	eventID := sanitizeText(ev.EventID)
	entityID := sanitizeText(ev.EntityID)

	c := s.client.CeoboardEvent.Create().
		SetID(uid).
		SetPlatform(ev.Platform).
		SetSubject(ev.Subject).
		SetSubjectKey(ev.SubjectKey).
		SetEventID(eventID).
		SetEntityID(entityID).
		SetReceivedAt(ev.ReceivedAt.UTC()).
		SetPayload(payload).
		SetImportedFromSpool(fromSpool).
		SetPayloadComplete(complete)
	if !ev.OccurredAt.IsZero() {
		// occurred_at 零值归一为 NULL:读侧(ListPairEvents 等)以 NULL 判断
		// "信封未带时间"并回退 received_at(原 occ==0 口径)。
		t := ev.OccurredAt.UTC()
		c.SetNillableOccurredAt(&t)
	}
	err := c.Exec(ctx)
	if err == nil {
		return true, nil
	}
	if !ent.IsConstraintError(err) {
		return false, fmt.Errorf("事件落库: %w", err)
	}
	// 重复投递(event_uid 冲突)→ 去重
	if !complete {
		return false, nil
	}
	// 升级前的同 UID 行可能只有截断原文;完整重投可单调补齐,完整行仍首写不变。
	// imported_from_spool 取 MAX(原值, 新值):任一侧经 spool 即保留标记。
	// (原为单条 UPDATE ... WHERE payload_complete=0;Ent 版先读后条件更新,
	// 由 eventWriteMu 串行化,单写入者语义等价。)
	existing, err := s.client.CeoboardEvent.Get(ctx, uid)
	if err != nil {
		return false, fmt.Errorf("补齐前读取历史事件: %w", err)
	}
	if err := s.client.CeoboardEvent.Update().
		Where(ceoboardevent.ID(uid), ceoboardevent.PayloadCompleteEQ(false)).
		SetPayload(payload).
		SetPayloadComplete(true).
		SetImportedFromSpool(existing.ImportedFromSpool || fromSpool).
		Exec(ctx); err != nil {
		return false, fmt.Errorf("补齐历史事件原文: %w", err)
	}
	return false, nil
}

// CountEvents 统计窗口内某 subject_key(或整平台,subjectKey 为空)的事件数。
// 按 received_at 计数(看板视角的吞吐);occurred_at 用于乱序分析,不参与窗口。
func (s *Store) CountEvents(ctx context.Context, platform, subjectKey string, since time.Time) (int, error) {
	q := s.client.CeoboardEvent.Query().
		Where(ceoboardevent.PlatformEQ(platform), ceoboardevent.ReceivedAtGTE(since.UTC()))
	if subjectKey != "" {
		q = q.Where(ceoboardevent.SubjectKeyEQ(subjectKey))
	}
	return q.Count(ctx)
}

// RecentEvents 返回某平台最近事件(新→旧)。
func (s *Store) RecentEvents(ctx context.Context, platform string, limit int) ([]Event, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.client.CeoboardEvent.Query().
		Where(ceoboardevent.PlatformEQ(platform)).
		Order(ent.Desc(ceoboardevent.FieldReceivedAt)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Event, 0, len(rows))
	for _, e := range rows {
		out = append(out, eventToDTO(e))
	}
	return out, nil
}

// RecentEventsBySubjects 返回某平台、限定 subject_key 集合的最近事件(新→旧)。
// 下钻按节点收窄用:空集合直接返回空,不做全平台查询(防跨工作流泄漏)。
func (s *Store) RecentEventsBySubjects(ctx context.Context, platform string, subjectKeys []string, limit int) ([]Event, error) {
	if len(subjectKeys) == 0 {
		return []Event{}, nil
	}
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.client.CeoboardEvent.Query().
		Where(ceoboardevent.PlatformEQ(platform), ceoboardevent.SubjectKeyIn(subjectKeys...)).
		Order(ent.Desc(ceoboardevent.FieldReceivedAt)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Event, 0, len(rows))
	for _, e := range rows {
		out = append(out, eventToDTO(e))
	}
	return out, nil
}

// TotalEvents 返回事件总数(健康自检用)。
func (s *Store) TotalEvents(ctx context.Context) (int, error) {
	return s.client.CeoboardEvent.Query().Count(ctx)
}

// ---------- webhook 投递 ----------

// deliveryStorageKeyMaxLen 投递流水存储主键(复合串)的长度上限,与 ent schema
// ceoboard_webhook_deliveries.key 列 MaxLen(512) 一致(schema 改值须同步)。
const deliveryStorageKeyMaxLen = 512

// deliverySourceMaxLen source 列长度上限,与 ent schema MaxLen(64) 一致。
const deliverySourceMaxLen = 64

// deliveryEventIDMaxLen 幂等键在 ceoboard_events.event_id 列的长度上限,与
// ent schema ceoboard_event.go 的 event_id MaxLen(256) 一致(schema 改值须同步)。
// adapter 落事件时 event_id 直接取幂等键(insertWebhookEvent),故这是键预算的
// 最紧下游约束。
const deliveryEventIDMaxLen = 256

// MaxIdempotencyKeyLen 幂等键长度预算(评审 2026-11-09 R1):取下游各列约束的
// 最小值——复合存储主键 512 − ':' − source 上限 64 = 447;event_id 列 256。
// 故预算 256:不超预算的键对任意合法 source(≤64)可全程落库(投递流水 + 事件)。
// webhook 入口(adapter)对超预算键显式 4xx 拒收——不再落到 ent 校验走"投递流水
// 缺失必须 503"的既定路径让源无限重试(毒消息);幂等键不静默替换/截断(dedup
// 语义不破)。提升上限须同步 bump ent schema 的 event_id/复合键 MaxLen 并随迁移
// 重跑回填(PG 侧待办)。
const MaxIdempotencyKeyLen = min(deliveryStorageKeyMaxLen-1-deliverySourceMaxLen, deliveryEventIDMaxLen) // 256

// ErrIdempotencyKeyTooLong 幂等键超长度预算。adapter HTTP 层据此映射 4xx;
// 其他调用方按普通持久化错误处理(既定 503 语义不变)。
var ErrIdempotencyKeyTooLong = errors.New("幂等键超过长度预算")

// deliveryStorageID 投递流水的存储主键:source + ":" + 幂等键 复合串(评审
// 2026-11-09 MEDIUM-7)。幂等维度 (source,key):不同源同名键互不冲突。构造收口
// 于此,包外一律透明。source/键自身可能含 ':'(如摘要回退键 "sha256:…"),不影响
// 正确性——剥前缀总是按行上的 source 列精确还原。
func deliveryStorageID(source, key string) string {
	return source + ":" + key
}

// deliveryKeyFromStorageID 由存储主键还原裸幂等键(剥掉 source: 前缀)。
// 无前缀的存量裸键行(未经迁移 20261311 回填,或超长键被回填 guard 跳过的行)
// 恒等返回——读取兼容不破。
func deliveryKeyFromStorageID(source, storageID string) string {
	return strings.TrimPrefix(storageID, source+":")
}

// InsertDelivery 幂等记录一条 webhook 投递;返回 true 表示新插入。
// 幂等维度 (source,key):同一源同键去重;不同源同键各自成单(MEDIUM-7)。
// 复合主键超 512 直接报 ErrIdempotencyKeyTooLong(R1;正常路径已被 adapter
// 入口 256 预算拦截,此为非 adapter 调用方的防御兜底,绝不静默存储)。
func (s *Store) InsertDelivery(ctx context.Context, d *Delivery) (bool, error) {
	storageID := deliveryStorageID(d.Source, d.Key)
	if len(storageID) > deliveryStorageKeyMaxLen {
		return false, fmt.Errorf("%w: 复合键 %d 字节(上限 %d)", ErrIdempotencyKeyTooLong, len(storageID), deliveryStorageKeyMaxLen)
	}
	err := s.client.CeoboardWebhookDelivery.Create().
		SetID(storageID).
		SetSource(d.Source).
		SetEvent(d.Event).
		SetEntityID(d.EntityID).
		SetAt(d.At.UTC()).
		SetDigest(d.Digest).
		SetFields(d.Fields).
		SetStatus(d.Status).
		SetError(d.Error).
		Exec(ctx)
	if err == nil {
		return true, nil
	}
	if !ent.IsConstraintError(err) {
		return false, fmt.Errorf("投递落库: %w", err)
	}
	return false, nil
}

// MarkDelivery 更新投递状态(处理失败时标 failed);行定位同样按 (source,key)。
func (s *Store) MarkDelivery(ctx context.Context, source, key, status, errStr string) error {
	storageID := deliveryStorageID(source, key)
	if len(storageID) > deliveryStorageKeyMaxLen {
		return fmt.Errorf("%w: 复合键 %d 字节(上限 %d)", ErrIdempotencyKeyTooLong, len(storageID), deliveryStorageKeyMaxLen)
	}
	return s.client.CeoboardWebhookDelivery.UpdateOneID(storageID).
		SetStatus(status).
		SetError(errStr).
		Exec(ctx)
}

// CountDeliveries 统计窗口内某源(可按事件名前缀过滤)的成功投递数(status='ok')。
func (s *Store) CountDeliveries(ctx context.Context, source, eventPrefix string, since time.Time) (int, error) {
	q := s.client.CeoboardWebhookDelivery.Query().
		Where(
			ceoboardwebhookdelivery.SourceEQ(source),
			ceoboardwebhookdelivery.StatusEQ("ok"),
			ceoboardwebhookdelivery.AtGTE(since.UTC()),
		)
	if eventPrefix != "" {
		q = q.Where(ceoboardwebhookdelivery.EventHasPrefix(eventPrefix))
	}
	return q.Count(ctx)
}

// RecentDeliveries 返回某源最近投递(新→旧)。
func (s *Store) RecentDeliveries(ctx context.Context, source string, limit int) ([]Delivery, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.client.CeoboardWebhookDelivery.Query().
		Where(ceoboardwebhookdelivery.SourceEQ(source)).
		Order(ent.Desc(ceoboardwebhookdelivery.FieldAt)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Delivery, 0, len(rows))
	for _, d := range rows {
		out = append(out, Delivery{
			// 剥 source: 前缀还原裸幂等键(复合存储主键对包外透明);存量未回填
			// 裸键行恒等返回。
			Key: deliveryKeyFromStorageID(d.Source, d.ID), Source: d.Source, Event: d.Event, EntityID: d.EntityID,
			At: d.At.UTC(), Digest: d.Digest, Fields: d.Fields, Status: d.Status, Error: d.Error,
		})
	}
	return out, nil
}

// ---------- 死信 ----------

// AddDeadLetter 写入一条死信;同一来源的未重放业务键重试时幂等(返回已有 id)。
// 原为单条 INSERT...WHERE NOT EXISTS 原子语句;Ent 版双保险(评审 2026-11-09 M2):
// ① dlMu 进程内串行化"查重+插入"(ceoboard 单实例写入者快路径);
// ② 库层部分唯一索引 ceoboard_dead_letters_unreplayed_ref_uidx(迁移 20261310)
//
//	兜底跨实例/双跑并发——查重窗口被另一写者抢插时 Create 撞唯一冲突,重查返回
//	已有 id,与"先查后插"等幂。
func (s *Store) AddDeadLetter(ctx context.Context, kind, source, refKey, payload, reason string) (int64, error) {
	s.dlMu.Lock()
	defer s.dlMu.Unlock()
	// 消毒(H3):payload/refKey 源自消息字节,PG TEXT 拒收非法 UTF-8/NUL;
	// 先消毒再查重,保证查重键与写入键一致。
	refKey = sanitizeText(refKey)
	payload = sanitizeText(payload)
	reason = sanitizeText(reason)
	// findUnreplayed 查同 (kind,source,refKey) 的未重放行(最新优先);查重与
	// 冲突兜底共用同一谓词,防止两路漂移。
	findUnreplayed := func() (*ent.CeoboardDeadLetter, error) {
		return s.client.CeoboardDeadLetter.Query().
			Where(
				ceoboarddeadletter.KindEQ(kind),
				ceoboarddeadletter.SourceEQ(source),
				ceoboarddeadletter.RefKeyEQ(refKey),
				ceoboarddeadletter.ReplayedAtIsNil(),
			).
			Order(ent.Desc(ceoboarddeadletter.FieldID)).
			First(ctx)
	}
	if refKey != "" {
		existing, err := findUnreplayed()
		if err == nil {
			return existing.ID, nil
		}
		if !ent.IsNotFound(err) {
			return 0, err
		}
	}
	d, err := s.client.CeoboardDeadLetter.Create().
		SetKind(kind).
		SetSource(source).
		SetRefKey(refKey).
		SetPayload(payload).
		SetReason(truncate(reason, 1024)).
		SetFailedAt(time.Now().UTC()).
		Save(ctx)
	if err != nil {
		// 唯一冲突 = 查重窗口内被另一实例抢先插入同键(未重放);重查返回其 id。
		if refKey != "" && ent.IsConstraintError(err) {
			if existing, qerr := findUnreplayed(); qerr == nil {
				return existing.ID, nil
			}
		}
		return 0, fmt.Errorf("死信落库: %w", err)
	}
	return d.ID, nil
}

// ListDeadLetters 返回死信(未重放的在前,新→旧)。
func (s *Store) ListDeadLetters(ctx context.Context, limit int) ([]DeadLetter, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.client.CeoboardDeadLetter.Query().
		Order(func(sel *entsql.Selector) {
			// (replayed_at IS NULL) DESC 在 PG/SQLite 均为真值在前
			sel.OrderBy("(replayed_at IS NULL) DESC")
			sel.OrderBy(entsql.Desc(ceoboarddeadletter.FieldFailedAt))
		}).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]DeadLetter, 0, len(rows))
	for _, d := range rows {
		out = append(out, deadLetterToDTO(d))
	}
	return out, nil
}

func deadLetterToDTO(d *ent.CeoboardDeadLetter) DeadLetter {
	out := DeadLetter{
		ID: d.ID, Kind: d.Kind, Source: d.Source, RefKey: d.RefKey,
		Payload: d.Payload, Reason: d.Reason, FailedAt: d.FailedAt.UTC(),
	}
	if d.ReplayedAt != nil {
		t := d.ReplayedAt.UTC()
		out.ReplayedAt = &t
	}
	return out
}

// GetDeadLetter 取单条死信。
func (s *Store) GetDeadLetter(ctx context.Context, id int64) (*DeadLetter, error) {
	d, err := s.client.CeoboardDeadLetter.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	out := deadLetterToDTO(d)
	return &out, nil
}

// MarkReplayed 标记死信已重放。
func (s *Store) MarkReplayed(ctx context.Context, id int64) error {
	return s.client.CeoboardDeadLetter.UpdateOneID(id).
		SetReplayedAt(time.Now().UTC()).
		Exec(ctx)
}

// PendingDeadLetters 返回未重放死信数。
func (s *Store) PendingDeadLetters(ctx context.Context) (int, error) {
	return s.client.CeoboardDeadLetter.Query().
		Where(ceoboarddeadletter.ReplayedAtIsNil()).
		Count(ctx)
}

// ---------- 写操作审计 ----------

// AddAudit 写一条审计;失败仅返回错误(审计不阻断主流程,由调用方记日志)。
// 截断口径:actor 128 / action 64 / target 256 / detail 1024。
func (s *Store) AddAudit(ctx context.Context, actor, action, target string, ok bool, detail string) error {
	err := s.client.CeoboardAudit.Create().
		SetAt(time.Now().UTC()).
		SetActor(truncate(actor, 128)).
		SetAction(truncate(action, 64)).
		SetTarget(truncate(target, 256)).
		SetOk(ok).
		SetDetail(truncate(detail, 1024)).
		Exec(ctx)
	if err != nil {
		s.auditWriteFailed.Add(1)
	}
	return err
}

// ListAudit 按时间倒序取审计(可空 action 过滤);limit 封顶 500 防导出拖库。
func (s *Store) ListAudit(ctx context.Context, limit int, action string) ([]AuditEntry, error) {
	return s.listAudit(ctx, limit, action, nil, nil)
}

// ListAuditWindow 按页面筛选条件返回审计记录,时间窗与 CSV 导出一致。
func (s *Store) ListAuditWindow(ctx context.Context, limit int, action string, from, to time.Time) ([]AuditEntry, error) {
	return s.listAudit(ctx, limit, action, &from, &to)
}

func (s *Store) listAudit(ctx context.Context, limit int, action string, from, to *time.Time) ([]AuditEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	q := s.client.CeoboardAudit.Query()
	if from != nil && to != nil {
		q = q.Where(ceoboardaudit.AtGTE(from.UTC()), ceoboardaudit.AtLT(to.UTC()))
	}
	if action != "" {
		q = q.Where(ceoboardaudit.ActionEQ(action))
	}
	rows, err := q.
		Order(ent.Desc(ceoboardaudit.FieldAt), ent.Desc(ceoboardaudit.FieldID)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AuditEntry, 0, len(rows))
	for _, e := range rows {
		out = append(out, auditToDTO(e))
	}
	return out, nil
}

func auditToDTO(e *ent.CeoboardAudit) AuditEntry {
	return AuditEntry{
		ID: e.ID, At: e.At.UTC(), Actor: e.Actor, Action: e.Action,
		Target: e.Target, OK: e.Ok, Detail: e.Detail,
	}
}

// ---------- latency 物化样本 ----------

// UpsertLatencySample 幂等写入(同 metric+b_uid 重复忽略);返回 true 表示新插入。
func (s *Store) UpsertLatencySample(ctx context.Context, sm *LatencySample) (bool, error) {
	err := s.client.CeoboardLatencySample.Create().
		SetMetricID(sm.MetricID).
		SetBUID(sm.BUID).
		SetEntityID(sm.EntityID).
		SetAUID(sm.AUID).
		SetLatencyMs(sm.LatencyMs).
		SetAt(sm.At.UTC()).
		Exec(ctx)
	if err == nil {
		return true, nil
	}
	if !ent.IsConstraintError(err) {
		return false, err
	}
	return false, nil
}

// LatencyWindow 取窗口内样本的延迟毫秒列(供 p50/p95)。
func (s *Store) LatencyWindow(ctx context.Context, metricID string, since time.Time) ([]int64, error) {
	vals, err := s.client.CeoboardLatencySample.Query().
		Where(ceoboardlatencysample.MetricIDEQ(metricID), ceoboardlatencysample.AtGTE(since.UTC())).
		Select(ceoboardlatencysample.FieldLatencyMs).
		Ints(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]int64, len(vals))
	for i, v := range vals {
		out[i] = int64(v)
	}
	return out, nil
}

// ListPairEvents 取配对候选(上限 5000 防大窗拖垮;物化周期短,正常远小于此)。
// occurred_at NULL(信封未带时间)退 received_at。
func (s *Store) ListPairEvents(ctx context.Context, platform, subjectKey string, since time.Time) ([]PairEvent, error) {
	var rows []struct {
		ID         string     `json:"event_uid"` // 存储列名(StorageKey),非 ent 字段名 id
		EntityID   string     `json:"entity_id"`
		OccurredAt *time.Time `json:"occurred_at"`
		ReceivedAt time.Time  `json:"received_at"`
	}
	err := s.client.CeoboardEvent.Query().
		Where(
			ceoboardevent.PlatformEQ(platform),
			ceoboardevent.SubjectKeyEQ(subjectKey),
			ceoboardevent.EntityIDNEQ(""),
			ceoboardevent.ReceivedAtGTE(since.UTC()),
		).
		Order(ent.Asc(ceoboardevent.FieldReceivedAt)).
		Limit(5000).
		Select(ceoboardevent.FieldID, ceoboardevent.FieldEntityID, ceoboardevent.FieldOccurredAt, ceoboardevent.FieldReceivedAt).
		Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	out := make([]PairEvent, 0, len(rows))
	for _, r := range rows {
		pe := PairEvent{UID: r.ID, EntityID: r.EntityID, At: r.ReceivedAt.UTC()}
		if r.OccurredAt != nil {
			pe.At = r.OccurredAt.UTC()
		}
		out = append(out, pe)
	}
	return out, nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
