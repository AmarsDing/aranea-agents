/**
 * 反应堆能量核（M74 设计 §7.4 v2）。
 *
 * Icosahedron + GLSL simplex 3D 顶点置换（液态有机形变）+ Fresnel 边缘光 +
 * 双色等离子混合。speaking 态由音频振幅驱动置换幅度。
 */

import * as THREE from 'three';

import type { HudAudioFrame, HudPart } from './HudPart';
import type { HudParams } from '../hudParams';

/** ashima/webgl-noise 3D simplex（公共领域实现，Three.js 社区标准做法）。 */
const SNOISE_3D = `
vec3 mod289(vec3 x){return x-floor(x*(1.0/289.0))*289.0;}
vec4 mod289(vec4 x){return x-floor(x*(1.0/289.0))*289.0;}
vec4 permute(vec4 x){return mod289(((x*34.0)+1.0)*x);}
vec4 taylorInvSqrt(vec4 r){return 1.79284291400159-0.85373472095314*r;}
float snoise(vec3 v){
  const vec2 C=vec2(1.0/6.0,1.0/3.0);
  const vec4 D=vec4(0.0,0.5,1.0,2.0);
  vec3 i=floor(v+dot(v,C.yyy));
  vec3 x0=v-i+dot(i,C.xxx);
  vec3 g=step(x0.yzx,x0.xyz);
  vec3 l=1.0-g;
  vec3 i1=min(g.xyz,l.zxy);
  vec3 i2=max(g.xyz,l.zxy);
  vec3 x1=x0-i1+C.xxx;
  vec3 x2=x0-i2+C.yyy;
  vec3 x3=x0-D.yyy;
  i=mod289(i);
  vec4 p=permute(permute(permute(i.z+vec4(0.0,i1.z,i2.z,1.0))+i.y+vec4(0.0,i1.y,i2.y,1.0))+i.x+vec4(0.0,i1.x,i2.x,1.0));
  float n_=0.142857142857;
  vec3 ns=n_*D.wyz-D.xzx;
  vec4 j=p-49.0*floor(p*ns.z*ns.z);
  vec4 x_=floor(j*ns.z);
  vec4 y_=floor(j-7.0*x_);
  vec4 x=x_*ns.x+ns.yyyy;
  vec4 y=y_*ns.x+ns.yyyy;
  vec4 h=1.0-abs(x)-abs(y);
  vec4 b0=vec4(x.xy,y.xy);
  vec4 b1=vec4(x.zw,y.zw);
  vec4 s0=floor(b0)*2.0+1.0;
  vec4 s1=floor(b1)*2.0+1.0;
  vec4 sh=-step(h,vec4(0.0));
  vec4 a0=b0.xzyw+s0.xzyw*sh.xxyy;
  vec4 a1=b1.xzyw+s1.xzyw*sh.zzww;
  vec3 p0=vec3(a0.xy,h.x);
  vec3 p1=vec3(a0.zw,h.y);
  vec3 p2=vec3(a1.xy,h.z);
  vec3 p3=vec3(a1.zw,h.w);
  vec4 norm=taylorInvSqrt(vec4(dot(p0,p0),dot(p1,p1),dot(p2,p2),dot(p3,p3)));
  p0*=norm.x;p1*=norm.y;p2*=norm.z;p3*=norm.w;
  vec4 m=max(0.6-vec4(dot(x0,x0),dot(x1,x1),dot(x2,x2),dot(x3,x3)),0.0);
  m=m*m;
  return 42.0*dot(m*m,vec4(dot(p0,x0),dot(p1,x1),dot(p2,x2),dot(p3,x3)));
}`;

const VERT = `
uniform float uTime;
uniform float uWobble;
uniform float uAudio;
varying vec3 vPos;
varying vec3 vNormalW;
varying vec3 vViewW;
${SNOISE_3D}
void main() {
  vPos = position;
  float slow = 0.6 * uTime;
  // 双层 simplex：低频大形变 + 高频细节，speaking 振幅放大置换
  float n1 = snoise(position * 1.6 + vec3(slow * 0.7, slow * 0.5, slow * 0.6));
  float n2 = snoise(position * 4.2 + vec3(-slow * 0.9, slow * 0.8, slow * 0.4));
  float amp = uWobble * (1.0 + uAudio * 1.8);
  vec3 displaced = position + normal * (n1 * amp + n2 * amp * 0.35);
  vec4 world = modelMatrix * vec4(displaced, 1.0);
  vNormalW = normalize(mat3(modelMatrix) * normal);
  vViewW = cameraPosition - world.xyz;
  gl_Position = projectionMatrix * viewMatrix * world;
}`;

const FRAG = `
uniform vec3 uColorA;
uniform vec3 uColorB;
uniform float uIntensity;
varying vec3 vPos;
varying vec3 vNormalW;
varying vec3 vViewW;
${SNOISE_3D}
void main() {
  vec3 n = normalize(vNormalW);
  vec3 v = normalize(vViewW);
  float fresnel = pow(1.0 - abs(dot(n, v)), 2.2);
  // 等离子双色混合：法线 y 分量 + 噪声扰动
  float plasma = 0.5 + 0.5 * n.y + 0.3 * snoise(vPos * 2.8);
  vec3 base = mix(uColorA, uColorB, clamp(plasma, 0.0, 1.0));
  float body = 0.45 + 0.4 * (0.5 + 0.5 * n.y);
  vec3 color = base * body * 0.5 + uColorA * fresnel * uIntensity;
  gl_FragColor = vec4(color, 0.92);
}`;

export class ReactorCore implements HudPart {
  private readonly mesh: THREE.Mesh;
  private readonly material: THREE.ShaderMaterial;

  constructor(parent: THREE.Scene) {
    const geometry = new THREE.IcosahedronGeometry(0.52, 32);
    this.material = new THREE.ShaderMaterial({
      vertexShader: VERT,
      fragmentShader: FRAG,
      uniforms: {
        uTime: { value: 0 },
        uWobble: { value: 0.05 },
        uAudio: { value: 0 },
        uColorA: { value: new THREE.Color('#22d3ee') },
        uColorB: { value: new THREE.Color('#0ea5e9') },
        uIntensity: { value: 1.4 },
      },
      transparent: true,
      blending: THREE.AdditiveBlending,
      depthWrite: false,
    });
    this.mesh = new THREE.Mesh(geometry, this.material);
    parent.add(this.mesh);
  }

  update(dt: number, timeS: number, params: HudParams, audio: HudAudioFrame): void {
    this.material.uniforms.uTime.value = timeS;
    this.material.uniforms.uWobble.value = params.coreWobble;
    this.material.uniforms.uAudio.value += (audio.level - this.material.uniforms.uAudio.value) * Math.min(1, dt * 20);
    this.mesh.scale.setScalar(params.coreScale);
    this.mesh.rotation.y += dt * 0.15;
    this.mesh.rotation.x = Math.sin(timeS * 0.22) * 0.08;
  }

  setTint(a: THREE.Color, b: THREE.Color): void {
    (this.material.uniforms.uColorA.value as THREE.Color).set(a);
    (this.material.uniforms.uColorB.value as THREE.Color).copy(b).multiplyScalar(0.55);
  }

  dispose(): void {
    this.mesh.geometry.dispose();
    this.material.dispose();
    this.mesh.removeFromParent();
  }
}
