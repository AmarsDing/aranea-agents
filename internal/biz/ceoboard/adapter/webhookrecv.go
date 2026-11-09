// webhook 接收端(自 ceo-dashboard/internal/adapter 移植,语义等价)。
// 接收、鉴权(共享密钥 X-CEO-Key)、幂等去重(投递幂等键主键,重启不丢)、失败死信与重放。
// 幂等键优先级链(风险点⑨):强制键(重放) > 显式头 > body.idempotency_key > body.event_id
// > 实体 id > 内容摘要。投递流水缺失必须 503 让源重试,不能 202。
//
// 移植差异:存储改走 Ent 仓储并全量穿 context.Context;实体提取在 opportunity 之外
// 增补顶层 entity_id 字段(aranea 进程内 outbox 直投路径,见 outbox.go;原 HTTP 投递
// 不带该字段,行为不变)。WebhookEvent 视图类型在 types.go(agg 依赖面)。
package adapter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"aranea-agents/internal/biz/ceoboard/store"
)

// WebhookReceiver 是 webhook-recv 适配器:
// 接收、鉴权(共享密钥,签名头预留)、幂等去重(主键,重启不丢)、失败死信与重放。
// aranea 侧签名头待其补齐 → 接口预留 X-Aranea-Signature 透传记录,不强制。
type WebhookReceiver struct {
	st        *store.Store
	sharedKey string // X-CEO-Key;空 = 不校验(仅内网 ACL)
}

// NewWebhookReceiver 构造接收器。st 为 nil 时拒绝构造(不支持内存态)。
func NewWebhookReceiver(st *store.Store, sharedKey string) *WebhookReceiver {
	return &WebhookReceiver{st: st, sharedKey: sharedKey}
}

// Handler 返回 POST /v1/ceoboard/webhooks/{source} 的处理函数。
func (r *WebhookReceiver) Handler(source string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		if r.sharedKey != "" && req.Header.Get("X-CEO-Key") != r.sharedKey {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		const maxBodyBytes = 1 << 20
		body, err := io.ReadAll(io.LimitReader(req.Body, maxBodyBytes+1))
		if err != nil {
			// 接收失败:进死信(reason=读体失败),payload 不可得
			_, _ = r.st.AddDeadLetter(req.Context(), "webhook-receive", source, "", "", "读取请求体失败: "+err.Error())
			http.Error(w, `{"error":"read body"}`, http.StatusBadRequest)
			return
		}
		if len(body) > maxBodyBytes {
			http.Error(w, `{"error":"request body too large"}`, http.StatusRequestEntityTooLarge)
			return
		}
		ev := r.Ingest(req.Context(), source, req.Header, body)
		w.Header().Set("Content-Type", "application/json")
		if ev.PersistErr != nil {
			// R1:幂等键超预算是客户端错误(源必须换键,重试无意义)→ 4xx;
			// 其余持久化失败维持既定 503(让源重试,绝不 202 吞消息)。
			if errors.Is(ev.PersistErr, store.ErrIdempotencyKeyTooLong) {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"accepted": false, "error": ev.ErrMsg})
				return
			}
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]any{"accepted": false, "error": "事件持久化失败,请重试"})
			return
		}
		w.WriteHeader(http.StatusAccepted) // 回 202:已受理;重复投递也回 202(幂等,让源停重试)
		_ = json.NewEncoder(w).Encode(map[string]any{"accepted": true, "deduped": ev.Deduped, "key": ev.Key})
	}
}

// Ingest 归一化、持久化并入站一条 webhook;返回事件(含是否被去重)。
// 处理失败(JSON 不可解析):投递标 failed + 进死信,事件仍以摘要键落库(不丢)。
// 本方法同时为进程内 outbox 直投入口(不经过 HTTP 层)。
func (r *WebhookReceiver) Ingest(ctx context.Context, source string, header http.Header, body []byte) WebhookEvent {
	return r.ingest(ctx, source, header, body, "")
}

