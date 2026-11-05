package provider

import (
	"context"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// HA 切换观测（fork model/failover、model/hedge 的 WithSwitchCallback 的
// 应用层替代，2026-11-03 trpc 出库）。
//
// fork 在框架内部于「实际切换到下一候选」时点回调；上游 v1.11.2 无此能力。
// 本文件以 model.Model 装饰器在候选边界观测，语义等价说明：
//
//   - failover（顺序尝试）：候选 i 在未产出任何有效内容前失败（GenerateContent
//     返回错误或流出错误响应），框架随即准备候选 i+1——此时触发
//     onSwitch(i → i+1)。已产出有效内容后的中段错误不触发（框架不会再切换，
//     与 fork 行为一致）；末位候选失败无下一候选，不触发（同 fork）。
//   - hedge（并行竞速）：备份候选 i>0 的流完整收官（观察到 Done 响应）意味着
//     它赢得了竞速——上游在选出 winner 后立即 cancelLosers，落败候选的流会被
//     提前取消、走不到 Done。据此触发 onSwitch(0 → i, "hedge_winner")。
//     存在理论竞态（落败者 Done 帧先于取消到达被误记），概率极低且仅为观测
//     事件，不影响正确性——该近似语义由
//     TestHAObserve_HedgeLoserDonePinsApproximation 钉住。
//
// 事件消费：haSwitchCallback（trpc_llm.go）——进程 Warn 日志 + monitor bus
// 流程日志。**仅作观测，不作为任何正确性依据**：内容归属由上游 failover/
// hedge 框架自身的候选选择逻辑决定，本文件不产生也不参与任何控制流决策。

// HASwitchCallback 与 fork SwitchCallback 同签名：from/to 为候选下标与名称，
// reason 为机器可读短因（failover=错误消息，hedge="hedge_winner"）。
type HASwitchCallback func(fromIndex, toIndex int, fromName, toName, reason string)

// haObserveModel 包装单个 HA 候选，在不改变转发语义的前提下观测失败/夺冠。
type haObserveModel struct {
	inner trpcmodel.Model
	// onErr：failover 模式——候选在未产出有效内容前失败时触发（每次
	// GenerateContent 调用至多一次）。
	onErr func(reason string)
	// onWin：hedge 模式——候选流完整收官（Done）时触发（每次
	// GenerateContent 调用至多一次）。
	onWin func()
}

func (m *haObserveModel) Info() trpcmodel.Info { return m.inner.Info() }

func (m *haObserveModel) GenerateContent(ctx context.Context, req *trpcmodel.Request) (<-chan *trpcmodel.Response, error) {
	ch, err := m.inner.GenerateContent(ctx, req)
	if err != nil {
		if m.onErr != nil {
			m.onErr(err.Error())
		}
		return nil, err
	}
	if m.onErr == nil && m.onWin == nil {
		return ch, nil
	}
	out := make(chan *trpcmodel.Response)
	go func() {
		defer close(out)
		sawContent := false
		fired := false
		won := false
		for resp := range ch {
			if resp != nil {
				switch {
				case resp.Error != nil:
					if m.onErr != nil && !sawContent && !fired {
						fired = true
						m.onErr(resp.Error.Message)
					}
				case resp.IsValidContent():
					sawContent = true
				}
				if resp.Done && sawContent {
					won = true
				}
			}
			if !forwardResponse(ctx, out, resp) {
				return
			}
		}
		if m.onWin != nil && won {
			m.onWin()
		}
	}()
	return out, nil
}

// wrapFailoverCandidatesForObserve 为 failover 候选列表装载切换观测：
// 候选 i 失败且有下一候选时回调 cb(i, i+1, ...)。cb 为 nil 时原样返回。
func wrapFailoverCandidatesForObserve(candidates []trpcmodel.Model, cb HASwitchCallback) []trpcmodel.Model {
	if cb == nil {
		return candidates
	}
	wrapped := make([]trpcmodel.Model, len(candidates))
	for i, c := range candidates {
		if i+1 >= len(candidates) {
			wrapped[i] = c
			continue
		}
		idx := i
		wrapped[i] = &haObserveModel{
			inner: c,
			onErr: func(reason string) {
				cb(idx, idx+1,
					candidates[idx].Info().Name, candidates[idx+1].Info().Name,
					reason)
			},
		}
	}
	return wrapped
}

// wrapHedgeCandidatesForObserve 为 hedge 候选列表装载切换观测：备份候选
// i>0 赢得竞速（流完整收官）时回调 cb(0, i, ..., "hedge_winner")。主候选
// 夺冠不产生事件（与 fork 一致）。cb 为 nil 时原样返回。
func wrapHedgeCandidatesForObserve(candidates []trpcmodel.Model, cb HASwitchCallback) []trpcmodel.Model {
	if cb == nil {
		return candidates
	}
	wrapped := make([]trpcmodel.Model, len(candidates))
	for i, c := range candidates {
		if i == 0 {
			wrapped[i] = c
			continue
		}
		idx := i
		wrapped[i] = &haObserveModel{
			inner: c,
			onWin: func() {
				cb(0, idx,
					candidates[0].Info().Name, candidates[idx].Info().Name,
					"hedge_winner")
			},
		}
	}
	return wrapped
}
