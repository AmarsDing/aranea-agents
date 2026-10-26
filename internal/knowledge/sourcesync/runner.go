// runner.go —— G-38 SourceSync Runner：设计 ⑥ 八步序唯一编排真理。
//
// 步骤映射（设计 ⑥ ↔ 代码段）：
//
//	⑥-1 保留集载入        → walk 前 CountActiveItems（walk_empty_abort 输入）
//	⑥-2 walk 标记         → emit 闭包内 UpsertItemSeen（出席即 active/miss=0）
//	⑥-3 候选圈定          → walk 完满 + 无闸命中后 MarkAbsentItemsMissing
//	⑥-4 auth 整轮闸       → AuthError 任一点（walk/Fetch）→ auth_paused 收轮，跳删除
//	     walk_empty_abort → seen==0 ∧ prevActive>0 → failed 收轮，跳删除
//	⑥-5 ConfirmDeleted    → 三态逐条（Unknown/StillExists→miss_unknown 保留）
//	⑥-6 预算截断          → max_deletes 只闸 Confirmed 消费；工作预算哨兵
//	                        ErrBudgetExhausted 中止 walk、游标不动、partial 收轮
//	⑥-7 级联唯一正道      → own_lifecycle=true→applier.Delete；false→仅 MarkItemDeleted
//	⑥-8 收尾              → InsertRunWithRetention → FinalizeRunWithClaim（顺序钉：
//	                        先审计行后翻连接器——崩溃窗口宁留僵尸（可回收）不丢审计）
//
// W2 as-built 登记（design ⑭ 条）：
//   - 工作预算（max_items/max_fetch_bytes）只闸提交工作量不闸出席事实；命中哨兵
//     即中止 walk + 游标不动 + partial + 跳删除（部分 walk 非缺席权威，与
//     walk_empty_abort 同源安全理由）；预算耗尽轮的未处理项靠保留旧指纹/
//     空 document_id 下轮自然重扫续传（needsWork 判定含 DocumentID=="" 与
//     ContentHash==""，见 emit）。
//   - auth 闸即时生效：不继续 walk 烧配额（设计「整轮闸」保守读法），游标不动、
//     不入退避序列、next_run_at 保持原值等凭据 PUT 唤醒。
//   - fetch_failed 桶合口：Fetch 失败与 applier.Upsert 失败同桶（runs 表列冻结
//     无 apply 专列）；逐条病态 SetItemError（顺序钉：先 UpsertItemSeen 清旧
//     病态再记当轮——出席清空语义③-2 不可倒序）。
//   - 增量出席语义取代 vault 全有/全无：walk error 时跳过圈定与删除，已出席
//     标记保留（出席是事实），游标防骗由 walker 契约（emit 出错游标不动）保证。
//   - partial 判定唯一由 FetchFailed>0 触发；miss_unknown>0 是删除面常态
//     （Notion 三态保守性），不降级轮次整体评级。
//   - 仓口基础设施失败统一归 error_code=walk_failed（walk/圈定前段）或
//     confirm_failed（确认/级联段），真实文本进 error_message（≤512 截断）。
//   - fencing 取代心跳宽容（评审 W2 修，推翻宽容读法）：心跳失地置 lost
//     原子旗 → emit 顶闸 / ⑥-3 圈定前闸 / 删除循环随检三处读旗止步；
//     errClaimLost 哨兵经 walker 契约（出错游标不动）归一 superseded 收轮
//     （failed/walk_failed、游标不动、零圈定零删除、审计行照落、
//     FinalizeRunWithClaim 必 ClaimLost 兜底）。
//   - deleted 复活重绑（评审 W2 修）：needsWork 判定含 Status==deleted——
//     否则复活项保留旧 document_id 成悬挂绑定；重取重绑后 SetItemDocument
//     覆盖指针。
//   - 删除候选窗（评审 W2 修）：ListItemsByStatus limit=显式
//     max(500, 3×max_deletes_per_run)——PG repo 对 limit<=0 静默钳 500，
//     口径在编排层唯一显式化。
package sourcesync

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	bizsync "aranea-agents/internal/biz/knowledge/sourcesync"
	"aranea-agents/pkg/loggateway"
)

