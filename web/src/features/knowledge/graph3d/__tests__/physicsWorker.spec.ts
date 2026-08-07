/**
 * physicsWorker.spec：G5-B Worker tick 消息回归（红线④姊妹条款——出向也必须 slice 后 transfer）。
 *
 * 回归背景：fast-graph 原版 `engine.positions.slice()` 后 transfer；若直接 transfer
 * 引擎内部 buffer，首个 tick 后 Worker 侧 positions 被 detach（byteLength=0），
 * 后续 tick 读到 undefined → 力 NaN → 布局崩坏。本测试锁住「slice 语义」。
 */
import { describe, expect, it } from 'vitest';
import { FORCE_DEFAULTS, ForceEngine } from '../forces';
import { buildTickMessage } from '../physics.worker';

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