// ingest 核心实现;forceKey 非空时强制幂等键(死信重放保真:沿用原投递键)。
func (r *WebhookReceiver) ingest(ctx context.Context, source string, header http.Header, body []byte, forceKey string) WebhookEvent {
	sum := sha256.Sum256(body)
	ev := WebhookEvent{
		Source: source,
		At:     time.Now(),
		Digest: hex.EncodeToString(sum[:8]),
		Fields: map[string]any{},
		Status: "ok",
	}

	var doc map[string]any
	parseErr := json.Unmarshal(body, &doc)
	if parseErr == nil && doc != nil {
		if s, _ := doc["event"].(string); s != "" {
			ev.Event = s
		}
		// aranea 媒体电商 webhook:payload 含 opportunity 对象(config.mvp.media-commerce.example.yaml)
		if opp, ok := doc["opportunity"].(map[string]any); ok {
			ev.EntityID = strOf(opp["id"])
			for _, k := range []string{"id", "title", "status", "audience", "score"} {
				if v, ok := opp[k]; ok {
					ev.Fields["opportunity."+k] = v
				}
			}
		}
		if ev.EntityID == "" {
			// 进程内 outbox 直投扩展:顶层 entity_id 作实体兜底(原 HTTP 投递不带,行为不变)
			ev.EntityID = strOf(doc["entity_id"])
		}
	}

	// 幂等键优先级:强制键(重放) > 显式头 > body.idempotency_key > body.event_id > 实体 id > 内容摘要
	ev.Key = firstNonEmpty(
		forceKey,
		header.Get("Idempotency-Key"), header.Get("X-Idempotency-Key"),
		strOf(doc["idempotency_key"]), strOf(doc["event_id"]), ev.EntityID, "sha256:"+ev.Digest,
	)

	// 评审 2026-11-09 R1:幂等键长度预算显式校验——超预算键在入口即拒(HTTP 层
	// 映射 4xx),不落库任何足迹(投递/事件/死信都不写:复合存储键塞不下,写了
	// 也只在 ent 校验处失败,源会把 503 当瞬态故障无限重试=毒消息)。幂等键不
	// 静默替换/截断:换键即换 dedup 身份,重复投递会漏判。
	if len(ev.Key) > store.MaxIdempotencyKeyLen {
		ev.Status = "failed"
		ev.ErrMsg = fmt.Sprintf("幂等键长度 %d 超预算 %d", len(ev.Key), store.MaxIdempotencyKeyLen)
		ev.PersistErr = fmt.Errorf("%w: 长度 %d 超预算 %d", store.ErrIdempotencyKeyTooLong, len(ev.Key), store.MaxIdempotencyKeyLen)
		return ev
	}

	fieldsJSON, _ := json.Marshal(ev.Fields)
	d := &store.Delivery{
		Key: ev.Key, Source: source, Event: ev.Event, EntityID: ev.EntityID,
		At: ev.At, Digest: ev.Digest, Fields: string(fieldsJSON), Status: "ok",
	}
	if parseErr != nil {
		d.Status = "failed"
		d.Error = "JSON 解析失败: " + parseErr.Error()
	}
	inserted, err := r.st.InsertDelivery(ctx, d)
	if err != nil {
		// event 可先进 spool,但投递流水缺失时必须让源重试补齐,不能 202。
		_, _ = r.st.AddDeadLetter(ctx, "webhook-receive", source, ev.Key, string(body), "投递落库失败: "+err.Error())
		ev.Status = "failed"
		ev.ErrMsg = err.Error()
		ev.PersistErr = r.insertWebhookEvent(ctx, source, &ev, body)
		if ev.PersistErr == nil {
			ev.PersistErr = fmt.Errorf("投递落库失败: %w", err)
		}
		return ev
	}
	if parseErr != nil {
		ev.Status = "failed"
		ev.ErrMsg = d.Error
		// 首次和去重命中分支都必须确保死信存在;AddDeadLetter 按业务键幂等。
		if forceKey == "" {
			if _, deadLetterErr := r.st.AddDeadLetter(ctx, "webhook-process", source, ev.Key, string(body), d.Error); deadLetterErr != nil {
				ev.PersistErr = r.insertWebhookEvent(ctx, source, &ev, body)
				if ev.PersistErr == nil {
					ev.PersistErr = fmt.Errorf("死信落库失败: %w", deadLetterErr)
				}
				return ev
			}
		}
	}
	if !inserted {
		ev.Deduped = true
		ev.PersistErr = r.insertWebhookEvent(ctx, source, &ev, body)
		return ev // 重复投递:不计数、不改状态(去重,重启后仍成立)
	}
	if parseErr != nil {
		ev.PersistErr = r.insertWebhookEvent(ctx, source, &ev, body)
		return ev
	}

	// 统一事件流:webhook 也落 events 表(event_uid = 平台:幂等键),供窗口计数与双轨对账
	ev.PersistErr = r.insertWebhookEvent(ctx, source, &ev, body)
	return ev
}

