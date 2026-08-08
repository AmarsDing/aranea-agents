/**
 * PositionTexture：G5 渲染管线 v2 位置纹理（GPU 管线核心）。
 *
 * 物理 tick → positions 一次 memcpy 进 RGBA32F DataTexture → needsUpdate；
 * 节点（Points）/边（LineSegments）顶点着色器 texelFetch 取位置。
 * 每 tick CPU 成本 = 一次 memcpy + 一次纹理上传，万级 ≈ 0.3ms + 160KB。
 */
import * as THREE from 'three';
import { positionTextureDims, writePositionTexture } from '../../../../features/knowledge/graph3d/textureLayout';

export class PositionTexture {
  readonly texture: THREE.DataTexture;
  readonly width: number;
  readonly height: number;
  private readonly data: Float32Array;
  private readonly count: number;

  constructor(count: number) {
    this.count = count;
    const { width, height } = positionTextureDims(count);
    this.width = width;
    this.height = height;
    this.data = new Float32Array(width * height * 4);
    this.texture = new THREE.DataTexture(this.data, width, height, THREE.RGBAFormat, THREE.FloatType);
    this.texture.minFilter = THREE.NearestFilter;
    this.texture.magFilter = THREE.NearestFilter;
    this.texture.generateMipmaps = false;
    this.texture.needsUpdate = true;
  }

  /** 物理 tick 回写：memcpy + 标记上传。 */
  update(positions: Float32Array): void {
    writePositionTexture(this.data, positions, this.count);
    this.texture.needsUpdate = true;
  }

  dispose(): void {
    this.texture.dispose();
  }
}
