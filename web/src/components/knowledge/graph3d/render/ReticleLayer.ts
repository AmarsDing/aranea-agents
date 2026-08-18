/**
 * ReticleLayer：G5 渲染管线 v2 瞄准具层（hover 圆环 / 选中六边形，科幻 HUD）。
 *
 * - 2 个 billboard 四边形合一 Mesh：顶点着色器按 uniform index texelFetch 位置纹理锚定节点，
 *   视空间偏移 aCorner·size 实现恒面向相机（无 CPU 每帧变换）
 * - 片元程序化绘制：hover=软边圆环 + 呼吸脉冲；选中=缓旋六边形描边 + 角点亮度流（SDF）
 * - 纯呈现层：index/半径由 Canvas 注入；uTime 仅活跃期推进（lazy-render 友好）
 * - 配色对齐 UX token：hover 霓虹青 #54E6FF、选中霓虹紫 #A855F7（夜间二级强调）
 */
import * as THREE from 'three';

/** 环/六边形相对节点半径的放大倍数（四边形半尺寸 = 节点半径 × 倍率）。
 *  UX 优化：2.1/2.6 → 1.5/1.8（环贴合节点轮廓，原值在 hub 节点上大得夸张、喧宾夺主）。 */
export const RETICLE_HOVER_SCALE = 1.5;
export const RETICLE_SEL_SCALE = 1.8;

const RETICLE_VERTEX = `
  uniform sampler2D uPosTex;
  uniform float uTexW;
  uniform int uHoverIndex;
  uniform int uSelIndex;
  uniform float uHoverSize;
  uniform float uSelSize;
  uniform float uPointScale;
  attribute vec2 aCorner;
  attribute float aKind;
  varying vec2 vUv;
  varying float vKind;
  varying float vOn;
  void main() {
    bool isHover = aKind < 0.5;
    int idx = isHover ? uHoverIndex : uSelIndex;
    float size = isHover ? uHoverSize : uSelSize;
    vOn = idx >= 0 ? 1.0 : 0.0;
    int safe = max(idx, 0);
    vec3 wp = texelFetch(uPosTex, ivec2(safe % int(uTexW), safe / int(uTexW)), 0).xyz;
    vec4 mv = modelViewMatrix * vec4(wp, 1.0);
    // UX 优化：瞄准具屏幕半径钳制（节点 gl_PointSize 有 40px 上限，世界空间尺寸的
    // 瞄准具在近景会放大到满屏）；上限取节点屏幕半径 × 环倍率量级
    float maxPx = isHover ? 30.0 : 40.0;
    float worldPerPx = max(-mv.z, 1.0) / uPointScale;
    size = min(size, maxPx * worldPerPx);
    mv.xy += aCorner * size; // 视空间 billboard
    gl_Position = projectionMatrix * mv;
    vUv = aCorner;
    vKind = aKind;
  }`;

const RETICLE_FRAGMENT = `
  uniform float uTime;
  uniform vec3 uHoverColor;
  uniform vec3 uSelColor;
  varying vec2 vUv;
  varying float vKind;
  varying float vOn;
  float hexDist(vec2 p) {
    p = abs(p);
    return max(dot(p, normalize(vec2(1.0, 1.7320508))), p.x);
  }
  void main() {
    if (vOn < 0.5) discard;
    vec2 uv = vUv;
    float alpha;
    vec3 col;
    if (vKind < 0.5) {
      // hover 圆环：软边 + 呼吸脉冲
      float ring = 1.0 - smoothstep(0.045, 0.11, abs(length(uv) - 0.8));
      float breathe = 0.72 + 0.28 * sin(uTime * 4.2);
      alpha = ring * 0.85 * breathe;
      col = uHoverColor;
    } else {
      // 选中六边形：缓旋描边 + 六角亮度流
      float rot = uTime * 0.45;
      float c = cos(rot);
      float s = sin(rot);
      uv = mat2(c, -s, s, c) * uv;
      float line = 1.0 - smoothstep(0.03, 0.075, abs(hexDist(uv) - 0.74));
      float sparkle = 0.68 + 0.32 * sin(atan(uv.y, uv.x) * 6.0 - uTime * 2.4);
      alpha = line * sparkle * 0.95;
      col = uSelColor;
    }
    if (alpha < 0.01) discard;
    gl_FragColor = vec4(col, alpha);
    #include <tonemapping_fragment>
    #include <colorspace_fragment>
  }`;

