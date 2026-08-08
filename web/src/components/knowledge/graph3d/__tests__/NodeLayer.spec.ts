/**
 * NodeLayer.spec：G5 渲染管线 v2 节点层契约（Points + 位置纹理，设计 §V12.8-1 C-1）。
 */
import { describe, expect, it } from 'vitest';
import * as THREE from 'three';
import { EMPH_DIM, EMPH_HI, EMPH_NORMAL, NodeLayer } from '../render/NodeLayer';

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

function attr(l: NodeLayer, name: string): Float32Array {
  return (l.points.geometry.getAttribute(name) as THREE.BufferAttribute).array as Float32Array;
}

describe('NodeLayer v2', () => {
  it('构造：Points 顶点计数 + 普通混合 + depthWrite/depthTest=false', () => {
    const l = new NodeLayer(8);
    expect(l.points.isPoints).toBe(true);
    expect(attr(l, 'position')).toHaveLength(8 * 3);
    expect(attr(l, 'aColor')).toHaveLength(8 * 3);
    expect(attr(l, 'aSize')).toHaveLength(8);
    expect(attr(l, 'aEmph')).toHaveLength(8);
    const mat = l.points.material as THREE.ShaderMaterial;
    expect(mat.transparent).toBe(true);
    expect(mat.depthWrite).toBe(false);
    expect(mat.blending).toBe(THREE.NormalBlending); // 弃加法混合：重叠不烧白
    l.dispose();
  });

  it('setColors 写入 aColor 静态属性', () => {
    const l = mkLayer();
    expect([attr(l, 'aColor')[0], attr(l, 'aColor')[1], attr(l, 'aColor')[2]]).toEqual([1, 0, 0]);
    l.dispose();
  });

  it('setSizes：(base + √degree·scale) × 倍率 → aSize 与 nodeSize/sizeData', () => {
    const l = mkLayer(3);
    l.setSizes(Uint16Array.from([0, 4, 16]), 2, 3, Float32Array.from([1, 1.5, 2.5]));
    expect(l.nodeSize(0)).toBeCloseTo(2, 5);
    expect(l.nodeSize(1)).toBeCloseTo((2 + 2 * 3) * 1.5, 5);
    expect(l.nodeSize(2)).toBeCloseTo((2 + 4 * 3) * 2.5, 5);
    expect(attr(l, 'aSize')[2]).toBeCloseTo((2 + 4 * 3) * 2.5, 5);
    expect(l.sizeData[1]).toBeCloseTo((2 + 2 * 3) * 1.5, 5);
    l.dispose();
  });

  it('高亮：集内 EMPH_HI、集外 EMPH_DIM（aEmph 动态属性）', () => {
    const l = mkLayer(3);
    l.setHighlight(new Set([1]));
    const emph = attr(l, 'aEmph');
    expect(emph[1]).toBeCloseTo(EMPH_HI, 6);
    expect(emph[0]).toBeCloseTo(EMPH_DIM, 6);
    expect(emph[2]).toBeCloseTo(EMPH_DIM, 6);
    expect(EMPH_HI).toBeGreaterThan(EMPH_NORMAL); // 高亮才冒辉光
    expect(EMPH_DIM).toBeLessThan(EMPH_NORMAL);
    l.dispose();
  });

  it('setHighlight(null)/空集 全恢复 EMPH_NORMAL', () => {
    const l = mkLayer(2);
    l.setHighlight(new Set([0]));
    l.setHighlight(null);
    expect(Array.from(attr(l, 'aEmph'))).toEqual([EMPH_NORMAL, EMPH_NORMAL]);
    expect(l.highlightedSet).toBeNull();
    l.setHighlight(new Set([0]));
    l.setHighlight(new Set());
    expect(Array.from(attr(l, 'aEmph'))).toEqual([EMPH_NORMAL, EMPH_NORMAL]);
    l.dispose();
  });

  it('setPositionTexture 绑定 uPosTex/uTexW uniforms；setPointScale 写 uPointScale', () => {
    const l = new NodeLayer(2);
    const tex = new THREE.DataTexture(new Float32Array(8), 2, 1, THREE.RGBAFormat, THREE.FloatType);
    l.setPositionTexture(tex, 2);
    const mat = l.points.material as THREE.ShaderMaterial;
    expect(mat.uniforms.uPosTex.value).toBe(tex);
    expect(mat.uniforms.uTexW.value).toBe(2);
    l.setPointScale(321);
    expect(mat.uniforms.uPointScale.value).toBe(321);
    tex.dispose();
    l.dispose();
  });
});