const (
	runnerDomain      = "knowledge/sourcesync"
	lockKeyPrefix     = "knowledge-sourcesync:"
	runIDPrefix       = "ss:run:"
	runTokenPrefix    = "ss:tok:"
	minHeartbeatEvery = 5 * time.Second
	maxHeartbeatEvery = 3 * time.Minute
)

// Deps Runner 依赖面（全部接口注入——单测无 PG 替身即真）。
type Deps struct {
	Log        loggateway.Logger     // nil→NewNoop；构造注入铁律
	Connectors bizsync.ConnectorRepo // 必
	Items      bizsync.ItemRepo      // 必
	Runs       bizsync.RunRepo       // 必
	Locker     JobLocker             // nil→跳过跨副本排他（单进程测试面）
	Walkers    map[bizsync.Kind]Walker
	Appliers   map[bizsync.Kind]Applier
	Now        func() time.Time // nil→time.Now（测试可注入恒钟）
	LeaseTTL   time.Duration    // ≤0→bizsync.DefaultRunLeaseTTL
	// HeartbeatEvery 长轮租约续期间隔；≤0→LeaseTTL/3 钳 [5s, 3min]。
	HeartbeatEvery time.Duration
	Frames         FrameSink // nil→noop（W8 接 WS）
}

// Runner ⑥ 八步序编排器（无内部状态——每轮全量从仓口读真相）。
type Runner struct {
	d     Deps
	log   loggateway.Logger
	now   func() time.Time
	lease time.Duration
	hb    time.Duration
}

// NewRunner 依赖规范化（零值安全）。
func NewRunner(d Deps) *Runner {
	lg := d.Log
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	now := d.Now
	if now == nil {
		now = time.Now
	}
	lease := d.LeaseTTL
	if lease <= 0 {
		lease = bizsync.DefaultRunLeaseTTL
	}
	hb := d.HeartbeatEvery
	if hb <= 0 {
		hb = lease / 3
		if hb < minHeartbeatEvery {
			hb = minHeartbeatEvery
		}
		if hb > maxHeartbeatEvery {
			hb = maxHeartbeatEvery
		}
	}
	if d.Frames == nil {
		d.Frames = noopFrameSink{}
	}
	return &Runner{d: d, log: lg.With(loggateway.Domain(runnerDomain)), now: now, lease: lease, hb: hb}
}

// noopFrameSink W8 前的静默帧缝。
type noopFrameSink struct{}

func (noopFrameSink) OnRunStarted(context.Context, RunEvent)  {}
func (noopFrameSink) OnRunFinished(context.Context, RunEvent) {}

// ReclaimZombies 崩溃回收扫描（W4 cron 每 tick 前置调用；观测+抢救面，
// TryClaimRun 内嵌窃取是主路径——本方法返回 0 不代表无回收发生）。
func (r *Runner) ReclaimZombies(ctx context.Context) (int, error) {
	n, err := r.d.Connectors.ReclaimZombieRuns(ctx, r.now(), r.lease)
	if err != nil {
		return 0, err
	}
	if n > 0 {
		r.log.Warn("sourcesync zombie runs reclaimed", loggateway.Int("count", n))
	}
	return n, nil
}

