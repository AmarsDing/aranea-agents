// aranea 进程内事件 outbox 直投(整合裁定:aranea→ceoboard 不再绕 HTTP webhook 回路,
// 同进程直接调用接收管线)。与 HTTP 投递完全同语义:幂等键去重、失败进死信、
// 统一事件流落库;只是省去自环网络跳数与共享密钥。
// 直投事件以顶层 entity_id 携带实体标识(接收端归一化的增补字段,见 webhookrecv.go)。
package adapter

import (
	"context"
	"encoding/json"
	"fmt"
)

// Outbox 是某来源(如 "aranea")的进程内直投口;由 WebhookReceiver 派生,共享其存储与去重状态。
type Outbox struct {
	receiver *WebhookReceiver
	source   string
}

// Outbox 返回某来源的进程内直投口。
func (r *WebhookReceiver) Outbox(source string) *Outbox {
	return &Outbox{receiver: r, source: source}
}

// Emit 直投一条事件。idemKey 为幂等键(域名事件 id;空则退实体 id → 内容摘要);
// entityID 进实体列(供 latency 配对与下钻);payload 为调用方业务字段(展开进信封,
// 保留键 event/idempotency_key/entity_id 由参数强制覆盖,防误填)。
// 返回事件视图;持久化失败返回错误(语义同 HTTP 503:调用方应重试,重复投递被去重)。
func (o *Outbox) Emit(ctx context.Context, event, idemKey, entityID string, payload map[string]any) (WebhookEvent, error) {
	body := map[string]any{}
	for k, v := range payload {
		body[k] = v
	}
	body["event"] = event
	if idemKey != "" {
		body["idempotency_key"] = idemKey
	}
	if entityID != "" {
		body["entity_id"] = entityID
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return WebhookEvent{}, fmt.Errorf("outbox 信封编码失败: %w", err)
	}
	ev := o.receiver.Ingest(ctx, o.source, nil, raw)
	if ev.PersistErr != nil {
		return ev, ev.PersistErr
	}
	return ev, nil
}
