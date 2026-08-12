/**
 * forces.spec：G5-A 物理引擎契约（设计 §V12.8-1 forces.ts）。
 * 5 力模型 + maxStep 钳制 + alphaDecay=0.0228/alphaMin=0.005 + 分层 chargeScale。
 */
import { describe, expect, it } from 'vitest';
import { FORCE_DEFAULTS, GALAXY_FORCE_PARAMS, ForceEngine, type ForceParams } from '../forces';

function mkParams(over: Partial<ForceParams> = {}): ForceParams {
  return { ...FORCE_DEFAULTS, ...over };
}

/** 构造 count 节点、edges 为扁平索引对的引擎，positions 显式给定。 */
function mkEngine(
  count: number,
  edges: number[],
  positions: number[],
  params = mkParams(),
  groupId?: Uint16Array,
  chargeScale?: Float32Array,
) {
  return new ForceEngine({
    count,
    edges: Int32Array.from(edges),
    positions: Float32Array.from(positions),
    params,
    groupId,
    chargeScale,
  });
}

describe('ForceEngine 基本力', () => {
  it('弹簧：距离 > linkDistance 时相互靠近', () => {
    const e = mkEngine(
      2,
      [0, 1],
      [0, 0, 0, 100, 0, 0],
      mkParams({ repulsion: 0, gravity: 0, groupCohesion: 0, groupSeparation: 0 }),
    );
    e.tick();
    const p = e.positions;
    expect(p[0]).toBeGreaterThan(0); // a 向 b 靠拢
    expect(p[3]).toBeLessThan(100); // b 向 a 靠拢
  });

  it('斥力：无弹簧时相互远离', () => {
    const e = mkEngine(
      2,
      [],
      [0, 0, 0, 10, 0, 0],
      mkParams({ linkStrength: 0, gravity: 0, groupCohesion: 0, groupSeparation: 0 }),
    );
    const before = 10;
    e.tick();
    const after = e.positions[3] - e.positions[0];
    expect(after).toBeGreaterThan(before);
  });

  it('向心力：远距节点被拉向原点', () => {
    const e = mkEngine(
      1,
      [],
      [500, 0, 0],
      mkParams({ repulsion: 0, linkStrength: 0, groupCohesion: 0, groupSeparation: 0 }),
    );
    e.tick();
    expect(e.positions[0]).toBeLessThan(500);
  });

  it('簇凝聚：同组节点向组中心靠拢', () => {
    // 两个节点同组，相距 200，无其他力
    const e = mkEngine(
      2,
      [],
      [-100, 0, 0, 100, 0, 0],
      mkParams({ repulsion: 0, linkStrength: 0, gravity: 0, groupSeparation: 0 }),
      new Uint16Array([0, 0]),
    );
    e.tick();
    expect(e.positions[0]).toBeGreaterThan(-100);
    expect(e.positions[3]).toBeLessThan(100);
  });

  it('簇分离：两组中心相互远离', () => {
    const e = mkEngine(
      2,
      [],
      [-20, 0, 0, 20, 0, 0],
      mkParams({ repulsion: 0, linkStrength: 0, gravity: 0, groupCohesion: 0 }),
      new Uint16Array([0, 1]),
    );
    e.tick();
    expect(e.positions[0]).toBeLessThan(-20);
    expect(e.positions[3]).toBeGreaterThan(20);
  });
});

