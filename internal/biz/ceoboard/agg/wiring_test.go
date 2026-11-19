package agg

import (
	"aranea-agents/internal/biz/ceoboard/adapter"
	"aranea-agents/internal/biz/ceoboard/store"
)

// 编译期断言:P2-5 Ent 仓储与 P2-6 适配器实现 agg 的消费者侧窄接口
// (P2-7 装配前的漂移防线)。
var (
	_ EventReadStore = (*store.Store)(nil)
	_ LatencyStore   = (*store.Store)(nil)

	_ PollQuerier    = (*adapter.Poller)(nil)
	_ WebhookCounter = (*adapter.WebhookReceiver)(nil)
	_ NATSQuerier    = (*adapter.NATSManager)(nil)
)
