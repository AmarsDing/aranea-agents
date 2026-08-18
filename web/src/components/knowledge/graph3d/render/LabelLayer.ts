/**
 * LabelLayer：G5 节点标签层（three-spritetext，设计 §V12.8-1 C-5）。
 *
 * - 候选池：按 degree 取 top-K（默认 80，与 HIGH 档一致），Sprite 常驻复用
 * - 双阈值可见性：相机距离 ≤ maxDistance 且 (degree ≥ minDegree 或 extraVisible 包含)
 * - 标签开关关时只显示 extraVisible（hover/选中节点）
 * - 焦点标签（UX 优化）：hover/选中节点独立 sprite——粗体亮字 + 底牌描边 +
 *   屏幕恒尺寸；非候选池节点也能出名称，候选池内同名节点隐藏防重影
 * - 屏幕恒尺寸（像素目标）：候选/焦点标签按 2·dist·tanHalf·targetPx/(viewportPx·base)
 *   换算世界缩放，任何相机距离下屏幕字高恒定；NDC 矩形按像素目标精确换算供防重叠
 * - 位置：节点上方 y + nodeSize + 偏移
 */
import * as THREE from 'three';
import SpriteText from 'three-spritetext';

export interface LabelLayerOpts {
  names: string[];
  degree: Uint16Array;
  /** 候选池上限（按 degree 降序取 top-K）。 */
  maxLabels?: number;
  /** 标签文字颜色。 */
  color?: string;
  /** 标签字高（世界单位）。 */
  textHeight?: number;
}

export interface LabelVisibility {
  maxDistance: number;
  minDegree: number;
  /** 强制可见集（hover/选中）。 */
  extraVisible?: Set<number>;
  /** 节点半径查询（标签放在节点上方）。 */
  nodeSize?: (i: number) => number;
  /** 同屏候选标签上限（屏幕空间防重叠后再截断，默认 12）。 */
  maxVisible?: number;
  /** 画布 CSS 像素高（像素目标恒尺寸换算；缺省 800）。 */
  viewportPx?: number;
}

/** NDC 矩形（屏幕空间 -1..1 域，UX 防重叠用）。 */
export interface NdcRect {
  x0: number;
  y0: number;
  x1: number;
  y1: number;
}

/** 屏幕空间防重叠（纯函数）：按输入顺序（候选池=度数降序）贪婪保留，
 *  与已保留矩形相交或超上限的丢弃。UX 根治：相机深入核心时 20 个恒尺寸
 *  候选标签不再叠成 soup——高度数优先，重叠即隐藏（悬停/选中走焦点标签通道）。 */
export function filterOverlappingLabels(
  rects: readonly (NdcRect | null)[],
  maxVisible: number,
  margin = 0.015,
): boolean[] {
  const kept: NdcRect[] = [];
  return rects.map((r) => {
    if (!r || kept.length >= maxVisible) return false;
    const m = { x0: r.x0 - margin, y0: r.y0 - margin, x1: r.x1 + margin, y1: r.y1 + margin };
    if (kept.some((k) => !(m.x1 < k.x0 || m.x0 > k.x1 || m.y1 < k.y0 || m.y0 > k.y1))) return false;
    kept.push(m);
    return true;
  });
}

const LABEL_Y_OFFSET = 2;
/** 焦点标签抬高系数（随缩放，避让节点光晕与普通标签）。 */
const FOCUS_LABEL_Y_OFFSET = 2.5;
/** 标签锚定半径封顶（世界单位）：超 hub r 可达 15，不封顶时近景标签飘离节点数百 px。 */
const LABEL_ANCHOR_R_CAP = 3;
/** 屏幕像素目标（UX 真恒尺寸）：任何相机距离下标签在屏幕上的字高恒定。
 *  候选 12.5px 不抢视觉；焦点 16px 粗体底牌一眼可读。 */