describe('ForceEngine 稳定性', () => {
  it('maxStep 钳制：高刚度 hub 单 tick 位移 ≤ linkDistance', () => {
    // 1 个 hub 连 200 个叶节点，弹簧总刚度巨大，无钳制会发散
    const count = 201;
    const edges: number[] = [];
    const positions: number[] = [0, 0, 0];
    for (let i = 1; i < count; i++) {
      edges.push(0, i);
      const a = (i / count) * Math.PI * 2;
      positions.push(Math.cos(a) * 200, Math.sin(a) * 200, (i % 7) * 10);
    }
    const e = mkEngine(count, edges, positions);
    const before = Array.from(e.positions.slice(0, 3));
    e.tick();
    const p = e.positions;
    const step = Math.hypot(p[0] - before[0], p[1] - before[1], p[2] - before[2]);
    expect(step).toBeLessThanOrEqual(FORCE_DEFAULTS.linkDistance + 1e-4);
    expect(Number.isFinite(step)).toBe(true);
  });

  it('长程收敛：240 tick 后无 NaN 且 alpha < alphaMin（settled）', () => {
    const count = 50;
    const edges: number[] = [];
    for (let i = 1; i < count; i++) edges.push(0, i);
    const positions: number[] = [];
    for (let i = 0; i < count; i++) positions.push((i % 9) * 40, (i % 7) * 40, (i % 5) * 40);
    const e = mkEngine(count, edges, positions);
    for (let t = 0; t < 240; t++) e.tick();
    expect(e.settled).toBe(true);
    for (let i = 0; i < count * 3; i++) expect(Number.isFinite(e.positions[i])).toBe(true);
  });

  it('alpha 按 0.0228 衰减', () => {
    const e = mkEngine(1, [], [0, 0, 0], mkParams({ repulsion: 0, gravity: 0, groupCohesion: 0, groupSeparation: 0 }));
    e.tick();
    expect(e.alpha).toBeCloseTo(1 - 0.0228, 5);
  });

  it('pin：被 pin 节点不动，pin 立即 reheat（alpha 回 1）', () => {
    const e = mkEngine(2, [0, 1], [0, 0, 0, 100, 0, 0]);
    e.tick(); // alpha 先衰减
    expect(e.alpha).toBeLessThan(1);
    e.pin(0, 5, 6, 7);
    expect(e.alpha).toBe(1); // pin 触发 reheat（tick 前检查）
    e.tick();
    expect(e.positions[0]).toBe(5);
    expect(e.positions[1]).toBe(6);
    expect(e.positions[2]).toBe(7);
    e.unpin(0);
    e.tick();
    // unpin 后重新受力（不再恒等于 pin 位置）
  });

  it('分层 chargeScale：高 charge 节点单 tick 斥力位移更大', () => {
    // 两个节点等距受第三个节点斥力；chargeScale 不同 → 位移不同
    const mk = (scale: number[]) =>
      mkEngine(
        3,
        [],
        [0, 0, 0, 50, 0, 0, -50, 0, 0],
        mkParams({ linkStrength: 0, gravity: 0, groupCohesion: 0, groupSeparation: 0 }),
        undefined,
        Float32Array.from(scale),
      );
    const e1 = mk([1, 1, 1]);
    const e2 = mk([1, 2.92, 1]); // 节点 1 charge 放大
    e1.tick();
    e2.tick();
    const d1 = Math.abs(e1.positions[3] - 50);
    const d2 = Math.abs(e2.positions[3] - 50);
    expect(d2).toBeGreaterThan(d1);
  });
});

describe('M2 星系盘三力', () => {
  function makeGalaxyEngine(params: Partial<ForceParams>): ForceEngine {
    const count = 3;
    const positions = new Float32Array([10, 8, 0, -20, -4, 10, 30, 2, -15]);
    return new ForceEngine({
      count,
      edges: new Int32Array([0, 1]),
      positions,
      params: { ...FORCE_DEFAULTS, ...params },
    });
  }

  it('默认参数三力为 0：力导向行为不变（回归）', () => {
    expect(FORCE_DEFAULTS.coreGravity).toBe(0);
    expect(FORCE_DEFAULTS.discFlatten).toBe(0);
    expect(FORCE_DEFAULTS.spiralSwirl).toBe(0);
  });

  it('discFlatten>0：Y 坐标绝对值收敛（压向 XZ 盘面）', () => {
    const e = makeGalaxyEngine({ discFlatten: 0.12, repulsion: 0, linkStrength: 0, gravity: 0, groupCohesion: 0, groupSeparation: 0 });
    const before = Math.abs(e.positions[1]); // 节点0 的 y=8
    for (let t = 0; t < 40; t++) e.tick();
    expect(Math.abs(e.positions[1])).toBeLessThan(before);
  });

  it('spiralSwirl>0：产生 XZ 平面切向速度（角度位置变化）', () => {
    const e = makeGalaxyEngine({ spiralSwirl: 0.05, repulsion: 0, linkStrength: 0, gravity: 0, groupCohesion: 0, groupSeparation: 0 });
    const angleBefore = Math.atan2(e.positions[2], e.positions[0]); // 节点0 (10,8,0) → atan2(0,10)=0
    for (let t = 0; t < 10; t++) e.tick();
    const angleAfter = Math.atan2(e.positions[2], e.positions[0]);
    expect(angleAfter).not.toBeCloseTo(angleBefore, 5);
  });

  it('coreGravity>0：径向距离收缩快于纯线性 gravity', () => {
    const lin = makeGalaxyEngine({ gravity: 0.011, repulsion: 0, linkStrength: 0, groupCohesion: 0, groupSeparation: 0 });
    const core = makeGalaxyEngine({ gravity: 0, coreGravity: 0.08, repulsion: 0, linkStrength: 0, groupCohesion: 0, groupSeparation: 0 });
    const r0 = Math.hypot(core.positions[0], core.positions[1], core.positions[2]);
    for (let t = 0; t < 30; t++) { lin.tick(); core.tick(); }
    const rLin = Math.hypot(lin.positions[0], lin.positions[1], lin.positions[2]);
    const rCore = Math.hypot(core.positions[0], core.positions[1], core.positions[2]);
    expect(rCore).toBeLessThan(rLin);
    expect(rCore).toBeLessThan(r0);
  });

  it('GALAXY_FORCE_PARAMS 预设：三力启用且默认 gravity 减弱', () => {
    expect(GALAXY_FORCE_PARAMS.coreGravity).toBeGreaterThan(0);
    expect(GALAXY_FORCE_PARAMS.discFlatten).toBeGreaterThan(0);
    expect(GALAXY_FORCE_PARAMS.spiralSwirl).toBeGreaterThan(0);
    expect(GALAXY_FORCE_PARAMS.gravity).toBeLessThan(FORCE_DEFAULTS.gravity);
  });
});
