/**
 * textureLayout：G5 渲染管线 v2 位置纹理布局（纯 TS，可单测）。
 *
 * 位置纹理 = RGBA32F DataTexture：texel(x,y,z,w=1) ↔ 节点 index = y·width + x。
 * 物理 tick 到达后主线程只做一次 memcpy（本模块）+ needsUpdate，
 * 节点 Points / 边 LineSegments 的顶点着色器统一 texelFetch 取位置——每 tick 零 CPU 几何计算。
 */

/** 近正方形纹理尺寸：width=ceil(√N)，height=ceil(N/width)；count=0 防御 1×1。 */
export function positionTextureDims(count: number): { width: number; height: number } {
  if (count <= 1) return { width: 1, height: 1 };
  const width = Math.ceil(Math.sqrt(count));
  return { width, height: Math.ceil(count / width) };
}

/** positions(xyz 紧凑) → RGBA 步长纹理数据；w 通道写 1；超出 count 的 texel 不动。 */
export function writePositionTexture(dst: Float32Array, positions: Float32Array, count: number): void {
  for (let i = 0; i < count; i++) {
    const s = i * 3;
    const d = i * 4;
    dst[d] = positions[s];
    dst[d + 1] = positions[s + 1];
    dst[d + 2] = positions[s + 2];
    dst[d + 3] = 1;
  }
}
