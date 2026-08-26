package configgraph

import (
	"context"

	"aranea-agents/pkg/loggateway"
)

// LogIndexerStarted 是 P0 验收硬判据的启动日志消息（design §4.3 / R7：
// Indexer 漏挂 wire 会静默不实例化，必须以该日志为证）。
const LogIndexerStarted = "configgraph indexer started"

// Indexer 是配置资产图谱的后台组件（design §4.2）。P0 仅承载重建能力：
// 启动时播种内存当前代并输出判据日志；事件订阅、500ms 防抖、seed 局部
// 重算与 hourly 兜底在 P2 落地（dev plan 0.9 明确“仅重建能力”）。
type Indexer struct {
	rebuilder *Rebuilder
	lg        loggateway.Logger
}

// NewIndexer 构造后台组件。rebuilder 为空（源/图 repo 缺失）时返回 nil，
// 装配侧据此跳过启动。
func NewIndexer(rebuilder *Rebuilder, lg loggateway.Logger) *Indexer {
	if rebuilder == nil {
		return nil
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &Indexer{rebuilder: rebuilder, lg: lg.With(loggateway.Domain("config_graph"))}
}

// Rebuilder 暴露重建器（HTTP service 经此拿到异步触发/状态查询入口）。
func (i *Indexer) Rebuilder() *Rebuilder {
	if i == nil {
		return nil
	}
	return i.rebuilder
}

// Start 播种当前代并输出判据日志，随后驻留至 ctx 取消（workers.go 的
// BackgroundStarter 形态；P2 在此挂事件订阅与兜底 ticker）。播种失败不
// 致命——首次 Rebuild 会惰性补播。
func (i *Indexer) Start(ctx context.Context) {
	if i == nil {
		return
	}
	if err := i.rebuilder.Init(ctx); err != nil {
		i.lg.Warn("configgraph generation seed failed", loggateway.Err(err))
	}
	i.lg.Info(LogIndexerStarted, loggateway.StepID("configgraph.indexer"))
	<-ctx.Done()
}
