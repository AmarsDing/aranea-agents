/**
 * EdgeLayer.spec：G5 渲染管线 v2 边层契约（直线 LineSegments + 位置纹理，设计 §V12.8-1 C-2）。
 */
import { describe, expect, it } from 'vitest';
import * as THREE from 'three';
import { EDGE_HOVER_ALPHA, EDGE_REST_ALPHA, EdgeLayer } from '../render/EdgeLayer';

function mkLayer(): EdgeLayer {
  // 边 0：0→1；边 1：1→2
  const edges = Int32Array.from([0, 1, 1, 2]);
  const colors = new Float32Array([1, 0, 0, 0, 1, 0]);
  return new EdgeLayer(edges, colors);
}

function attr(l: EdgeLayer, name: string): Float32Array {
  return (l.object.geometry.getAttribute(name) as THREE.BufferAttribute).array as Float32Array;
}

describe('EdgeLayer v2', () => {
  it('缓冲结构：每边 2 顶点；aNode=端点索引、aT=0/1', () => {
    const l = mkLayer();
    expect(attr(l, 'position')).toHaveLength(2 * 2 * 3);
    expect(Array.from(attr(l, 'aNode'))).toEqual([0, 1, 1, 2]);
    expect(Array.from(attr(l, 'aT'))).toEqual([0, 1, 0, 1]);
    l.dispose();
  });

  it('aColor 双端同色（边类型色）', () => {
    const l = mkLayer();
    const c = attr(l, 'aColor');
    expect([c[0], c[1], c[2]]).toEqual([1, 0, 0]); // 边 0 源端红
    expect([c[3], c[4], c[5]]).toEqual([1, 0, 0]); // 边 0 宿端红
    expect([c[6], c[7], c[8]]).toEqual([0, 1, 0]); // 边 1 源端绿
    l.dispose();
  });

  it('材质：普通混合 + rest 低透明度契约（降亮度）', () => {
    const l = mkLayer();
    const mat = l.object.material as THREE.ShaderMaterial;
    expect(mat.blending).toBe(THREE.NormalBlending);
    expect(mat.uniforms.uRestAlpha.value).toBe(EDGE_REST_ALPHA);
    expect(mat.uniforms.uHoverAlpha.value).toBe(EDGE_HOVER_ALPHA);
    expect(EDGE_REST_ALPHA).toBeLessThan(0.2);
    l.dispose();
  });

  it('setHighlight：关联边 aHi=1，其余 0；null 全 0', () => {
    const l = mkLayer();
    l.setHighlight(new Set([1]));
    expect(Array.from(attr(l, 'aHi'))).toEqual([0, 0, 1, 1]);
    expect(l.highlightedEdges).toEqual(new Set([1]));
    l.setHighlight(null);
    expect(Array.from(attr(l, 'aHi'))).toEqual([0, 0, 0, 0]);
    expect(l.highlightedEdges).toBeNull();
    l.dispose();
  });

  it('setPositionTexture 绑定 uniforms；setTime 推进流动脉冲时钟', () => {
    const l = mkLayer();
    const tex = new THREE.DataTexture(new Float32Array(16), 2, 2, THREE.RGBAFormat, THREE.FloatType);
    l.setPositionTexture(tex, 2);
    const mat = l.object.material as THREE.ShaderMaterial;
    expect(mat.uniforms.uPosTex.value).toBe(tex);
    expect(mat.uniforms.uTexW.value).toBe(2);
    l.setTime(3.5);
    expect(mat.uniforms.uTime.value).toBe(3.5);
    tex.dispose();
    l.dispose();
  });
});