// SyncOnce 单连接器一轮完整 ⑥ 序列（W4 cron due 拾取 / W5 手动触发共用入口）。
// 返回 (outcome, nil)：运维性失败装进 outcome 落 run 审计行；非 nil error 只限
// 仓口基础设施故障与锁面故障（fail-closed）——调用侧按错误级别告警。
func (r *Runner) SyncOnce(ctx context.Context, connectorID string) (RunOutcome, error) {
	out := RunOutcome{ConnectorID: connectorID}
	lg := r.log.With(loggateway.Str("connector_id", connectorID))

	// ── 闸 1：活性行 + enabled ──
	conn, found, err := r.d.Connectors.GetConnector(ctx, connectorID)
	if err != nil {
		return out, err
	}
	if !found {
		out.Skipped, out.SkipReason = true, "not_found"
		return out, nil
	}
	if !conn.Enabled {
		out.Skipped, out.SkipReason = true, "disabled"
		return out, nil
	}

	// ── 闸 2：跨副本排他（⑤ fail-closed：锁面故障宁停不跑） ──
	if r.d.Locker != nil {
		release, ok, lerr := r.d.Locker.TryAcquire(ctx, lockKeyPrefix+conn.ID)
		if lerr != nil {
			lg.Error("sourcesync locker backend failed (fail-closed)", loggateway.Err(lerr))
			return out, lerr
		}
		if !ok {
			out.Skipped, out.SkipReason = true, "locked"
			return out, nil
		}
		defer release()
	}

	// ── 闸 3：轮次认领 CAS（唯一接管口；含窘迫尸窃取） ──
	out.RunToken = newRunToken()
	now := r.now()
	claimed, err := r.d.Connectors.TryClaimRun(ctx, conn.ID, out.RunToken, r.lease, now)
	if err != nil {
		return out, err
	}
	if !claimed {
		out.Skipped, out.SkipReason = true, "claim_lost"
		return out, nil
	}
	out.RunID = newRunID(conn.ID, now)
	lg = lg.With(loggateway.RunID(out.RunID))

	// 轮内状态先于心跳建立——失地旗（fencing）由心跳写、执行环三闸读
	//（评审 W2 修：失地不再「仅记日志」，写面全线止步于哨兵）。
	st := &runState{workLeft: conn.MaxItemsPerRun, cursorNext: conn.Cursor}

	// 心跳保活（长轮防窃取/误回收）；失地主径=置 lost 旗+実行环即刻止步，
	// 收轮 CAS 兜底（FinalizeRunWithClaim 对失地行 found=false→ClaimLost）。
	stopHb := r.startHeartbeat(ctx, conn.ID, out.RunToken, lg, st)
	defer close(stopHb)

	r.d.Frames.OnRunStarted(ctx, RunEvent{
		ConnectorID: conn.ID, Kind: conn.Kind, RunID: out.RunID, RunToken: out.RunToken,
	})

	out = r.execute(ctx, lg, conn, out, st)
	return out, nil
}

// errClaimLost 失地哨兵（emit 顶闸/前置闸触发即中止 walk——walker 契约原样
// 返回，runner 归一为 superseded 收轮：run 审计行照落、cursor 不动、零圈定
// 零删除；与新属主并发写面在栅栏前止步）。
var errClaimLost = errors.New("sourcesync: run superseded (claim lost)")

// runState 轮内可变状态（walker/emit 闭包共享；walker 串行 emit 契约保无竞态；
// lost 由心跳协程跨 goroutine 写——atomic 护栏）。
type runState struct {
	stats      RunStats
	workLeft   int  // max_items_per_run 余额（created+updated 工作预算）
	budgetHit  bool // 工作预算哨兵命中
	authCode   bizsync.ErrorCode
	authHit    bool
	walkDone   bool // walker 无错误返回（含预算哨兵中止——游标仍不动）
	cursorNext CursorState
	lost       atomic.Bool // 心跳失地→true；emit/del/⑥-3 三闸读
}