export class ReticleLayer {
  readonly object: THREE.Mesh;
  private readonly geometry: THREE.BufferGeometry;
  private readonly material: THREE.ShaderMaterial;

  constructor() {
    this.geometry = new THREE.BufferGeometry();
    // position 仅作顶点计数驱动；真实锚点走 uPosTex
    this.geometry.setAttribute('position', new THREE.BufferAttribute(new Float32Array(8 * 3), 3));
    this.geometry.setAttribute(
      'aCorner',
      new THREE.BufferAttribute(new Float32Array([-1, -1, 1, -1, 1, 1, -1, 1, -1, -1, 1, -1, 1, 1, -1, 1]), 2),
    );
    this.geometry.setAttribute('aKind', new THREE.BufferAttribute(new Float32Array([0, 0, 0, 0, 1, 1, 1, 1]), 1));
    this.geometry.setIndex([0, 1, 2, 0, 2, 3, 4, 5, 6, 4, 6, 7]);

    this.material = new THREE.ShaderMaterial({
      uniforms: {
        uPosTex: { value: null },
        uTexW: { value: 1 },
        uHoverIndex: { value: -1 },
        uSelIndex: { value: -1 },
        uHoverSize: { value: 1 },
        uSelSize: { value: 1 },
        uTime: { value: 0 },
        uHoverColor: { value: new THREE.Color(0x54e6ff) },
        uSelColor: { value: new THREE.Color(0xa855f7) },
        // 屏幕像素钳制换算系数（同 NodeLayer：drawingBufferHeight·0.5/tan(fov/2)），Canvas 注入
        uPointScale: { value: 540 },
      },
      vertexShader: RETICLE_VERTEX,
      fragmentShader: RETICLE_FRAGMENT,
      transparent: true,
      depthWrite: false,
      depthTest: false,
      blending: THREE.NormalBlending,
    });
    this.object = new THREE.Mesh(this.geometry, this.material);
    this.object.frustumCulled = false;
    this.object.renderOrder = 2;
  }

  /** 绑定位置纹理（Canvas 持有 PositionTexture）。 */
  setPositionTexture(texture: THREE.DataTexture, width: number): void {
    this.material.uniforms.uPosTex.value = texture;
    this.material.uniforms.uTexW.value = width;
  }

  /** hover 圆环：index=null 隐藏；nodeRadius 为节点世界半径。 */
  setHover(index: number | null, nodeRadius = 0): void {
    this.material.uniforms.uHoverIndex.value = index ?? -1;
    this.material.uniforms.uHoverSize.value = nodeRadius * RETICLE_HOVER_SCALE;
  }

  /** 选中六边形：index=null 隐藏；nodeRadius 为节点世界半径。 */
  setSelected(index: number | null, nodeRadius = 0): void {
    this.material.uniforms.uSelIndex.value = index ?? -1;
    this.material.uniforms.uSelSize.value = nodeRadius * RETICLE_SEL_SCALE;
  }

  /** 呼吸/旋转时钟（仅活跃期推进）。 */
  setTime(t: number): void {
    this.material.uniforms.uTime.value = t;
  }

  /** 像素缩放系数（Canvas 在创建/画质档/resize 时注入，与 NodeLayer 同源）。 */
  setPointScale(scale: number): void {
    this.material.uniforms.uPointScale.value = scale;
  }

  /** 任一瞄准具可见（lazy-render 保持渲染循环的判定条件之一）。 */
  get active(): boolean {
    return this.material.uniforms.uHoverIndex.value >= 0 || this.material.uniforms.uSelIndex.value >= 0;
  }

  dispose(): void {
    this.geometry.dispose();
    this.material.dispose();
  }
}
