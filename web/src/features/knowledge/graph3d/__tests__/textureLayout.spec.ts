/**
 * textureLayout.spec：G5 渲染管线 v2 位置纹理布局契约。
 *
 * 位置纹理 = RGBA32F DataTexture，texel (x,y,z,unused) 对应节点 index：
 * width=ceil(√N)、height=ceil(N/width)；GPU 顶点着色器 texelFetch 取数（零 CPU/tick）。
 */
import { describe, expect, it } from 'vitest';
import { positionTextureDims, writePositionTexture } from '../textureLayout';

describe('positionTextureDims', () => {
  it('count=0/1 → 1×1（防御空图）', () => {
    expect(positionTextureDims(0)).toEqual({ width: 1, height: 1 });
    expect(positionTextureDims(1)).toEqual({ width: 1, height: 1 });
  });

  it('小图：count=2 → 2×1；count=5 → 3×2', () => {
    expect(positionTextureDims(2)).toEqual({ width: 2, height: 1 });
    expect(positionTextureDims(5)).toEqual({ width: 3, height: 2 });
  });

  it('万级：count=10000 → 100×100；count=10001 容量充足', () => {
    expect(positionTextureDims(10000)).toEqual({ width: 100, height: 100 });
    const d = positionTextureDims(10001);
    expect(d.width * d.height).toBeGreaterThanOrEqual(10001);
    expect(d.width).toBe(101);
    expect(d.height).toBe(100);
  });

  it('十万级：count=100000 容量充足且近正方形', () => {
    const d = positionTextureDims(100000);
    expect(d.width * d.height).toBeGreaterThanOrEqual(100000);
    expect(Math.abs(d.width - d.height)).toBeLessThanOrEqual(1);
  });
});

describe('writePositionTexture', () => {
  it('RGB 通道按 RGBA 步长写入 xyz，w 通道写 1', () => {
    const positions = Float32Array.from([1, 2, 3, 4.5, -5.5, 6]);
    const dst = new Float32Array(2 * 4);
    writePositionTexture(dst, positions, 2);
    expect(Array.from(dst)).toEqual([1, 2, 3, 1, 4.5, -5.5, 6, 1]);
  });

  it('超出 count 的 texel 保持原值（不被污染）', () => {
    const positions = Float32Array.from([7, 8, 9]);
    const dst = new Float32Array(4 * 4).fill(-1);
    writePositionTexture(dst, positions, 1);
    expect(dst[0]).toBe(7);
    expect(dst[3]).toBe(1);
    expect(dst[4]).toBe(-1); // 未覆盖 texel 不变
  });
});