// execute 认领成功后的全部序列（每个出口都落 run 审计行 + finalize 回写）。
func (r *Runner) execute(ctx context.Context, lg loggateway.Logger, conn bizsync.Connector, out RunOutcome, st *runState) RunOutcome {
	startedAt := r.now()
	defer func() {
		out.Stats = st.stats
	}()

	walker, ok := r.d.Walkers[conn.Kind]
	if !ok {
		uerr := fmt.Errorf("walker for kind %q not registered", conn.Kind)
		return r.finalize(ctx, lg, conn, &out, st, startedAt,
			bizsync.RunStatusFailed, bizsync.SyncStateError, bizsync.ErrCodeWalkFailed,
			uerr.Error(), conn.Cursor, uerr)
	}
	applier, ok := r.d.Appliers[conn.Kind]
	if !ok {
		uerr := fmt.Errorf("applier for kind %q not registered", conn.Kind)
		return r.finalize(ctx, lg, conn, &out, st, startedAt,
			bizsync.RunStatusFailed, bizsync.SyncStateError, bizsync.ErrCodeWalkFailed,
			uerr.Error(), conn.Cursor, uerr)
	}

	// ⑥-1 保留集载入
	prevActive, err := r.d.Items.CountActiveItems(ctx, conn.ID)
	if err != nil {
		return r.finalize(ctx, lg, conn, &out, st, startedAt,
			bizsync.RunStatusFailed, bizsync.SyncStateError, bizsync.ErrCodeWalkFailed,
			bizsync.TruncateErrorMessage("count active: "+err.Error()), conn.Cursor, err)
	}
	st.stats.PrevActive = prevActive

	// ⑥-2 walk 标记（emit 闭包）
	emit := r.makeEmit(ctx, lg, conn, walker, applier, out.RunToken, st)
	newCursor, walkErr := walker.Walk(ctx, conn, CursorState(conn.Cursor), emit)

	// 错误归一：预算哨兵≠失败；失地哨兵→superseded 收轮；auth 整轮闸；其他 abort。
	if errors.Is(walkErr, ErrBudgetExhausted) {
		st.budgetHit = true
		st.walkDone = false // 部分 walk——游标不动、跳删除
		walkErr = nil
	} else if errors.Is(walkErr, errClaimLost) {
		st.lost.Store(true) // 与心跳旗合流（写面止步的唯一判官）
		st.walkDone = false
		walkErr = nil
	} else {
		st.walkDone = walkErr == nil
	}
	if walkErr != nil {
		if code, isAuth := AuthCodeOf(walkErr); isAuth {
			st.authHit, st.authCode = true, code
		}
	}
	st.cursorNext = newCursor // walker 契约：出错时等于 prev（游标不动）

	switch {
	case st.lost.Load():
		// fencing：被挤下的旧主全线止步—— finalize 必 ClaimLost（token 失配/行删），
		// 但审计行照落（登记裁决不变）。
		return r.finalize(ctx, lg, conn, &out, st, startedAt,
			bizsync.RunStatusFailed, bizsync.SyncStateError, bizsync.ErrCodeWalkFailed,
			"run superseded: claim lost mid-run", conn.Cursor, errClaimLost)
	case st.authHit:
		// ⑥-4 auth 整轮闸：auth_paused 收轮、跳删除、游标不动、不入退避
		return r.finalize(ctx, lg, conn, &out, st, startedAt,
			bizsync.RunStatusFailed, bizsync.SyncStateAuthPaused, st.authCode,
			bizsync.TruncateErrorMessage(authMessage(st.authCode)), conn.Cursor, walkErrForCause(walkErr, st))
	case walkErr != nil:
		// walker abort：零删除零圈定；已出席标记保留（出席是事实）
		return r.finalize(ctx, lg, conn, &out, st, startedAt,
			bizsync.RunStatusFailed, bizsync.SyncStateError, bizsync.ErrCodeWalkFailed,
			bizsync.TruncateErrorMessage("walk: "+walkErr.Error()), conn.Cursor, walkErr)
	case st.walkDone && st.stats.Seen == 0 && st.stats.PrevActive > 0:
		// ⑥-4 walk_empty_abort 通用闸：空 walk ∧ 上轮 active>0 → 跳过删除
		return r.finalize(ctx, lg, conn, &out, st, startedAt,
			bizsync.RunStatusFailed, bizsync.SyncStateError, bizsync.ErrCodeWalkEmptyAbort,
			"walk yielded zero items while previous run had active items", conn.Cursor, nil)
	case st.budgetHit:
		// 工作预算耗尽：partial 收轮（已提交工作有效）、跳删除、游标不动
		return r.finalize(ctx, lg, conn, &out, st, startedAt,
			bizsync.RunStatusPartial, bizsync.SyncStateIdle, bizsync.ErrCodeNone,
			"", conn.Cursor, nil)
	}

	// ── ⑥-3 候选圈定（仅完满 walk + 未失地） ──
	if st.lost.Load() {
		return r.finalize(ctx, lg, conn, &out, st, startedAt,
			bizsync.RunStatusFailed, bizsync.SyncStateError, bizsync.ErrCodeWalkFailed,
			"run superseded: claim lost pre-marking", conn.Cursor, errClaimLost)
	}
	if _, err := r.d.Items.MarkAbsentItemsMissing(ctx, conn.ID, out.RunToken); err != nil {
		return r.finalize(ctx, lg, conn, &out, st, startedAt,
			bizsync.RunStatusFailed, bizsync.SyncStateError, bizsync.ErrCodeConfirmFailed,
			bizsync.TruncateErrorMessage("mark absent: "+err.Error()), conn.Cursor, err)
	}

	// ── ⑥-5/⑥-6/⑥-7 三态确认 + 预算截断 + 级联（失地随检止步） ──
	// 候选窗（评审 W2 修注册）：显式 max(500, 3×max_deletes)——PG repo 对 limit<=0
	// 静默钳 500，口径在此唯一显式化；Unknown 大队首挤占时预算消耗仍受
	// maxDeletesPerRun 闸（fail-safe 方向），超窗候选留 missing 下轮续议。
	candLimit := conn.MaxDeletesPerRun * 3
	if candLimit < 500 {
		candLimit = 500
	}
	candidates, err := r.d.Items.ListItemsByStatus(ctx, conn.ID, bizsync.ItemStatusMissing, candLimit)
	if err != nil {
		return r.finalize(ctx, lg, conn, &out, st, startedAt,
			bizsync.RunStatusFailed, bizsync.SyncStateError, bizsync.ErrCodeConfirmFailed,
			bizsync.TruncateErrorMessage("list missing: "+err.Error()), conn.Cursor, err)
	}
	superseded := false
	for _, it := range candidates {
		if st.lost.Load() {
			superseded = true
			break
		}
		if st.stats.Deleted >= conn.MaxDeletesPerRun {
			break // ⑥-6 预算截断：余数留 missing 下轮再议
		}
		verdict := walker.ConfirmDeleted(ctx, conn, it.ExternalID)
		if st.lost.Load() {
			superseded = true // 源侧直查已发出，结果不再消费——新主写面优先
			break
		}
		if verdict != bizsync.VerdictConfirmed {
			st.stats.MissUnknown++ // Unknown/StillExists：保留 missing 不降级
			continue
		}
		if it.OwnLifecycle && it.DocumentID != "" {
			// ⑥-7 级联唯一正道：own_lifecycle=true 才动文档级删
			if derr := applier.Delete(ctx, conn, it); derr != nil {
				st.stats.MissUnknown++
				lg.Warn("sourcesync cascade delete failed, item stays missing",
					loggateway.Str("external_id", it.ExternalID),
					loggateway.Str("document_id", it.DocumentID),
					loggateway.Err(derr))
				continue
			}
		}
		if _, derr := r.d.Items.MarkItemDeleted(ctx, conn.ID, it.ExternalID, r.now()); derr != nil {
			st.stats.MissUnknown++
			continue
		}
		st.stats.Deleted++
	}
	if superseded {
		return r.finalize(ctx, lg, conn, &out, st, startedAt,
			bizsync.RunStatusFailed, bizsync.SyncStateError, bizsync.ErrCodeWalkFailed,
			"run superseded: claim lost mid-deletion", conn.Cursor, errClaimLost)
	}

	// ── 收轮：success / partial（walk 完满前提） ──
	if st.stats.FetchFailed > 0 {
		return r.finalize(ctx, lg, conn, &out, st, startedAt,
			bizsync.RunStatusPartial, bizsync.SyncStateIdle, bizsync.ErrCodeNone, "", st.cursorNext, nil)
	}
	return r.finalize(ctx, lg, conn, &out, st, startedAt,
		bizsync.RunStatusSuccess, bizsync.SyncStateIdle, bizsync.ErrCodeNone, "", st.cursorNext, nil)
}

