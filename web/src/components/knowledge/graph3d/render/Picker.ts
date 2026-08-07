/**
 * Picker：G5 逐实例拾取（fast-graph 移植，设计 §V12.8-1 C-6）。
 * Raycaster 对 InstancedMesh 求交，返回 instanceId；去抖在 Canvas 交互层。
 */
import * as THREE from 'three';

export class Picker {
  private readonly raycaster = new THREE.Raycaster();
  private readonly pointer = new THREE.Vector2();

  constructor(
    private readonly camera: THREE.Camera,
    private readonly mesh: THREE.InstancedMesh,
  ) {}

  /** NDC 坐标拾取节点索引；未命中返回 null。 */
  pick(ndcX: number, ndcY: number): number | null {
    this.pointer.set(ndcX, ndcY);
    this.raycaster.setFromCamera(this.pointer, this.camera);
    const hits = this.raycaster.intersectObject(this.mesh, false);
    for (const h of hits) {
      if (h.instanceId !== undefined && h.instanceId !== null) return h.instanceId;
    }
    return null;
  }
}
