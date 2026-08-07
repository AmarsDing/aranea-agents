/**
 * BackdropLayer：G5 深空背景三层（orrery 蓝本，设计 §V12.8-1 C-3）。
 *
 * - FBM 星云反转球：3-octave value-noise，colA 紫(0.12,0.06,0.22)/colB 青(0.05,0.17,0.21)，
 *   bright=0.5，pow(fbm,2.2) 压在 bloom 阈值下（防糊屏）
 * - 三档星空（jarvis 参数）：dim 2400/med 4800/bright 800，球面确定性散布（黄金角+固定种子），
 *   sizeAttenuation:false（像素尺寸）+ 64px 柔光 dotTexture
 * - 核雾：520 颗加法 Points，布局收敛后锚定度数最大 hub（setHazeAnchor）
 */
import * as THREE from 'three';
import { mulberry32 } from '../../../../features/knowledge/graph3d/model';

let cachedDotTex: THREE.Texture | null | undefined;

/** 64px 径向渐变纹理工厂（stops 参数化）；无 2D 上下文环境（测试）返回 null。 */
export function makeRadialTexture(stops: [number, string][]): THREE.Texture | null {
  try {
    const size = 64;
    const c = globalThis.document.createElement('canvas');
    c.width = c.height = size;
    const ctx = c.getContext('2d');
    if (!ctx) return null;
    const g = ctx.createRadialGradient(size / 2, size / 2, 0, size / 2, size / 2, size / 2);
    for (const [off, color] of stops) g.addColorStop(off, color);
    ctx.fillStyle = g;
    ctx.fillRect(0, 0, size, size);
    const tex = new THREE.CanvasTexture(c);
    tex.colorSpace = THREE.SRGBColorSpace;
    return tex;
  } catch {
    return null;
  }
}

/** 星空/核雾柔光点纹理（缓存）。 */
export function dotTexture(): THREE.Texture | null {
  if (cachedDotTex !== undefined) return cachedDotTex;
  cachedDotTex = makeRadialTexture([
    [0, 'rgba(255,255,255,1)'],
    [0.25, 'rgba(255,255,255,0.5)'],
    [1, 'rgba(255,255,255,0)'],
  ]);
  return cachedDotTex;
}

/** 星空档位（数量/像素尺寸/不透明度/颜色）。 */
export const STAR_TIERS = [
  { count: 2400, size: 1.2, opacity: 0.35, color: 0x6b7fb8, seed: 101 },
  { count: 4800, size: 1.8, opacity: 0.6, color: 0x9aa6ff, seed: 202 },
  { count: 800, size: 3.0, opacity: 0.95, color: 0xd6e0ff, seed: 303 },
] as const;

export const STAR_SPREAD = 4200;
export const HAZE_COUNT = 520;
export const HAZE_SPREAD = 1050;

/** 确定性球面壳层散布（mulberry32 固定种子，帧间稳定）。 */
export function scatterSphere(count: number, radius: number, seed: number): Float32Array {
  const rand = mulberry32(seed);
  const out = new Float32Array(count * 3);
  for (let i = 0; i < count; i++) {
    const u = rand() * 2 - 1;
    const theta = rand() * Math.PI * 2;
    const r = radius * (0.8 + rand() * 0.4); // 壳层 ±20%
    const s = Math.sqrt(1 - u * u);
    out[i * 3] = r * s * Math.cos(theta);
    out[i * 3 + 1] = r * s * Math.sin(theta);
    out[i * 3 + 2] = r * u;
  }
  return out;
}

const NEBULA_VERTEX = `
  varying vec3 vDir;
  void main() {
    vDir = normalize(position);
    gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
  }`;

