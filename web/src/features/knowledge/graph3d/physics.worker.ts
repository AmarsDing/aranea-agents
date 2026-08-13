/**
 * physics.worker：G5 物理 Worker（Vite `?worker` 引入）。
 *
 * 移植 fast-graph physics.worker：
 * - init 后 16ms tick 循环（耗时补偿）
 * - tick 回传 positions 必须 slice 后 transfer（直接 transfer 引擎 buffer 会被 detach，
 *   下一 tick 读 undefined → NaN 发散——fast-graph 原版即 slice）
 * - alpha < alphaMin 自停并发 stopped（省 CPU）；reheat/pin 重新唤醒
 */
import { FORCE_DEFAULTS, ForceEngine } from './forces';
import type { InMessage, OutMessage, TickMessage } from './protocol';

const post = (msg: OutMessage, transfer?: Transferable[]): void => {
  const g = globalThis as { postMessage: (m: OutMessage, t?: Transferable[]) => void };
  if (transfer) g.postMessage(msg, transfer);
  else g.postMessage(msg);
};

/** tick 消息构造：slice 副本再 transfer（引擎内部 buffer 不被 detach）。导出供单测。 */
export function buildTickMessage(engine: ForceEngine): { msg: TickMessage; transfer: Transferable[] } {
  const out = engine.positions.slice();
  return { msg: { type: 'tick', positions: out, alpha: engine.alpha }, transfer: [out.buffer as ArrayBuffer] };
}

let engine: ForceEngine | null = null;
let interval: ReturnType<typeof setTimeout> | null = null;
let running = false;

const TICK_MS = 16;

/**
 * 下一拍延迟（导出供单测）：只按本拍耗时补到 16ms 节奏。
 * 红线：禁止把「上一拍间隔 elapsed」计入补偿——elapsed 单向锁存会形成棘轮，
 * 一次调度抖动（GC/主线程抢占）即把节奏永久抬高（基准实测 2k 节点收敛被拖到 116s）。
 */
export function nextTickDelay(costMs: number): number {
  return Math.max(0, TICK_MS - costMs);
}

function startLoop(): void {
  if (running) return;
  running = true;
  const step = (): void => {
    if (!running || !engine) return;
    const now = performance.now();
    engine.tick();
    // 末帧也回传（fast-graph 语义：stopped 前的最终位置不丢）
    const { msg, transfer } = buildTickMessage(engine);
    post(msg, transfer);
    if (engine.alpha < engine.alphaMin) {
      running = false;
      post({ type: 'stopped' });
      return;
    }
    // 耗时补偿：保持 ~16ms 节奏（抖动自恢复，不棘轮锁存）
    interval = setTimeout(step, nextTickDelay(performance.now() - now));
  };
  interval = setTimeout(step, TICK_MS);
}

function stopLoop(): void {
  running = false;
  if (interval !== null) {
    clearTimeout(interval);
    interval = null;
  }
}

const g = globalThis as { onmessage: ((e: MessageEvent<InMessage>) => void) | null };
g.onmessage = (e: MessageEvent<InMessage>): void => {
  const m = e.data;
  switch (m.type) {
    case 'init': {
      engine = new ForceEngine({
        count: m.count,
        edges: m.edges,
        positions: m.positions,
        params: { ...FORCE_DEFAULTS, ...m.params },
        groupId: m.groupId,
        chargeScale: m.chargeScale,
      });
      startLoop();
      break;
    }
    case 'setParams': {
      engine?.setParams(m.params);
      break;
    }
    case 'pin': {
      if (engine) {
        engine.pin(m.i, m.x, m.y, m.z);
        startLoop();
      }
      break;
    }
    case 'unpin': {
      engine?.unpin(m.i);
      break;
    }
    case 'reheat': {
      if (engine) {
        engine.reheat();
        startLoop();
      }
      break;
    }
    case 'stop': {
      stopLoop();
      engine = null;
      break;
    }
  }
};