const CANDIDATE_LABEL_PX = 12.5;
const FOCUS_LABEL_PX = 16;
/** 世界缩放系数钳制：仅防极端深度下世界尺寸病态（近景世界尺寸随 dist→0 自然缩，不会爆炸）。
 *  min 取 0.002：dist<0.38（已穿入节点内部）才触钳，实机路径不可达；
 *  旧「距离/参考距离+min 0.4 钳制」方案在 min 钳住后近景字高 ∝ 1/dist 爆炸（实测 60~90px），已废弃。 */
const SCALE_K_MIN = 0.002;
/** max 16：dist ≤ ~2350（fit 视图 + 用户拉远裕量）内屏幕字高恒定；极端拉远后温和缩小防世界尺寸病态。 */
const SCALE_K_MAX = 16;

/** 像素目标恒尺寸缩放系数（纯函数）：把 baseHeight（世界单位）映射到屏幕上 targetPx 像素高。
 *  推导：屏幕像素高 = baseHeight·k·(viewportPx/2)/(dist·tanHalf) ≡ targetPx
 *  ⇒ k = 2·dist·tanHalf·targetPx/(viewportPx·baseHeight)，与距离成正比 ⇒ 屏幕尺寸严格恒定。 */
export function scaleForPixels(
  dist: number,
  tanHalf: number,
  viewportPx: number,
  targetPx: number,
  baseHeight: number,
): number {
  const vh = Math.max(viewportPx, 1);
  const bh = Math.max(baseHeight, 1e-3);
  const k = (2 * Math.max(dist, 1e-3) * tanHalf * targetPx) / (vh * bh);
  return Math.min(SCALE_K_MAX, Math.max(SCALE_K_MIN, k));
}

/** 应用缩放（纯缩放不重绘 canvas，GPU 采样低开销）；userData.k 跟踪上次系数防累积漂移。 */
function applyScale(sprite: SpriteText, k: number): void {
  const prev = (sprite.userData.k as number | undefined) ?? 1;
  if (Math.abs(k - prev) > 1e-3) {
    sprite.scale.multiplyScalar(k / prev);
    sprite.userData.k = k;
  }
}

/** 焦点标签种类（hover 青 / selected 紫，与 ReticleLayer 配色同源）。 */
export type FocusLabelKind = 'hover' | 'selected';

/** 组标签条目（超点组名+成员数，FAR/MID 常驻）。 */
export interface GroupLabelEntry {
  /** 标签文本（如 "agents · 5"）。 */
  text: string;
  /** 世界坐标（超点质心）。 */
  x: number;
  y: number;
  z: number;
  /** 组色（标签边框色，与组同源）。 */
  borderColor: string;
  /** 超点半径（世界单位，标签放在超点上方）。 */
  size: number;
}

/** 组标签 sprite 工厂：中等醒目度（介于候选与焦点之间），屏幕恒尺寸。 */
function makeGroupSprite(borderColor: string): SpriteText {
  const s = new SpriteText(' ', 3.8, '#d8ecff');
  s.backgroundColor = 'rgba(5, 8, 16, 0.72)';
  s.borderColor = borderColor;
  s.borderWidth = 0.25;
  s.borderRadius = 1.0;
  s.padding = 0.9;
  s.fontWeight = '600';
  s.visible = false;
  s.renderOrder = 3; // 与候选标签同层
  return s;
}

/** 焦点 sprite 工厂：粗体亮字 + 深色底牌 + 描边，任何距离一眼可读。
 *  UX 优化：字高 6.5→4.2、底牌 padding 同步收敛（近景 ~76px 盒偏大 → ~45px 内）。 */
function makeFocusSprite(color: string, borderColor: string): SpriteText {
  const s = new SpriteText(' ', 4.2, color);
  s.backgroundColor = 'rgba(5, 8, 16, 0.78)';
  s.borderColor = borderColor;
  s.borderWidth = 0.3;
  s.borderRadius = 1.2;
  s.padding = 1.0;
  s.fontWeight = '700';
  s.visible = false;
  s.renderOrder = 4; // 压在候选标签（3）之上
  return s;
}

