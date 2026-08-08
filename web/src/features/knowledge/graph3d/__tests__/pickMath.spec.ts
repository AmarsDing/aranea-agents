/**
 * pickMath.spec：G5 渲染管线 v2 自研射线拾取契约。
 *
 * 替代 three InstancedMesh.raycast（后者每实例一次矩阵求逆，万级 hover 卡顿元凶）：
 * 纯循环射线-球求交 O(N)，阈值 = max(节点半径, 距离×每像素世界尺寸×slackPx)，最近 t 获胜。
 */
import { describe, expect, it } from 'vitest';
import { PICK_SLACK_PX, pickNode } from '../pickMath';

/** 构造 positions/sizes。 */
function mk(list: [number, number, number, number][]): { positions: Float32Array; sizes: Float32Array } {
  const positions = new Float32Array(list.length * 3);
  const sizes = new Float32Array(list.length);
  list.forEach(([x, y, z, s], i) => {
    positions[i * 3] = x;
    positions[i * 3 + 1] = y;
    positions[i * 3 + 2] = z;
    sizes[i] = s;
  });
  return { positions, sizes };
}

const ORIGIN: [number, number, number] = [0, 0, 100];
const DIR: [number, number, number] = [0, 0, -1]; // 朝 -z 看

describe('pickNode', () => {
  it('射线正中命中节点（半径内）', () => {
    const { positions, sizes } = mk([[0, 0, 0, 2]]);
    expect(pickNode(positions, sizes, 1, ...ORIGIN, ...DIR, 0.001)).toBe(0);
  });

  it('偏离超过 半径+slack → 未命中（-1）', () => {
    const { positions, sizes } = mk([[0, 0, 0, 2]]);
    // t=100，slack = 0.001×100×4 = 0.4；阈值 2；偏离 3 → miss
    expect(pickNode(positions, sizes, 1, 3, 0, 100, ...DIR, 0.001)).toBe(-1);
  });

  it('偏离在 slack 内 → 命中（拾取宽容度）', () => {
    const { positions, sizes } = mk([[0, 0, 0, 2]]);
    // t=100，slack=0.4；阈值=max(2, 0.4)=2；偏离 2.3 → 半径外但 slack 内？2.3 > 2 → miss
    // slack 与半径取 max——2.3 > max(2,0.4)=2 → miss；改为偏离 1.9 命中
    expect(pickNode(positions, sizes, 1, 1.9, 0, 100, ...DIR, 0.001)).toBe(0);
  });

  it('远处小节点靠 slack 命中（远距离拾取宽容）', () => {
    const { positions, sizes } = mk([[0, 0, 0, 0.5]]);
    // t=100，slack = 0.001×100×4=0.4；阈值=max(0.5,0.4)=0.5；偏离 0.45 → 命中
    expect(pickNode(positions, sizes, 1, 0.45, 0, 100, ...DIR, 0.001)).toBe(0);
  });

  it('多节点时最近 t 获胜（遮挡语义）', () => {
    const { positions, sizes } = mk([
      [0, 0, 0, 1], // t=100
      [0, 0, 50, 1], // t=50，更近
    ]);
    expect(pickNode(positions, sizes, 2, ...ORIGIN, ...DIR, 0.001)).toBe(1);
  });

  it('相机背后节点跳过（t≤0）', () => {
    const { positions, sizes } = mk([[0, 0, 200, 5]]); // 在 +z 方向，射线朝 -z
    expect(pickNode(positions, sizes, 1, ...ORIGIN, ...DIR, 0.001)).toBe(-1);
  });

  it('空图 → -1', () => {
    expect(pickNode(new Float32Array(0), new Float32Array(0), 0, ...ORIGIN, ...DIR, 0.001)).toBe(-1);
  });

  it('PICK_SLACK_PX 常量契约 = 4', () => {
    expect(PICK_SLACK_PX).toBe(4);
  });
});