// insertWebhookEvent 把 webhook 投递同步为 CEOEvent(与 NATS 事件同表,event_uid 同源幂等)。
func (r *WebhookReceiver) insertWebhookEvent(ctx context.Context, source string, ev *WebhookEvent, body []byte) error {
	occ := ev.At
	if s, ok := ev.Fields["opportunity.occurred_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			occ = t
		}
	}
	_, err := r.st.IngestEvent(ctx, &store.Event{
		UID: source + ":" + ev.Key, Platform: source, Subject: ev.Event, SubjectKey: SubjectKey(ev.Event),
		EventID: ev.Key, EntityID: ev.EntityID, OccurredAt: occ, ReceivedAt: ev.At, Payload: string(body),
	}, nil)
	return err
}

// CountSince 统计某源某事件在 since 之后的成功投递数(窗口指标口径;failed 不计)。
// eventPrefix 为空串时统计该源全部;否则按前缀匹配(如 "opportunity.")。
func (r *WebhookReceiver) CountSince(ctx context.Context, source, eventPrefix string, since time.Time) int {
	n, err := r.st.CountDeliveries(ctx, source, eventPrefix, since)
	if err != nil {
		return 0
	}
	return n
}

// Events 返回某源最近的投递(新→旧,limit<=0 默认 20)。
func (r *WebhookReceiver) Events(ctx context.Context, source string, limit int) []WebhookEvent {
	ds, err := r.st.RecentDeliveries(ctx, source, limit)
	if err != nil {
		return []WebhookEvent{}
	}
	out := make([]WebhookEvent, 0, len(ds))
	for _, d := range ds {
		fields := map[string]any{}
		_ = json.Unmarshal([]byte(d.Fields), &fields)
		out = append(out, WebhookEvent{
			Source: d.Source, Event: d.Event, Key: d.Key, EntityID: d.EntityID,
			At: d.At, Digest: d.Digest, Fields: fields, Status: d.Status, ErrMsg: d.Error,
		})
	}
	return out
}

// ReplayDeadLetter 重放一条 webhook 死信:重新走 Ingest;返回重放结果。
// 幂等:原文幂等键仍在 → 去重命中,不会重复计数。
func (r *WebhookReceiver) ReplayDeadLetter(ctx context.Context, id int64) (*WebhookEvent, error) {
	dl, err := r.st.GetDeadLetter(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("死信不存在: %w", err)
	}
	if dl.ReplayedAt != nil {
		return nil, fmt.Errorf("死信 #%d 已重放过(%s)", id, dl.ReplayedAt.Format(time.RFC3339))
	}
	if !strings.HasPrefix(dl.Kind, "webhook") {
		return nil, fmt.Errorf("死信 #%d 类型 %s 不支持 webhook 重放", id, dl.Kind)
	}
	source := dl.Source
	if source == "" {
		source = "aranea" // 历史死信缺来源时兜底;新死信均带 source
	}
	// 重放保真:沿用原投递幂等键(ref_key),确保与原投递去重命中、不重复建行
	ev := r.ingest(ctx, source, http.Header{}, []byte(dl.Payload), dl.RefKey)
	if ev.PersistErr != nil {
		return nil, fmt.Errorf("重放持久化失败: %w", ev.PersistErr)
	}
	if err := r.st.MarkReplayed(ctx, id); err != nil {
		return nil, err
	}
	return &ev, nil
}

func strOf(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
