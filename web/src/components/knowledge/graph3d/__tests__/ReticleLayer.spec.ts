/**
 * ReticleLayer.spec：G5 渲染管线 v2 瞄准具层契约（hover 圆环 / 选中六边形）。
 */
import { describe, expect, it } from 'vitest';
import * as THREE from 'three';
import { RETICLE_HOVER_SCALE, RETICLE_SEL_SCALE, ReticleLayer } from '../render/ReticleLayer';

function uniforms(l: ReticleLayer): Record<string, { value: unknown }> {
  return (l.object.material as THREE.ShaderMaterial).uniforms;
}

describe('ReticleLayer', () => {
  it('缓冲结构：2 个 billboard 四边形（8 顶点 + aCorner/aKind）', () => {
    const l = new ReticleLayer();
    const geo = l.object.geometry;
    expect((geo.getAttribute('position') as THREE.BufferAttribute).count).toBe(8);
    expect((geo.getAttribute('aCorner') as THREE.BufferAttribute).count).toBe(8);
    const kind = (geo.getAttribute('aKind') as THREE.BufferAttribute).array as Float32Array;
    expect(Array.from(kind)).toEqual([0, 0, 0, 0, 1, 1, 1, 1]); // 前 4 hover 环，后 4 选中六边形
    expect(geo.getIndex()?.count).toBe(12);
    l.dispose();
  });

  it('初始全隐藏（index=-1，active=false）', () => {
    const l = new ReticleLayer();
    expect(uniforms(l).uHoverIndex.value).toBe(-1);
    expect(uniforms(l).uSelIndex.value).toBe(-1);
    expect(l.active).toBe(false);
    l.dispose();
  });

  it('setHover/setSelected 写 index 与放大半径；null 隐藏', () => {
    const l = new ReticleLayer();
    l.setHover(7, 2);
    expect(uniforms(l).uHoverIndex.value).toBe(7);
    expect(uniforms(l).uHoverSize.value).toBeCloseTo(2 * RETICLE_HOVER_SCALE, 5);
    expect(l.active).toBe(true);
    l.setSelected(9, 3);
    expect(uniforms(l).uSelIndex.value).toBe(9);
    expect(uniforms(l).uSelSize.value).toBeCloseTo(3 * RETICLE_SEL_SCALE, 5);
    l.setHover(null);
    expect(uniforms(l).uHoverIndex.value).toBe(-1);
    expect(l.active).toBe(true); // selected 仍在
    l.setSelected(null);
    expect(l.active).toBe(false);
    l.dispose();
  });

  it('setPositionTexture 绑定 uniforms；setTime 推进时钟', () => {
    const l = new ReticleLayer();
    const tex = new THREE.DataTexture(new Float32Array(8), 2, 1, THREE.RGBAFormat, THREE.FloatType);
    l.setPositionTexture(tex, 2);
    expect(uniforms(l).uPosTex.value).toBe(tex);
    expect(uniforms(l).uTexW.value).toBe(2);
    l.setTime(1.25);
    expect(uniforms(l).uTime.value).toBe(1.25);
    l.setPointScale(321); // 屏幕像素钳制换算系数（UX：近景瞄准具防满屏）
    expect(uniforms(l).uPointScale.value).toBe(321);
    tex.dispose();
    l.dispose();
  });
});
