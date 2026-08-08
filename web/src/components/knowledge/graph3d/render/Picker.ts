/**
 * Picker：G5 渲染管线 v2 拾取（射线-球 O(N) 纯循环，设计 §V12.8-1 C-6）。
 *
 * v1 用 Raycaster 对 InstancedMesh 求交（每实例一次矩阵求逆 = 万级 hover 卡顿元凶）；
 * v2 仅借 Raycaster 求射线原点/方向，命中判定走 pickMath.pickNode（无矩阵运算）。
 */
import * as THREE from 'three';
import { pickNode } from '../../../../features/knowledge/graph3d/pickMath';

export class Picker {
  private readonly raycaster = new THREE.Raycaster();
  private readonly pointer = new THREE.Vector2();

  constructor(private readonly camera: THREE.PerspectiveCamera) {}

  /**
   * NDC 坐标拾取节点索引；未命中返回 null。
   *
   * @param positions 物理位置缓冲（3N，engine.positions 直读）
   * @param sizes     节点半径缓冲（N，NodeLayer.sizeData）
   * @param viewportHeightPx 视口 CSS 像素高度（slack 按 CSS 像素计）
   */
  pick(
    ndcX: number,
    ndcY: number,
    positions: Float32Array,
    sizes: Float32Array,
    count: number,
    viewportHeightPx: number,
  ): number | null {
    this.pointer.set(ndcX, ndcY);
    this.raycaster.setFromCamera(this.pointer, this.camera);
    const o = this.raycaster.ray.origin;
    const d = this.raycaster.ray.direction;
    const worldPerPixel = (2 * Math.tan((this.camera.fov * Math.PI) / 360)) / Math.max(viewportHeightPx, 1);
    const idx = pickNode(positions, sizes, count, o.x, o.y, o.z, d.x, d.y, d.z, worldPerPixel);
    return idx < 0 ? null : idx;
  }
}
