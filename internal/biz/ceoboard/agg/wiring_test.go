package agg

import (
	"aranea-agents/internal/biz/ceoboard/store"
)

// 编译期断言:P2-5 Ent 仓储实现 agg 的消费者侧窄接口(P2-7 装配前的漂移防线)。
var (
	_ EventReadStore = (*store.Store)(nil)
	_ LatencyStore   = (*store.Store)(nil)
)