// walkErrForCause auth 闸的 cause 载体（authHit 时 walkErr 即 AuthError 原文）。
func walkErrForCause(walkErr error, st *runState) error {
	if st.authHit {
		return walkErr
	}
	return nil
}

// makeEmit ⑥-2 walk 标记闭包：Get→needsWork 判定→Fetch→Apply→Upsert 出席。
// 判定铁律：needsWork＝新项 ∨ DocumentID=="" ∨ ContentHash=="" ∨ 指纹漂移——
// 预算/失败跳过的项不得推进指纹（保留旧指纹或留空绑定，下轮自然重试）。
func (r *Runner) makeEmit(
	ctx context.Context, lg loggateway.Logger, conn bizsync.Connector,
	walker Walker, applier Applier, runToken string, st *runState,
) func(Snapshot) error {
	seen := func(it bizsync.Item, found bool, externalID, fingerprint string, at time.Time) error {
		docID, own := "", false
		if found {
			docID, own = it.DocumentID, it.OwnLifecycle
		}
		return r.d.Items.UpsertItemSeen(ctx, conn.ID, externalID, docID, fingerprint, own, runToken, at)
	}
	return func(snap Snapshot) error {
		if st.lost.Load() {
			return errClaimLost // fencing 顶闸：心跳失地→即刻中止 walk（游标不动）
		}
		st.stats.Seen++
		if snap.ExternalID == "" {
			return fmt.Errorf("sourcesync: walker %q emitted empty external id", conn.Kind)
		}
		at := r.now()
		existing, found, err := r.d.Items.GetItem(ctx, conn.ID, snap.ExternalID)
		if err != nil {
			return err // 仓口故障→walk abort（归 walk_failed，文本带真相）
		}
		// 铁律＋复活条款（评审 W2 修）：deleted 项重新出席必须重取重绑——否则
		// 指针仍挂旧 document_id（文档或已级联删除）成悬挂绑定。
		needsWork := !found || existing.DocumentID == "" || existing.ContentHash == "" ||
			existing.Fingerprint != snap.Fingerprint || existing.Status == bizsync.ItemStatusDeleted
		if !needsWork {
			st.stats.Unchanged++
			return seen(existing, found, snap.ExternalID, snap.Fingerprint, at)
		}
		// 工作预算闸：items 计数或字节耗尽即哨兵中止整条 walk（游标不动）
		if st.workLeft <= 0 || st.stats.BytesPulled >= conn.MaxFetchBytesPerRun {
			return ErrBudgetExhausted
		}
		preserveFP := snap.Fingerprint
		if found {
			preserveFP = existing.Fingerprint // 失败/跳过路径保留旧指纹，下轮重试
		}
		payload, ferr := walker.Fetch(ctx, conn, snap.ExternalID)
		if ferr != nil {
			if ctx.Err() != nil {
				return ctx.Err() // 取消传播不记病态
			}
			if _, isAuth := AuthCodeOf(ferr); isAuth {
				return ferr // auth 整轮闸：中止 walk（walker 契约原样返回）
			}
			st.stats.FetchFailed++
			if uerr := seen(existing, found, snap.ExternalID, preserveFP, at); uerr != nil {
				return uerr
			}
			r.noteItemError(ctx, lg, conn, snap, "fetch", ferr)
			return nil
		}
		st.stats.BytesPulled += payload.SizeBytes
		apOut, aerr := applier.Upsert(ctx, conn, snap, payload)
		if aerr != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			st.stats.FetchFailed++ // W2 合口：提交失败同桶
			if uerr := seen(existing, found, snap.ExternalID, preserveFP, at); uerr != nil {
				return uerr
			}
			r.noteItemError(ctx, lg, conn, snap, "apply", aerr)
			return nil
		}
		// 提交成功才回填绑定与 checkpoint hash（设计：提交成功=checkpoint 推进点）
		if uerr := seen(existing, found, snap.ExternalID, snap.Fingerprint, at); uerr != nil {
			return uerr
		}
		if _, uerr := r.d.Items.SetItemDocument(ctx, conn.ID, snap.ExternalID, apOut.DocumentID, apOut.OwnLifecycle); uerr != nil {
			return uerr
		}
		if _, uerr := r.d.Items.SetItemContentHash(ctx, conn.ID, snap.ExternalID, apOut.ContentHash); uerr != nil {
			return uerr
		}
		st.workLeft--
		if found {
			st.stats.Updated++
		} else {
			st.stats.Created++
		}
		return nil
	}
}