/** 候选池：按 degree 降序取 top-K（纯函数，可单测）。 */
export function selectLabelCandidates(degree: Uint16Array, maxLabels: number): number[] {
  return Array.from({ length: degree.length }, (_, i) => i)
    .sort((a, b) => degree[b] - degree[a])
    .slice(0, maxLabels);
}

/** 动态度数下限（纯函数，G5-G 小图标签修复）：图最大度数低于基准时降档到最大度数，
 *  保证小图 hub 标签可见；全孤立图钳到 1（孤立节点不出标签）。 */
export function effectiveMinDegree(maxDegree: number, base: number): number {
  return Math.max(1, Math.min(base, maxDegree));
}

/** 可见性判定（纯函数）：forced（hover/选中）无条件显示（豁免距离/度数/开关——拉远超 maxDistance 时 hover 仍须出标签）；普通标签受 距离+度数+开关 三重阈值。 */
export function shouldShowLabel(
  dist: number,
  maxDistance: number,
  deg: number,
  minDegree: number,
  forced: boolean,
  labelsEnabled: boolean,
): boolean {
  if (forced) return true;
  return dist <= maxDistance && labelsEnabled && deg >= minDegree;
}

export class LabelLayer {
  readonly group = new THREE.Group();
  /** 候选节点索引（degree 降序 top-K）。 */
  readonly candidates: number[];
  private readonly sprites: SpriteText[] = [];
  /** 候选标签构造字高（scaleForPixels 的 baseHeight）。 */
  private readonly baseTextHeight: number;
  private readonly degree: Uint16Array;
  private readonly names: string[];
  private readonly tmp = new THREE.Vector3();
  private labelsEnabled = true;
  /** 焦点标签（hover/selected 各一）：独立于候选池，任何节点都能出名称。 */
  private readonly focusSprites: Record<FocusLabelKind, SpriteText>;
  private readonly focusIndex: Record<FocusLabelKind, number | null> = { hover: null, selected: null };
  /** 组标签 sprites（与 groupLabels 一一对应，FAR/MID 常驻）。 */
  private readonly groupSprites: SpriteText[] = [];
  private groupLabels: GroupLabelEntry[] = [];

  constructor(opts: LabelLayerOpts) {
    const { names, degree } = opts;
    this.degree = degree;
    this.names = names;
    const maxLabels = opts.maxLabels ?? 80;
    // v3 可读性：#9fdcff→#8fb9d9（原色亮度≈0.8 易冒辉光，压暗配合 bloom 提阈 0.85）
    const color = opts.color ?? '#8fb9d9';
    // UX 优化：字高 4→3.2（zoomToFit 鲁棒化后相机更近，原字高同屏叠字）
    const textHeight = opts.textHeight ?? 3.2;
    this.baseTextHeight = textHeight;

    this.candidates = selectLabelCandidates(degree, maxLabels);

    for (const i of this.candidates) {
      const sprite = new SpriteText(names[i], textHeight, color);
      // UX 可读性：深色底牌（无描边）——近景边线穿过文字时仍可辨读
      sprite.backgroundColor = 'rgba(5, 8, 16, 0.55)';
      sprite.padding = 0.7;
      sprite.borderRadius = 0.6;
      sprite.visible = false;
      sprite.renderOrder = 3;
      this.sprites.push(sprite);
      this.group.add(sprite);
    }

    this.focusSprites = {
      hover: makeFocusSprite('#e8f7ff', 'rgba(84, 230, 255, 0.9)'),
      selected: makeFocusSprite('#ffffff', 'rgba(168, 85, 247, 0.95)'),
    };
    this.group.add(this.focusSprites.hover, this.focusSprites.selected);
  }

  /** 标签总开关：关时只显示 extraVisible。 */
  setLabelsEnabled(on: boolean): void {
    this.labelsEnabled = on;
  }

