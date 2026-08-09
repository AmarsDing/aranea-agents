/**
 * TwinSprite 光球核心（M74 设计 §7.4 v3，V7 复刻 TwinSprite SpriteOrb）。
 *
 * shader / 几何 / 配色均为 TwinSprite 原值：3D value noise 双层顶点置换 +
 * Fresnel 边缘光，Icosahedron(1.15, 48)，深蓝 #123a6e → 青 #4dd8e8，加法混合。
 *
 * 相对 TwinSprite 的唯一扩展：`uIntensity` 乘算最终颜色（加法混合下等效调光），
 * 用于本产品特有的待机微光 / 启动点亮过场（boot）；`uTime` 由部件按
 * `noiseSpeedFactor` 自累积（thinking 噪声高速流动），TwinSprite 为原始时间。
 */

import * as THREE from 'three';

import type { HudAudioFrame, HudPart } from './HudPart';
import type { HudParams } from '../hudParams';

const VERT = /* glsl */ `
uniform float uTime;
uniform float uAmp;
varying float vNoise;
varying vec3 vNormal;
varying vec3 vView;

// 简化 3D value noise（足够光球表面起伏）
vec3 hash3(vec3 p) {
  p = vec3(dot(p, vec3(127.1, 311.7, 74.7)),
           dot(p, vec3(269.5, 183.3, 246.1)),
           dot(p, vec3(113.5, 271.9, 124.6)));
  return -1.0 + 2.0 * fract(sin(p) * 43758.5453123);
}
float noise(vec3 p) {
  vec3 i = floor(p);
  vec3 f = fract(p);
  vec3 u = f * f * (3.0 - 2.0 * f);
  return mix(
    mix(mix(dot(hash3(i), f), dot(hash3(i + vec3(1,0,0)), f - vec3(1,0,0)), u.x),
        mix(dot(hash3(i + vec3(0,1,0)), f - vec3(0,1,0)), dot(hash3(i + vec3(1,1,0)), f - vec3(1,1,0)), u.x), u.y),
    mix(mix(dot(hash3(i + vec3(0,0,1)), f - vec3(0,0,1)), dot(hash3(i + vec3(1,0,1)), f - vec3(1,0,1)), u.x),
        mix(dot(hash3(i + vec3(0,1,1)), f - vec3(0,1,1)), dot(hash3(i + vec3(1,1,1)), f - vec3(1,1,1)), u.x), u.y),
    u.z);
}

void main() {
  float n = noise(normal * 2.2 + vec3(0.0, 0.0, uTime * 0.6));
  n += 0.5 * noise(normal * 5.0 + vec3(uTime * 1.2));
  vNoise = n;
  vec3 displaced = position + normal * n * uAmp;
  vec4 mv = modelViewMatrix * vec4(displaced, 1.0);
  vNormal = normalize(normalMatrix * normal);
  vView = normalize(-mv.xyz);
  gl_Position = projectionMatrix * mv;
}
`;

const FRAG = /* glsl */ `
uniform float uLevel;
uniform float uIntensity;
uniform vec3 uColorA;
uniform vec3 uColorB;
varying float vNoise;
varying vec3 vNormal;
varying vec3 vView;

void main() {
  float fresnel = pow(1.0 - abs(dot(vNormal, vView)), 2.0);
  float body = smoothstep(-0.4, 0.9, vNoise);
  vec3 col = mix(uColorA, uColorB, body);
  col += fresnel * (0.6 + uLevel * 1.4) * uColorB;
  col *= uIntensity;
  float alpha = 0.35 + body * 0.3 + fresnel * 0.5;
  gl_FragColor = vec4(col, alpha);
}
`;

export class SpriteOrbCore implements HudPart {
  private readonly mesh: THREE.Mesh;
  private readonly material: THREE.ShaderMaterial;
  /** 自累积噪声时间（×noiseSpeedFactor；thinking 高速流动）。 */
  private noiseTimeS = 0;

  constructor(parent: THREE.Scene) {
    const geometry = new THREE.IcosahedronGeometry(1.15, 48);
    this.material = new THREE.ShaderMaterial({
      vertexShader: VERT,
      fragmentShader: FRAG,
      transparent: true,
      blending: THREE.AdditiveBlending,
      depthWrite: false,
      uniforms: {
        uTime: { value: 0 },
        uAmp: { value: 0.12 },
        uLevel: { value: 0 },
        uIntensity: { value: 1 },
        uColorA: { value: new THREE.Color('#123a6e') },
        uColorB: { value: new THREE.Color('#4dd8e8') },
      },
    });
    this.mesh = new THREE.Mesh(geometry, this.material);
    parent.add(this.mesh);
  }

  update(dt: number, _timeS: number, params: HudParams, audio: HudAudioFrame): void {
    this.noiseTimeS += dt * params.noiseSpeedFactor;
    this.material.uniforms.uTime.value = this.noiseTimeS;
    this.material.uniforms.uLevel.value = audio.level;
    // TwinSprite 声波震动公式：呼吸 0.12 → 强震 0.5
    this.material.uniforms.uAmp.value = params.ampBase + audio.level * params.ampGain;
    this.material.uniforms.uIntensity.value = params.intensity;
    this.mesh.scale.setScalar(params.orbScale);
  }

  setTint(a: THREE.Color, b: THREE.Color): void {
    (this.material.uniforms.uColorA.value as THREE.Color).copy(a);
    (this.material.uniforms.uColorB.value as THREE.Color).copy(b);
  }

  dispose(): void {
    this.mesh.geometry.dispose();
    this.material.dispose();
    this.mesh.removeFromParent();
  }
}