// noteItemError 逐条病态记账（出席已先行清旧账，此为当轮失败留痕）。
func (r *Runner) noteItemError(ctx context.Context, lg loggateway.Logger, conn bizsync.Connector, snap Snapshot, phase string, cause error) {
	msg := bizsync.TruncateErrorMessage(fmt.Sprintf("%s: %s: %v", phase, snap.DisplayPath, cause))
	if _, err := r.d.Items.SetItemError(ctx, conn.ID, snap.ExternalID, msg); err != nil {
		lg.Warn("sourcesync set item error failed",
			loggateway.Str("external_id", snap.ExternalID), loggateway.Err(err))
	}
}

// finalize ⑥-8 收尾唯一出口：InsertRunWithRetention → FinalizeRunWithClaim
// + done 帧。run 审计行插入失败仅记日志仍续行收轮（宁缺审计不留僵尸）。
// cause 贯通 RunOutcome.Error（评审 W2 修：观测面死字段回填——调用侧/测试
// 可取原始错误链；errMsg 仍是落库截断口径）。
func (r *Runner) finalize(
	ctx context.Context, lg loggateway.Logger, conn bizsync.Connector,
	out *RunOutcome, st *runState, startedAt time.Time,
	status bizsync.RunStatus, syncState bizsync.SyncState,
	code bizsync.ErrorCode, errMsg string, cursorToStore string, cause error,
) RunOutcome {
	out.Stats = st.stats
	out.Status = status
	out.SyncState = syncState
	out.Error = cause
	if code.ByteStable() {
		out.ErrorCode = code
	} else {
		out.ErrorCode = bizsync.ErrCodeWalkFailed
	}
	now := r.now()

	// 状态机定值（⑤/③-1 铁律）：
	fc := conn
	fc.SyncState = syncState
	fc.Cursor = cursorToStore
	fc.LastRunAt = now
	fc.ErrorCode = out.ErrorCode
	fc.ErrorMessage = errMsg
	switch status {
	case bizsync.RunStatusSuccess, bizsync.RunStatusPartial:
		fc.ConsecutiveFailures = 0
		fc.NextRunAt = now.Add(time.Duration(conn.IntervalSeconds) * time.Second)
		if st.walkDone {
			fc.LastSuccessAt = now // 完满 walk（含 partial）才推成功水位
		} else {
			fc.LastSuccessAt = conn.LastSuccessAt
		}
	case bizsync.RunStatusFailed:
		if syncState == bizsync.SyncStateAuthPaused {
			// ⑥-4：auth 不进退避、不动 next_run_at（等凭据 PUT 唤醒）
			fc.ConsecutiveFailures = conn.ConsecutiveFailures
			fc.NextRunAt = conn.NextRunAt
			fc.LastSuccessAt = conn.LastSuccessAt
		} else {
			fc.ConsecutiveFailures = conn.ConsecutiveFailures + 1
			fc.NextRunAt = now.Add(bizsync.BackoffDuration(fc.ConsecutiveFailures))
			fc.LastSuccessAt = conn.LastSuccessAt
		}
	}

	// 审计行（顺序钉：先 INSERT 后 finalize；插入失败只记日志继续收轮）
	run := bizsync.Run{
		ID: out.RunID, ConnectorID: conn.ID, StartedAt: startedAt, FinishedAt: now,
		Status: status, Created: st.stats.Created, Updated: st.stats.Updated,
		Unchanged: st.stats.Unchanged, FetchFailed: st.stats.FetchFailed,
		Deleted: st.stats.Deleted, MissUnknown: st.stats.MissUnknown,
		BytesPulled: st.stats.BytesPulled, ErrorCode: out.ErrorCode,
		ErrorMessage: bizsync.TruncateErrorMessage(errMsg),
	}
	if err := r.d.Runs.InsertRunWithRetention(ctx, run); err != nil {
		lg.Error("sourcesync run audit insert failed (continuing finalize)",
			loggateway.RunID(out.RunID), loggateway.Err(err))
	}

	ok, err := r.d.Connectors.FinalizeRunWithClaim(ctx, fc, out.RunToken)
	if err != nil {
		lg.Error("sourcesync finalize connector failed",
			loggateway.RunID(out.RunID), loggateway.Err(err))
	} else if !ok {
		out.ClaimLost = true
		lg.Warn("sourcesync finalize claim lost (reclaimed or soft-deleted)",
			loggateway.RunID(out.RunID))
	} else {
		out.Finalized = true
	}
	r.d.Frames.OnRunFinished(ctx, RunEvent{
		ConnectorID: conn.ID, Kind: conn.Kind, RunID: out.RunID, RunToken: out.RunToken,
		Stats: out.Stats, Status: status, ErrorCode: out.ErrorCode,
	})
	lg.Info("sourcesync run finished",
		loggateway.Str("status", string(status)),
		loggateway.Str("sync_state", string(syncState)),
		loggateway.Str("error_code", string(out.ErrorCode)),
		loggateway.Int("seen", st.stats.Seen),
		loggateway.Int("created", st.stats.Created),
		loggateway.Int("updated", st.stats.Updated),
		loggateway.Int("unchanged", st.stats.Unchanged),
		loggateway.Int("fetch_failed", st.stats.FetchFailed),
		loggateway.Int("deleted", st.stats.Deleted),
		loggateway.Int("miss_unknown", st.stats.MissUnknown),
		loggateway.Int64("bytes", st.stats.BytesPulled),
		loggateway.Bool("budget_hit", st.budgetHit),
		loggateway.Duration(now.Sub(startedAt).Milliseconds()))
	return *out
}