  /** 组标签（超点）：rebuild 时重建 sprites 池，FAR/MID 每帧 updateGroupLabels 驱动显隐。 */
  setGroupLabels(labels: GroupLabelEntry[]): void {
    this.groupLabels = labels;
    // 按需扩容 sprite 池（只增不缩，复用 GPU 纹理）
    while (this.groupSprites.length < labels.length) {
      const s = makeGroupSprite('#9fdcff');
      this.groupSprites.push(s);
      this.group.add(s);
    }
    for (let i = 0; i < this.groupSprites.length; i++) {
      if (i < labels.length) {
        const l = labels[i];
        const s = this.groupSprites[i];
        s.text = l.text;
        s.borderColor = l.borderColor;
        s.userData.baseScale = { x: s.scale.x, y: s.scale.y };
        s.userData.k = 1;
      } else {
        this.groupSprites[i].visible = false;
      }
    }
  }

  /** 焦点标签：index=null 隐藏；同名同 index 重复设置不重绘 canvas（交互事件驱动，低频）。 */
  setFocusLabel(kind: FocusLabelKind, index: number | null): void {
    if (this.focusIndex[kind] === index) return;
    this.focusIndex[kind] = index;
    const sprite = this.focusSprites[kind];
    if (index === null || index < 0 || index >= this.names.length) {
      sprite.visible = false;
      return;
    }
    sprite.text = this.names[index];
    // text 变更触发 canvas 重绘 + 基础 scale 重算：捕获新基准供 scaleForPixels 换算，缩放累计器复位
    sprite.userData.baseScale = { x: sprite.scale.x, y: sprite.scale.y };
    sprite.userData.k = 1;
  }

  /** 每帧按相机距离/度数双阈值刷新可见性与位置；末段做屏幕空间防重叠。 */
  update(positions: Float32Array, camera: THREE.Camera, vis: LabelVisibility): void {
    const camPos = camera.position;
    const persp = camera as THREE.PerspectiveCamera;
    const tanHalf = Math.tan(((persp.fov || 60) * Math.PI) / 360);
    const aspect = persp.aspect || 1;
    const viewportPx = vis.viewportPx ?? 800;
    const n = this.candidates.length;
    const shows = new Array<boolean>(n).fill(false);
    const rects: (NdcRect | null)[] = new Array<NdcRect | null>(n).fill(null);
    for (let k = 0; k < n; k++) {
      const i = this.candidates[k];
      const sprite = this.sprites[k];
      // 焦点节点统一由焦点 sprite 呈现（防重影）
      if (i === this.focusIndex.hover || i === this.focusIndex.selected) {
        sprite.visible = false;
        continue;
      }
      const x = positions[i * 3];
      const y = positions[i * 3 + 1];
      const z = positions[i * 3 + 2];
      this.tmp.set(x, y, z);
      const dist = this.tmp.distanceTo(camPos);
      const forced = vis.extraVisible?.has(i) ?? false;
      const show = shouldShowLabel(dist, vis.maxDistance, this.degree[i], vis.minDegree, forced, this.labelsEnabled);
      if (!show) {
        sprite.visible = false;
        continue;
      }
      // UX 真恒尺寸：像素目标换算（任何距离屏幕上恒 CANDIDATE_LABEL_PX），近景不再 1/dist 爆炸
      const ks = scaleForPixels(dist, tanHalf, viewportPx, CANDIDATE_LABEL_PX, this.baseTextHeight);
      applyScale(sprite, ks);
      // 锚定半径封顶：超 hub 世界半径大，偏移整体随 ks 缩放，标签始终贴合节点上方
      const r = vis.nodeSize ? Math.min(vis.nodeSize(i), LABEL_ANCHOR_R_CAP) : 0;
      const ly = y + (r + LABEL_Y_OFFSET) * ks;
      sprite.position.set(x, ly, z);
      // NDC 矩形（供防重叠）：中心投影 + 像素目标精确换算（宽=高×文本纵横比）；相机背后/远平面外直接隐藏
      this.tmp.set(x, ly, z).project(camera);
      if (this.tmp.z > 1 || this.tmp.z < -1) {
        sprite.visible = false;
        continue;
      }
      shows[k] = true;
      const worldH = this.baseTextHeight * ks;
      const worldW = sprite.scale.x; // applyScale 后实际世界宽（纵横比源自 canvas）
      const d = Math.max(dist, 1);
      const w = worldW / (d * tanHalf * aspect);
      const h = worldH / (d * tanHalf);
      rects[k] = { x0: this.tmp.x - w / 2, y0: this.tmp.y - h / 2, x1: this.tmp.x + w / 2, y1: this.tmp.y + h / 2 };
    }
    // UX 根治：屏幕空间防重叠（度数降序贪婪），近景核心不再叠字 soup
    const keep = filterOverlappingLabels(rects, vis.maxVisible ?? 12);
    for (let k = 0; k < n; k++) {
      if (shows[k]) this.sprites[k].visible = keep[k];
    }
    this.updateFocusSprite(this.focusSprites.hover, this.focusIndex.hover, positions, camera, vis);
    this.updateFocusSprite(this.focusSprites.selected, this.focusIndex.selected, positions, camera, vis);
  }

