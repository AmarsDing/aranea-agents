// run_heartbeat.go — P2-1（2026-09-03 持久化心跳）：团队运行流式心跳。
//
// 背景：biz idle 探测（spirit_orchestration.probeTeamActivity）的活跃信号是
// steps_v2.started_at 聚合——成员单 step 内超长流式生成（>idle 窗口无新
// step 启动）会被误判 idle 击杀。本文件把流式事件转化为持久化心跳
// （team_runs_v2.heartbeat_at），探测端取 max(steps, heartbeat) 后长生成
// 不再误杀。语义对齐 LangGraph progress signals / Temporal activity
// heartbeat：流式活动本身就是活性证据。
package team

import (
	"context"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// teamRunHeartbeatInterval 是心跳节流间隔。Temporal 经验：心跳超时取
// 2~3×心跳间隔；biz idle 窗口（默认 600s）>> 3×10s，误杀余量充足。
// 10s 间隔对单行单列 UPDATE 的写放大约束足够保守（一次 run 每小时 360 行
// UPDATE，无索引膨胀）。
const teamRunHeartbeatInterval = 10 * time.Second

// teamRunHeartbeatPinger 节流心跳写入器。Ping 可能被图运行时的多个成员
// 流并发调用（并行成员共享同一 streamOpts），内部用 mutex 保护。
type teamRunHeartbeatPinger struct {
	writer biz.TeamRunHeartbeatWriter
	runID  string
	lg     loggateway.Logger

	mu   sync.Mutex
	last time.Time
}

func newTeamRunHeartbeatPinger(writer biz.TeamRunHeartbeatWriter, runID string, lg loggateway.Logger) *teamRunHeartbeatPinger {
	return &teamRunHeartbeatPinger{writer: writer, runID: runID, lg: lg}
}

// Ping 在流式事件到达时调用；距上次成功心跳不足间隔则直接返回。
// ctx 取消后静默跳过（run 收尾期事件仍在排干，写库无意义）。
func (p *teamRunHeartbeatPinger) Ping(ctx context.Context) {
	if p == nil || p.writer == nil || p.runID == "" {
		return
	}
	if ctx.Err() != nil {
		return
	}
	p.mu.Lock()
	if time.Since(p.last) < teamRunHeartbeatInterval {
		p.mu.Unlock()
		return
	}
	p.last = time.Now()
	p.mu.Unlock()
	// 写库在 consume goroutine 内同步执行（间隔 10s，单列 UPDATE 毫秒级）；
	// 失败仅记日志——心跳丢失的代价是探测回退 steps.started_at 旧语义，
	// 不得反向影响流式消费。
	if err := p.writer.TouchTeamRunHeartbeat(ctx, p.runID, time.Now()); err != nil {
		p.lg.Warn("团队运行心跳写入失败",
			loggateway.StepID("team.run.heartbeat_err"),
			loggateway.Str("run_id", p.runID),
			loggateway.Err(err),
		)
	}
}