// startHeartbeat 租约续期协程（fencing 主径：失地即置 lost 旗，emit 顶闸/
// ⑥-3 前闸/删除循环随检三处读旗止步；收轮 CAS 是兜底不是判官）。
func (r *Runner) startHeartbeat(ctx context.Context, connectorID, runToken string, lg loggateway.Logger, st *runState) chan struct{} {
	stop := make(chan struct{})
	go func() {
		t := time.NewTicker(r.hb)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ctx.Done():
				return
			case <-t.C:
				ok, err := r.d.Connectors.HeartbeatClaim(ctx, connectorID, runToken, r.now())
				if err != nil {
					lg.Warn("sourcesync heartbeat failed", loggateway.Err(err))
					continue
				}
				if !ok {
					st.lost.Store(true)
					lg.Warn("sourcesync heartbeat claim lost mid-run (fencing engaged)")
					return
				}
			}
		}
	}()
	return stop
}

// authMessage ⑥-4 闸错误消息（静态安全文本，无动态片段）。
func authMessage(code bizsync.ErrorCode) string {
	switch code {
	case bizsync.ErrCodeCredentialMissing:
		return "credential not configured; paused until credential PUT"
	case bizsync.ErrCodeCredentialDecryptFailed:
		return "credential decrypt failed; paused until credential PUT"
	default:
		return "upstream authorization rejected (401/403); paused until credential PUT"
	}
}

// ── ID 生成（crypto/rand 主径 + nano 兜底） ──

func newRunToken() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("ss:tok:%d", time.Now().UnixNano())
	}
	return runTokenPrefix + hex.EncodeToString(b[:])
}

func newRunID(connectorID string, startedAt time.Time) string {
	return fmt.Sprintf("%s%s:%d", runIDPrefix, connectorID, startedAt.UnixNano())
}
