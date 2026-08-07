/**
 * NodeLayer.spec：G5-C 节点层契约（设计 §V12.8-1 C-1）。
 */
import { describe, expect, it } from 'vitest';
import * as THREE from 'three';
import { NodeLayer } from '../render/NodeLayer';

function mkLayer(count = 4): NodeLayer {
  const l = new NodeLayer(count);
  const colors = new Float32Array(count * 3);
  for (let i = 0; i < count; i++) {
    colors[i * 3] = 1;
    colors[i * 3 + 1] = 0;
    colors[i * 3 + 2] = 0; // 红
  }
  l.setColors(colors);
  return l;
}

function instanceColor(l: NodeLayer, i: number): [number, number, number] {
  const arr = l.mesh.instanceColor!.array as Float32Array;
  return [arr[i * 3], arr[i * 3 + 1], arr[i * 3 + 2]];
}

describe('NodeLayer', () => {
  it('构造：InstancedMesh 计数 + 加法混合 + depthWrite=false', () => {
    const l = mkLayer(8);
    expect(l.mesh.count).toBe(8);
    const mat = l.mesh.material as { blending: number; depthWrite: boolean; transparent: boolean };
    expect(mat.transparent).toBe(true);
    expect(mat.depthWrite).toBe(false);
    l.dispose();
  });

  it('setColors 写入 instanceColor', () => {
    const l = mkLayer();
    expect(instanceColor(l, 0)).toEqual([1, 0, 0]);
    l.dispose();
  });

  it('setSizes：(base + √degree·scale) × 倍率', () => {
    const l = mkLayer(3);
    l.setSizes(Uint16Array.from([0, 4, 16]), 2, 3, Float32Array.from([1, 1.5, 2.5]));
    expect(l.nodeSize(0)).toBeCloseTo(2, 5);
    expect(l.nodeSize(1)).toBeCloseTo((2 + 2 * 3) * 1.5, 5);
    expect(l.nodeSize(2)).toBeCloseTo((2 + 4 * 3) * 2.5, 5);
    l.dispose();
  });

  it('高亮：集内 lerp(white,0.5)，集外压暗（保留 8% 原色）', () => {
    const l = mkLayer(3);
    l.setHighlight(new Set([1]));
    const hi = instanceColor(l, 1);
    expect(hi[0]).toBeCloseTo(1, 5); // 红 1 → lerp 白后仍 1
    expect(hi[1]).toBeCloseTo(0.5, 5); // 绿 0 → 0.5
    const dim = instanceColor(l, 0);
    // three 色彩管理下 '#050810' 经 sRGB→linear；期望 = bg(linear) + base·0.08
    const bg = new THREE.Color('#050810');
    expect(dim[0]).toBeCloseTo(bg.r + 1 * 0.08, 4);
    expect(dim[0]).toBeLessThan(0.2);
    l.dispose();
  });

  it('setHighlight(null) 全恢复 base', () => {
    const l = mkLayer(2);
    l.setHighlight(new Set([0]));
    l.setHighlight(null);
    expect(instanceColor(l, 0)).toEqual([1, 0, 0]);
    expect(instanceColor(l, 1)).toEqual([1, 0, 0]);
    expect(l.highlightedSet).toBeNull();
    l.dispose();
  });

  it('updatePositions 写入 instanceMatrix 平移与缩放', () => {
    const l = mkLayer(2);
    l.setSizes(Uint16Array.from([0, 0]), 2, 0);
    const pos = new Float32Array([10, 20, 30, -5, -6, -7]);
    l.updatePositions(pos);
    const m = new THREE.Matrix4();
    l.mesh.getMatrixAt(0, m);
    const e = m.elements;
    // 列主序：平移在 elements[12..14]，缩放在对角 [0],[5],[10]
    expect(e[12]).toBe(10);
    expect(e[13]).toBe(20);
    expect(e[14]).toBe(30);
    expect(e[0]).toBeCloseTo(2, 5);
    l.dispose();
  });
});
