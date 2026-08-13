/**
 * physicsWorker.spec：G5-B Worker tick 消息回归（红线④姊妹条款——出向也必须 slice 后 transfer）。
 *
 * 回归背景：fast-graph 原版 `engine.positions.slice()` 后 transfer；若直接 transfer
 * 引擎内部 buffer，首个 tick 后 Worker 侧 positions 被 detach（byteLength=0），
 * 后续 tick 读到 undefined → 力 NaN → 布局崩坏。本测试锁住「slice 语义」。
 */
import { describe, expect, it } from 'vitest';
import { FORCE_DEFAULTS, ForceEngine } from '../forces';
import { buildTickMessage, nextTickDelay } from '../physics.worker';

function mkEngine(): ForceEngine {
  return new ForceEngine({
    count: 3,
    edges: new Int32Array([0, 1, 1, 2]),
    positions: new Float32Array([0, 0, 0, 10, 0, 0, 20, 0, 0]),
    params: { ...FORCE_DEFAULTS },
  });
}

describe('buildTickMessage（Worker 出向 tick）', () => {
  it('tick positions 是独立副本：值一致但 buffer 与引擎隔离', () => {
    const eng = mkEngine();
    eng.tick();
    const { msg, transfer } = buildTickMessage(eng);
    expect(msg.type).toBe('tick');
    if (msg.type !== 'tick') return;
    // 关键断言：msg buffer ≠ 引擎 buffer（直接 transfer 引擎 buffer 时此断言红）
    expect(msg.positions).not.toBe(eng.positions);
    expect(msg.positions.buffer).not.toBe(eng.positions.buffer);
    expect(Array.from(msg.positions)).toEqual(Array.from(eng.positions));
    expect(transfer).toContain(msg.positions.buffer);
  });

  it('模拟 transfer detach 后引擎仍可继续 tick（位置有限值）', () => {
    const eng = mkEngine();
    eng.tick();
    const { msg } = buildTickMessage(eng);
    if (msg.type !== 'tick') return;
    // 模拟 postMessage transfer 的 detach 语义（ES2024，环境支持时执行）
    const proto = ArrayBuffer.prototype as ArrayBuffer & { transfer?: () => ArrayBuffer };
    if (typeof proto.transfer === 'function') {
      proto.transfer.call(msg.positions.buffer);
      expect(msg.positions.byteLength).toBe(0); // 已 detach
      expect(eng.positions.byteLength).toBeGreaterThan(0); // 引擎 buffer 完好
    }
    eng.tick();
    for (const v of eng.positions) expect(Number.isFinite(v)).toBe(true);
  });

  it('alpha 原样携带', () => {
    const eng = mkEngine();
    eng.alpha = 0.42;
    const { msg } = buildTickMessage(eng);
    expect(msg.alpha).toBe(0.42);
  });
});

/**
 * nextTickDelay 棘轮回归（2026-08-13 P0，基准实测 2k 收敛 116s→4.5s）：
 * 旧实现把「上一拍实际间隔 elapsed」计入补偿，单向锁存形成棘轮——一次 GC/抢占抖动
 * 即把节奏永久抬高（~425ms/拍）。修复后延迟只由本拍耗时决定，无状态、不锁存。
 */
describe('nextTickDelay（反棘轮节奏补偿）', () => {
  it('本拍耗时 <16ms：补足到 16ms 节奏', () => {
    expect(nextTickDelay(5)).toBe(11);
    expect(nextTickDelay(0)).toBe(16);
    expect(nextTickDelay(15.9)).toBeCloseTo(0.1, 5);
  });

  it('本拍耗时 ≥16ms：立即下一拍（delay=0，不倒扣）', () => {
    expect(nextTickDelay(16)).toBe(0);
    expect(nextTickDelay(400)).toBe(0);
  });

  it('反棘轮：抖动不影响后续延迟（同输入恒同输出，无状态锁存）', () => {
    // 模拟一次 400ms 抖动后，后续正常拍的延迟必须与抖动前一致
    const before = nextTickDelay(5);
    nextTickDelay(400); // 抖动拍
    expect(nextTickDelay(5)).toBe(before);
  });
});