  /** 组标签定位：屏幕恒尺寸 + 质心上方偏移（FAR/MID 调用，NEAR 隐藏）。 */
  updateGroupLabels(visible: boolean, camera: THREE.Camera, vis: LabelVisibility): void {
    const n = Math.min(this.groupLabels.length, this.groupSprites.length);
    if (!visible || !this.labelsEnabled) {
      for (let i = 0; i < n; i++) this.groupSprites[i].visible = false;
      return;
    }
    const persp = camera as THREE.PerspectiveCamera;
    const tanHalf = Math.tan(((persp.fov || 60) * Math.PI) / 360);
    const viewportPx = vis.viewportPx ?? 800;
    for (let i = 0; i < n; i++) {
      const l = this.groupLabels[i];
      const s = this.groupSprites[i];
      this.tmp.set(l.x, l.y, l.z);
      const dist = this.tmp.distanceTo(camera.position);
      const base = (s.userData.baseScale as { x: number; y: number } | undefined) ?? { x: 1, y: 1 };
      const k = scaleForPixels(dist, tanHalf, viewportPx, 14, base.y);
      applyScale(s, k);
      const r = Math.min(l.size, LABEL_ANCHOR_R_CAP);
      s.position.set(l.x, l.y + (r + FOCUS_LABEL_Y_OFFSET) * k, l.z);
      s.visible = true;
    }
  }

  /** 焦点标签定位：像素目标恒尺寸（按相机距离精确换算，任何距离屏幕字高恒定）。 */
  private updateFocusSprite(
    sprite: SpriteText,
    index: number | null,
    positions: Float32Array,
    camera: THREE.Camera,
    vis: LabelVisibility,
  ): void {
    if (index === null) {
      sprite.visible = false;
      return;
    }
    const persp = camera as THREE.PerspectiveCamera;
    const tanHalf = Math.tan(((persp.fov || 60) * Math.PI) / 360);
    const viewportPx = vis.viewportPx ?? 800;
    const x = positions[index * 3];
    const y = positions[index * 3 + 1];
    const z = positions[index * 3 + 2];
    this.tmp.set(x, y, z);
    const dist = this.tmp.distanceTo(camera.position);
    const base = (sprite.userData.baseScale as { x: number; y: number } | undefined) ?? { x: 1, y: 1 };
    const k = scaleForPixels(dist, tanHalf, viewportPx, FOCUS_LABEL_PX, base.y);
    applyScale(sprite, k);
    const r = vis.nodeSize ? Math.min(vis.nodeSize(index), LABEL_ANCHOR_R_CAP) : 0;
    sprite.position.set(x, y + (r + FOCUS_LABEL_Y_OFFSET) * k, z);
    sprite.visible = true;
  }

  dispose(): void {
    for (const s of [
      ...this.sprites,
      this.focusSprites.hover,
      this.focusSprites.selected,
      ...this.groupSprites,
    ]) {
      // SpriteText 无公开 dispose()：手动释放 canvas 纹理与材质（GC 路径）。
      s.material.map?.dispose();
      s.material.dispose();
      this.group.remove(s);
    }
    this.sprites.length = 0;
    this.groupSprites.length = 0;
    this.groupLabels = [];
  }
}