const NEBULA_FRAGMENT = `
  varying vec3 vDir;
  uniform vec3 colA; uniform vec3 colB; uniform float bright;
  float hash(vec3 p){ p = fract(p * 0.3183099 + 0.1); p *= 17.0; return fract(p.x*p.y*p.z*(p.x+p.y+p.z)); }
  float noise(vec3 x){
    vec3 i = floor(x); vec3 f = fract(x); f = f*f*(3.0-2.0*f);
    return mix(mix(mix(hash(i+vec3(0,0,0)),hash(i+vec3(1,0,0)),f.x),
                   mix(hash(i+vec3(0,1,0)),hash(i+vec3(1,1,0)),f.x),f.y),
               mix(mix(hash(i+vec3(0,0,1)),hash(i+vec3(1,0,1)),f.x),
                   mix(hash(i+vec3(0,1,1)),hash(i+vec3(1,1,1)),f.x),f.y),f.z);
  }
  float fbm(vec3 p){ float v=0.0,a=0.5; for(int i=0;i<3;i++){ v+=a*noise(p); p*=2.0; a*=0.5; } return v; }
  void main(){
    vec3 d = normalize(vDir);
    float n = pow(fbm(d * 3.0), 2.2);
    vec3 col = mix(colA, colB, fbm(d * 1.5 + 5.0));
    gl_FragColor = vec4(col * n * bright, 1.0);
  }`;

export class BackdropLayer {
  readonly group = new THREE.Group();
  readonly nebula: THREE.Mesh;
  readonly stars: THREE.Points[];
  readonly haze: THREE.Points;
  private readonly disposables: { dispose(): void }[] = [];

  constructor() {
    // FBM 星云反转球
    const nebulaMat = new THREE.ShaderMaterial({
      uniforms: {
        colA: { value: new THREE.Color(0.12, 0.06, 0.22) },
        colB: { value: new THREE.Color(0.05, 0.17, 0.21) },
        bright: { value: 0.5 },
      },
      vertexShader: NEBULA_VERTEX,
      fragmentShader: NEBULA_FRAGMENT,
      side: THREE.BackSide,
      depthWrite: false,
      depthTest: false,
    });
    const nebulaGeo = new THREE.SphereGeometry(5000, 48, 48);
    this.nebula = new THREE.Mesh(nebulaGeo, nebulaMat);
    this.nebula.renderOrder = -1;
    this.group.add(this.nebula);
    this.disposables.push(nebulaGeo, nebulaMat);

    // 三档星空
    const tex = dotTexture();
    this.stars = STAR_TIERS.map((tier) => {
      const geo = new THREE.BufferGeometry();
      geo.setAttribute('position', new THREE.BufferAttribute(scatterSphere(tier.count, STAR_SPREAD, tier.seed), 3));
      const mat = new THREE.PointsMaterial({
        color: tier.color,
        map: tex,
        size: tier.size,
        sizeAttenuation: false,
        transparent: true,
        opacity: tier.opacity,
        depthWrite: false,
      });
      const points = new THREE.Points(geo, mat);
      points.frustumCulled = false;
      this.group.add(points);
      this.disposables.push(geo, mat);
      return points;
    });

    // 核雾（初始在原点，收敛后锚定 hub）
    const hazeGeo = new THREE.BufferGeometry();
    const hazePos = new Float32Array(HAZE_COUNT * 3);
    const rand = mulberry32(404);
    for (let i = 0; i < HAZE_COUNT; i++) {
      const a = i * 2.39996; // 黄金角
      const t = rand();
      const rad = HAZE_SPREAD * (0.18 + 0.82 * Math.pow(t, 1.5));
      const phi = Math.acos(2 * rand() - 1);
      hazePos[i * 3] = rad * Math.sin(phi) * Math.cos(a);
      hazePos[i * 3 + 1] = rad * Math.sin(phi) * Math.sin(a);
      hazePos[i * 3 + 2] = rad * Math.cos(phi);
    }
    hazeGeo.setAttribute('position', new THREE.BufferAttribute(hazePos, 3));
    const hazeMat = new THREE.PointsMaterial({
      color: 0xb39dff,
      map: tex,
      size: 14,
      sizeAttenuation: true,
      transparent: true,
      opacity: 0.2,
      blending: THREE.AdditiveBlending,
      depthWrite: false,
    });
    this.haze = new THREE.Points(hazeGeo, hazeMat);
    this.haze.frustumCulled = false;
    this.group.add(this.haze);
    this.disposables.push(hazeGeo, hazeMat);
  }

  /** 核雾锚定（布局收敛后锚定度数最大 hub）。 */
  setHazeAnchor(x: number, y: number, z: number): void {
    this.haze.position.set(x, y, z);
  }

  dispose(): void {
    for (const d of this.disposables) d.dispose();
  }
}
