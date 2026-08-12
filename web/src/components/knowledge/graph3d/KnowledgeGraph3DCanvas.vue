<template>
  <!-- G5 深空图谱画布 v2（V12.8）：GPU 纹理管线装配（PositionTexture 一次 memcpy/tick，节点/边/瞄准具
       顶点着色器 texelFetch 取位）；自适应画质 governor（FPS 滑动窗降档保帧率，HUD 指示档位）；
       lazy-render：needsRender || particles.active || reticle.active || 高亮脉冲 || autoRotate 才渲染；
       IntersectionObserver 离屏暂停；WebGL 不可用友好占位。 -->
  <div ref="containerEl" class="kg3d-canvas">
    <div v-if="webglFailed" class="kg3d-canvas__fallback">
      <q-icon name="visibility_off" size="20px" />
      <span>{{ t('knowledgePage.graphWebglUnavailable') }}</span>
    </div>
    <div v-else class="kg3d-canvas__quality">{{ t('knowledgePage.graphQualityTier', { tier: qualityTier }) }}</div>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import * as THREE from 'three';
import { OrbitControls } from 'three/examples/jsm/controls/OrbitControls.js';
import { buildGraphModel, seedPositions, type GraphModel } from '../../../features/knowledge/graph3d/model';
import { GraphEngine } from '../../../features/knowledge/graph3d/engine';
import { classifyTiers, TIER_SIZE_MULT, tierChargeScales } from '../../../features/knowledge/graph3d/tiering';
import { buildGroupPalette, hexToRgbFloat } from '../../../features/knowledge/graph3d/palette';
import { easeInOutQuad } from '../../../features/knowledge/graph3d/particleMath';
import {
  GraphInteraction,
  isClickMovement,
  nHop,
  oneHop,
  wheelZoomFactor,
} from '../../../features/knowledge/graph3d/interaction';
import { graphLinkColor } from '../../../features/knowledge/graphUi';
import {
  GOVERN_DOWN_FPS,
  GOVERN_UP_FPS,
  QUALITY_SPECS,
  governTier,
  initialTier,
  type QualityTier,
} from '../../../features/knowledge/graph3d/qualityTiers';
import type { CollectionGraphEdge, CollectionGraphNode } from '../../../features/knowledge/types';
import { NodeLayer } from './render/NodeLayer';
import { EdgeLayer } from './render/EdgeLayer';
import { ParticleLayer } from './render/ParticleLayer';
import { BackdropLayer } from './render/BackdropLayer';
import { BloomPipeline } from './render/BloomPipeline';
import { effectiveMinDegree, LabelLayer, type LabelVisibility } from './render/LabelLayer';
import { Picker } from './render/Picker';
import { PositionTexture } from './render/PositionTexture';
import { ReticleLayer } from './render/ReticleLayer';

const props = withDefaults(
  defineProps<{
    /** 渲染节点（已经过孤立裁剪）。 */
    nodes: CollectionGraphNode[];
    edges: CollectionGraphEdge[];
    /** 选中节点 doc_id。 */
    selectedNodeId: string;
    /** 聚焦信号：+1 时相机飞往选中节点。 */
    focusSignal: number;
    /** 数据代际：变化时布局收敛后重置视野。 */
    generation: number;
    /** HUD：自动旋转开关（G5-E 接线）。 */
    autoRotate?: boolean;
    /** HUD：标签开关（G5-E 接线）。 */
    showLabels?: boolean;
    /** M2：布局模式（力导向/星系盘）。 */
    layout?: 'force' | 'galaxy';
  }>(),
  { autoRotate: false, showLabels: true, layout: 'force' },
);

const emit = defineEmits<{
  'node-click': [docId: string];
  'background-click': [];
  /** 双击节点 =「在浏览中打开」（D-4，沿用 G4 跨 tab 定位链路）。 */
  'node-dblclick': [payload: { docId: string; relPath: string }];
  /** M4：聚焦锁定变化（''=解除）。 */
  'focus-change': [docId: string];
}>();

const { t } = useI18n();

/** 深空不透明底（bloom 与透明背景不兼容——反模式①）。 */
const BG_HEX = 0x050810;
/** 布局确定性播种（同数据同布局）。 */
const LAYOUT_SEED = 1337;
/** 节点尺寸 = base + √degree·scale（沿用 G4 graphNodeVal 曲线）。 */
const NODE_SIZE_BASE = 1.5;
const NODE_SIZE_SCALE = 1.5;
/** 标签度数阈值（候选池上限随画质档）。 */
const LABEL_MIN_DEGREE = 4;
/** hover 拾取去抖：位移不足不重射线（防粒子相位重置）。 */
const HOVER_REPICK_PX = 4;
/** 相机远裁剪（需覆盖星云球半径 5000）。 */
const CAMERA_FAR = 20000;

const containerEl = ref<HTMLElement | null>(null);
const webglFailed = ref(false);
/** HUD 画质档指示（governor 驱动）。 */
const qualityTier = ref<'HIGH' | 'MID' | 'LOW'>('HIGH');

// ---- three/引擎实例（非响应式） ----
let renderer: THREE.WebGLRenderer | null = null;
let scene: THREE.Scene | null = null;
let camera: THREE.PerspectiveCamera | null = null;
let controls: OrbitControls | null = null;
let bloom: BloomPipeline | null = null;
let backdrop: BackdropLayer | null = null;
let nodeLayer: NodeLayer | null = null;
let edgeLayer: EdgeLayer | null = null;
let labelLayer: LabelLayer | null = null;
let posTex: PositionTexture | null = null;
let reticle: ReticleLayer | null = null;
let picker: Picker | null = null;
const particleLayer = new ParticleLayer();
let engine: GraphEngine | null = null;
let model: GraphModel | null = null;
const interaction = new GraphInteraction();
/** M2：当前布局模式（决定边层细分/曲率 + 物理三力预设；初始取 prop 支持刷新后恢复星系盘）。 */
let currentLayout: 'force' | 'galaxy' = props.layout;
const labelVis: LabelVisibility = { maxDistance: 600, minDegree: LABEL_MIN_DEGREE, extraVisible: new Set() };

// ---- 画质 governor 状态（FPS EMA + 连续低/高帧计数） ----
let tier: QualityTier = 0 as QualityTier; // QUALITY_HIGH 起步，rebuild 时按节点数重定
let tierCeiling: QualityTier = 0 as QualityTier;
let fpsEma = 60;
let lowFrames = 0;
let highFrames = 0;
/** 画布尺寸缓存（applyQuality 重设 pixelRatio 后需回放 setSize）。 */
let lastW = 1;
let lastH = 1;

// ---- lazy-render 状态 ----
let needsRender = true;
let rafId: number | null = null;
let paused = false;
let lastFrameT = 0;
let resizeObserver: ResizeObserver | null = null;
let intersectionObserver: IntersectionObserver | null = null;
/** 布局收敛后待执行的 zoomToFit（首载/代际变化）。 */
let pendingFit = true;

// ---- M3 创世绽放（genesis reveal，非响应式） ----
/** 创世进度：0=收拢于核心，1=完全显现（默认 1 无动画；LOW 档恒 1）。 */
let revealT = 1;
let genesisStart = 0;

// ---- hover 拾取合并（RAF 内每帧最多一次射线） ----
let hoverDirty = false;
let lastPickX = 0;
let lastPickY = 0;
const ndc = new THREE.Vector2();

// ---- 拖拽状态（pin-and-move） ----
const drag = {
  index: -1,
  active: false,
  moved: false,
  startX: 0,
  startY: 0,
  plane: new THREE.Plane(),
  offset: new THREE.Vector3(),
};
/** pointerdown 落在画布内才允许 click 判定（防外部按下拖入抬起误触 background-click）。 */
let downOnCanvas = false;

// ---- 相机动画（zoomToFit/聚焦共用一个 tween） ----
const tween = {
  active: false,
  t0: 0,
  dur: 0,
  fromPos: new THREE.Vector3(),
  toPos: new THREE.Vector3(),
  fromTgt: new THREE.Vector3(),
  toTgt: new THREE.Vector3(),
};

const raycaster = new THREE.Raycaster();
const tmpV1 = new THREE.Vector3();
const tmpV2 = new THREE.Vector3();

function requestRender(): void {
  needsRender = true;
}

// ---------------------------------------------------------------- 场景装配

function initScene(el: HTMLElement): boolean {
  try {
    renderer = new THREE.WebGLRenderer({ antialias: true, alpha: false });
  } catch {
    return false;
  }
  renderer.setClearColor(BG_HEX, 1);
  renderer.toneMapping = THREE.ACESFilmicToneMapping;
  renderer.toneMappingExposure = 1.0; // v2 降亮度：1.2 → 1.0
  renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
  const w = el.clientWidth || 1;
  const h = el.clientHeight || 1;
  lastW = w;
  lastH = h;
  renderer.setSize(w, h, false);
  el.appendChild(renderer.domElement);

  scene = new THREE.Scene();
  camera = new THREE.PerspectiveCamera(60, w / h, 0.1, CAMERA_FAR);
  camera.position.set(0, 0, 400);
  picker = new Picker(camera);

  controls = new OrbitControls(camera, renderer.domElement);
  controls.enableDamping = false; // lazy-render 友好：无阻尼惯性帧
  controls.enableZoom = false; // 滚轮走自研 zoom-to-cursor（D-3）
  controls.autoRotate = props.autoRotate;
  controls.autoRotateSpeed = 0.6;
  controls.addEventListener('change', requestRender);
  // M3：用户拖拽接管 → 创世动画立即完成（镜头让位手控）
  controls.addEventListener('start', () => {
    if (revealT < 1) {
      revealT = 1;
      nodeLayer?.setRevealT(1);
    }
  });

  backdrop = new BackdropLayer(renderer); // 构造内一次性烘焙星云
  scene.add(backdrop.group);
  scene.add(particleLayer.points);
  bloom = new BloomPipeline(renderer, scene, camera, w, h);
  return true;
}

/** gl_PointSize 像素缩放 = drawingBufferHeight·0.5 / tan(fov/2)（设备像素域）。 */
function pointScale(): number {
  const hPx = renderer?.domElement.height ?? 1;
  const fov = camera?.fov ?? 60;
  return (hPx * 0.5) / Math.tan((fov * Math.PI) / 360);
}

/** 应用画质档：pixelRatio 上限 + bloom 开关/分辨率 + HUD 指示；计数器清零防立刻回弹。 */
function applyQuality(next: QualityTier): void {
  tier = next;
  const spec = QUALITY_SPECS[next];
  qualityTier.value = spec.label;
  lowFrames = 0;
  highFrames = 0;
  if (renderer) {
    renderer.setPixelRatio(Math.min(window.devicePixelRatio, spec.maxPixelRatio));
    renderer.setSize(lastW, lastH, false);
  }
  bloom?.setBloomEnabled(spec.bloom);
  bloom?.setResolutionScale(spec.bloomScale);
  nodeLayer?.setPointScale(pointScale());
  requestRender();
}

/** 数据 → 图模型 + 渲染层重建（nodes/edges 变更）。 */
function rebuildGraph(): void {
  if (!scene || !camera) return;
  disposeGraph();
  model = buildGraphModel(
    props.nodes.map((n) => ({ docId: n.doc_id, name: n.name, relPath: n.rel_path, docType: n.doc_type })),
    props.edges.map((e) => ({ source: e.source, target: e.target, type: e.type })),
  );
  if (model.count === 0) {
    model = null;
    return;
  }
  seedPositions(model, LAYOUT_SEED);
  const m = model;

  // 画质初始分级（万级 LOW 起步保帧率），governor 升档不越此顶
  tierCeiling = initialTier(m.count);
  applyQuality(tierCeiling);

  const tiers = classifyTiers(m.degree, m.edges);
  const sizeMult = new Float32Array(m.count);
  for (let i = 0; i < m.count; i++) sizeMult[i] = TIER_SIZE_MULT[tiers[i]];

  // GPU 纹理管线：物理 tick → 一次 memcpy → 节点/边/瞄准具顶点着色器 texelFetch
  posTex = new PositionTexture(m.count);
  posTex.update(m.positions);

  // 节点基础色（palette 分组色 → aColor 静态属性）
  const palette = buildGroupPalette(m.groups).map((hex) => hexToRgbFloat(hex));
  const nodeColors = new Float32Array(m.count * 3);
  for (let i = 0; i < m.count; i++) {
    const [r, g, b] = palette[m.groupId[i]] ?? [0.5, 0.5, 0.5];
    nodeColors[i * 3] = r;
    nodeColors[i * 3 + 1] = g;
    nodeColors[i * 3 + 2] = b;
  }
  nodeLayer = new NodeLayer(m.count);
  nodeLayer.setPositionTexture(posTex.texture, posTex.width);
  nodeLayer.setPointScale(pointScale());
  nodeLayer.setColors(nodeColors);
  nodeLayer.setSizes(m.degree, NODE_SIZE_BASE, NODE_SIZE_SCALE, sizeMult);
  scene.add(nodeLayer.points);

  // 边层（M2：按布局选细分段数/曲率；颜色与位置纹理接线在 rebuildEdges 内）
  rebuildEdges();

  reticle = new ReticleLayer();
  reticle.setPositionTexture(posTex.texture, posTex.width);
  scene.add(reticle.object);

  labelLayer = new LabelLayer({ names: m.names, degree: m.degree, maxLabels: QUALITY_SPECS[tier].labelCandidates });
  labelLayer.setLabelsEnabled(props.showLabels);
  scene.add(labelLayer.group);
  // 动态度数下限（G5-G）：小图（最大度数 < 基准 4）降档到最大度数，hub 标签适应视图后可见。
  let maxDeg = 0;
  for (let i = 0; i < m.count; i++) if (m.degree[i] > maxDeg) maxDeg = m.degree[i];
  labelVis.minDegree = effectiveMinDegree(maxDeg, LABEL_MIN_DEGREE);

  interaction.setHover(null);
  interaction.setSelected(m.docIdToIndex.get(props.selectedNodeId) ?? null);
  // M4：数据重建后节点索引失效，聚焦锁定一并复位（通知上层卸载 FocusCard）
  if (interaction.focused !== null) {
    interaction.clearFocus();
    emit('focus-change', '');
  }

  engine = new GraphEngine(
    m,
    { onTick: handleTick, onSettled: handleSettled },
    { chargeScale: tierChargeScales(tiers) },
  );
  pendingFit = true;
  engine.start();
  // M2：初始布局为星系盘时，启动后即切预设（setParams+reheat；须 start 后调用，启动前 setParams 为空操作）
  if (currentLayout === 'galaxy') engine.setLayout('galaxy');
  // M3 创世绽放：LOW 档跳过（uniform 直接 1）
  if (QUALITY_SPECS[tier].label !== 'LOW') {
    revealT = 0;
    genesisStart = performance.now();
    nodeLayer?.setRevealT(0);
  } else {
    revealT = 1;
    nodeLayer?.setRevealT(1);
  }
  requestRender();
}

/** M2：边层重建（布局切换联动）——dispose 旧层 → 按 currentLayout 新建（星系盘 8 段弧线/力导向直线）→ 恢复位置纹理与高亮。 */
function rebuildEdges(): void {
  if (!scene || !model || !posTex) return;
  // 仅「替换存活层」（布局切换）时恢复高亮；初次重建沿用旧行为（全新层零高亮，interaction 随后重置）
  const replacing = edgeLayer !== null;
  if (edgeLayer) {
    scene.remove(edgeLayer.object);
    edgeLayer.dispose();
  }
  const m = model;
  // 边基础色（边类型色，层内乘 rest/hover 系数）
  const edgeColors = new Float32Array(m.edgeCount * 3);
  for (let e = 0; e < m.edgeCount; e++) {
    const [r, g, b] = hexToRgbFloat(graphLinkColor(m.edgeTypes[e]));
    edgeColors[e * 3] = r;
    edgeColors[e * 3 + 1] = g;
    edgeColors[e * 3 + 2] = b;
  }
  edgeLayer = new EdgeLayer(m.edges, edgeColors, currentLayout === 'galaxy' ? 8 : 1);
  edgeLayer.setCurvature(currentLayout === 'galaxy' ? 0.18 : 0);
  edgeLayer.setPositionTexture(posTex.texture, posTex.width);
  scene.add(edgeLayer.object);
  if (replacing) applyHighlight(); // 恢复高亮状态
}

function disposeGraph(): void {
  engine?.stop();
  engine = null;
  if (scene) {
    if (nodeLayer) scene.remove(nodeLayer.points);
    if (edgeLayer) scene.remove(edgeLayer.object);
    if (reticle) scene.remove(reticle.object);
    if (labelLayer) scene.remove(labelLayer.group);
  }
  nodeLayer?.dispose();
  nodeLayer = null;
  edgeLayer?.dispose();
  edgeLayer = null;
  reticle?.dispose();
  reticle = null;
  labelLayer?.dispose();
  labelLayer = null;
  posTex?.dispose();
  posTex = null;
  particleLayer.setSource(null, []);
  revealT = 1; // M3：重置创世进度（防残留动画状态）
  drag.active = false;
  drag.index = -1;
}

// ---------------------------------------------------------------- 物理回调

function handleTick(positions: Float32Array): void {
  // GPU 纹理管线：每 tick 仅一次 memcpy + 纹理上传（万级 ≈0.3ms），节点/边/瞄准具零 CPU 几何计算
  posTex?.update(positions);
  requestRender();
}

function handleSettled(): void {
  // 核雾锚定度数最大 hub
  if (model && backdrop) {
    let hub = 0;
    for (let i = 1; i < model.count; i++) if (model.degree[i] > model.degree[hub]) hub = i;
    const p = engine?.positions ?? model.positions;
    backdrop.setHazeAnchor(p[hub * 3], p[hub * 3 + 1], p[hub * 3 + 2]);
  }
  if (pendingFit) {
    pendingFit = false;
    zoomToFit(600);
  }
  requestRender();
}

// ---------------------------------------------------------------- 高亮/拾取

/** 邻居集驱动 NodeLayer 提亮 / EdgeLayer 脉冲 / ParticleLayer 发射 / ReticleLayer 瞄准具（D-1）。 */
function applyHighlight(): void {
  if (!model || !nodeLayer || !edgeLayer) return;
  // M4 聚焦锁定：BFS N 跳 dim（优先级高于 hover，锁定态 hover 不覆盖）
  const focused = interaction.focused;
  if (focused !== null) {
    const { nodes, edges } = nHop(model.edges, model.edgeCount, focused, interaction.focusHops);
    nodeLayer.setHighlight(nodes);
    edgeLayer.setHighlight(edges);
    particleLayer.setSource(null, []);
    if (reticle) {
      const sel = interaction.selected;
      reticle.setHover(null, 0);
      reticle.setSelected(sel, sel !== null ? nodeLayer.nodeSize(sel) : 0);
    }
    labelVis.extraVisible = new Set([focused]);
    requestRender();
    return;
  }
  const active = interaction.active;
  if (active === null) {
    nodeLayer.setHighlight(null);
    edgeLayer.setHighlight(null);
    particleLayer.setSource(null, []);
  } else {
    const { nodes, edges } = oneHop(model.edges, model.edgeCount, active);
    nodeLayer.setHighlight(nodes);
    edgeLayer.setHighlight(edges);
    if (interaction.hover !== null) {
      const neighbors = [...nodes].filter((i) => i !== interaction.hover);
      particleLayer.setSource(interaction.hover, neighbors);
    } else {
      particleLayer.setSource(null, []);
    }
  }
  if (reticle) {
    const hov = interaction.hover;
    const sel = interaction.selected;
    reticle.setHover(hov, hov !== null ? nodeLayer.nodeSize(hov) : 0);
    reticle.setSelected(sel, sel !== null ? nodeLayer.nodeSize(sel) : 0);
  }
  labelVis.extraVisible = new Set([interaction.hover, interaction.selected].filter((v): v is number => v !== null));
  requestRender();
}

function eventToNdc(ev: PointerEvent | MouseEvent | WheelEvent): void {
  const el = renderer!.domElement;
  const rect = el.getBoundingClientRect();
  ndc.set(((ev.clientX - rect.left) / rect.width) * 2 - 1, -((ev.clientY - rect.top) / rect.height) * 2 + 1);
}

/** 统一拾取入口：射线-球 O(N) 纯循环（物理缓冲 + 半径缓冲直读，无矩阵求逆）。 */
function pickAt(): number | null {
  if (!picker || !engine || !model || !nodeLayer) return null;
  return picker.pick(ndc.x, ndc.y, engine.positions, nodeLayer.sizeData, model.count, containerEl.value?.clientHeight ?? 1);
}

/** hover 拾取（RAF 内合并调用）：去抖防粒子相位重置。 */
function doHoverPick(): void {
  if (!model) return;
  const idx = pickAt();
  if (interaction.setHover(idx)) applyHighlight();
  const el = containerEl.value;
  if (el) el.style.cursor = idx !== null ? 'pointer' : '';
}

// ---------------------------------------------------------------- 指针交互

function onPointerDown(ev: PointerEvent): void {
  if (ev.button !== 0 || !model || !camera) return;
  tween.active = false; // 用户介入取消相机动画
  downOnCanvas = true;
  eventToNdc(ev);
  const idx = pickAt();
  drag.startX = ev.clientX;
  drag.startY = ev.clientY;
  drag.moved = false;
  drag.index = idx ?? -1;
  drag.active = idx !== null;
  if (drag.active && engine) {
    renderer!.domElement.setPointerCapture(ev.pointerId);
    // 拖拽平面：过节点、法线朝相机；grabOffset 防跳变
    const p = engine.positions;
    tmpV1.set(p[drag.index * 3], p[drag.index * 3 + 1], p[drag.index * 3 + 2]);
    tmpV2.copy(camera.position).sub(tmpV1).normalize();
    drag.plane.setFromNormalAndCoplanarPoint(tmpV2, tmpV1);
    raycaster.setFromCamera(ndc, camera);
    const hit = raycaster.ray.intersectPlane(drag.plane, tmpV2);
    drag.offset.copy(hit ? hit.sub(tmpV1) : tmpV2.set(0, 0, 0));
  }
}

function onPointerMove(ev: PointerEvent): void {
  if (!camera || !engine || !model) return;
  eventToNdc(ev);
  if (drag.active) {
    const dx = ev.clientX - drag.startX;
    const dy = ev.clientY - drag.startY;
    if (!drag.moved && !isClickMovement(dx, dy)) {
      drag.moved = true;
      if (controls) controls.enabled = false; // 拖拽中挂起 controls/autoRotate
    }
    if (drag.moved) {
      raycaster.setFromCamera(ndc, camera);
      const hit = raycaster.ray.intersectPlane(drag.plane, tmpV1);
      if (hit) {
        hit.sub(drag.offset);
        engine.pin(drag.index, hit.x, hit.y, hit.z);
        // 零延迟直写当前渲染位置（下一 tick 同值覆盖）
        const p = engine.positions;
        p[drag.index * 3] = hit.x;
        p[drag.index * 3 + 1] = hit.y;
        p[drag.index * 3 + 2] = hit.z;
        posTex?.update(p);
        requestRender();
      }
    }
    return;
  }
  // hover 去抖：位移不足不重新射线（同时防粒子相位重置）
  if (Math.abs(ev.clientX - lastPickX) + Math.abs(ev.clientY - lastPickY) >= HOVER_REPICK_PX) {
    lastPickX = ev.clientX;
    lastPickY = ev.clientY;
    hoverDirty = true;
  }
}

function onPointerUp(ev: PointerEvent): void {
  if (!model || !downOnCanvas) return;
  downOnCanvas = false;
  const wasDragging = drag.active && drag.moved;
  const wasCandidate = drag.active;
  drag.active = false;
  if (renderer?.domElement.hasPointerCapture(ev.pointerId)) {
    renderer.domElement.releasePointerCapture(ev.pointerId);
  }
  if (controls && !controls.enabled) controls.enabled = true;

  if (wasDragging) {
    // pin 后位置持久（保持 fx/fy/fz）；恢复时 hoverId 指向刚放下节点（反模式⑤）
    if (interaction.setHover(drag.index)) applyHighlight();
    drag.index = -1;
    return;
  }
  drag.index = -1;
  const dx = ev.clientX - drag.startX;
  const dy = ev.clientY - drag.startY;
  if (wasCandidate || isClickMovement(dx, dy)) {
    eventToNdc(ev);
    const idx = pickAt();
    if (idx !== null) {
      interaction.setSelected(idx);
      interaction.setFocus(idx, 2); // M4：单击锁定聚焦（2 跳 dim）
      emit('node-click', model.docIds[idx]);
      emit('focus-change', model.docIds[idx]);
    } else {
      interaction.setSelected(null);
      interaction.clearFocus(); // M4：单击空白解除聚焦锁定
      emit('background-click');
      emit('focus-change', '');
    }
    applyHighlight();
  }
}

function onPointerLeave(): void {
  if (interaction.setHover(null)) applyHighlight();
}

/** zoom-to-cursor（D-3）：滚轮射线 ∩ 过 target 面向相机平面求 pivot，相机+target 同步缩放。 */
function onWheel(ev: WheelEvent): void {
  if (!camera || !controls) return;
  ev.preventDefault();
  tween.active = false;
  eventToNdc(ev);
  raycaster.setFromCamera(ndc, camera);
  camera.getWorldDirection(tmpV1);
  drag.plane.setFromNormalAndCoplanarPoint(tmpV1, controls.target);
  const pivot = raycaster.ray.intersectPlane(drag.plane, tmpV2) ?? tmpV2.copy(controls.target);
  const dy = ev.deltaMode === 1 ? ev.deltaY * 16 : ev.deltaY; // 行模式 → px
  const f = wheelZoomFactor(dy);
  camera.position.sub(pivot).multiplyScalar(f).add(pivot);
  controls.target.sub(pivot).multiplyScalar(f).add(pivot);
  requestRender();
}

function onDblClick(ev: MouseEvent): void {
  if (!model) return;
  eventToNdc(ev);
  const idx = pickAt();
  if (idx !== null) {
    emit('node-dblclick', { docId: model.docIds[idx], relPath: model.relPaths[idx] });
  }
}

// ---------------------------------------------------------------- 相机

/** 相机动画 tween（zoomToFit/聚焦共用）。 */
function flyTo(toPos: THREE.Vector3, toTgt: THREE.Vector3, ms: number): void {
  if (!camera || !controls) return;
  tween.fromPos.copy(camera.position);
  tween.toPos.copy(toPos);
  tween.fromTgt.copy(controls.target);
  tween.toTgt.copy(toTgt);
  tween.t0 = performance.now();
  tween.dur = Math.max(1, ms);
  tween.active = true;
  requestRender();
}

function stepTween(now: number): void {
  if (!camera || !controls) return;
  const p = Math.min(1, (now - tween.t0) / tween.dur);
  const e = easeInOutQuad(p);
  camera.position.lerpVectors(tween.fromPos, tween.toPos, e);
  controls.target.lerpVectors(tween.fromTgt, tween.toTgt, e);
  if (p >= 1) tween.active = false;
  requestRender();
}

/** 适应视图：包围球 + 当前视角方向拉远（工具条/收敛后调用）。 */
function zoomToFit(ms = 600): void {
  if (!camera || !controls || !engine || !model || model.count === 0) return;
  const p = engine.positions;
  const box = new THREE.Box3();
  for (let i = 0; i < model.count; i++) {
    box.expandByPoint(tmpV1.set(p[i * 3], p[i * 3 + 1], p[i * 3 + 2]));
  }
  const center = box.getCenter(new THREE.Vector3());
  const radius = Math.max(box.getSize(tmpV1).length() / 2, 20);
  const fov = (camera.fov * Math.PI) / 180;
  const distV = radius / Math.tan(fov / 2);
  const distH = distV / Math.max(camera.aspect, 0.01);
  const dist = Math.max(distV, distH) * 1.15;
  tmpV1.copy(camera.position).sub(controls.target);
  if (tmpV1.lengthSq() < 1e-6) tmpV1.set(0, 0, 1);
  tmpV1.normalize();
  const toPos = center.clone().add(tmpV1.multiplyScalar(dist));
  // 标签距离阈值（G5-G 修复）：fitDist + 半径 → 适应视图时全图候选标签可达（由度数下限控量），
  // 拉远后按距离渐进隐藏（原 ×0.85 在适应视图即全隐藏）。
  labelVis.maxDistance = dist + radius;
  flyTo(toPos, center, ms);
}

// ---------------------------------------------------------------- 渲染循环

function frame(now: number): void {
  if (paused) return;
  rafId = requestAnimationFrame(frame);
  const rawDt = (now - lastFrameT) / 1000;
  const dt = Math.min(rawDt, 0.05);
  lastFrameT = now;

  if (hoverDirty) {
    hoverDirty = false;
    doHoverPick();
  }
  if (tween.active) stepTween(now);
  // M3 创世绽放推进：1.2s 内 revealT 0→1，期间持续保活渲染
  if (revealT < 1) {
    revealT = Math.min(1, (performance.now() - genesisStart) / 1200);
    nodeLayer?.setRevealT(revealT);
    requestRender();
  }
  if (controls?.autoRotate) {
    controls.update();
    requestRender();
  }
  if (particleLayer.active && engine) {
    particleLayer.update(engine.positions, dt);
    requestRender();
  }
  // 高亮边流动脉冲 / 瞄准具呼吸：活跃期持续推进时钟（lazy-render 保活条件）
  if ((edgeLayer?.highlightedEdges?.size ?? 0) > 0 && edgeLayer) {
    edgeLayer.setTime(now / 1000);
    requestRender();
  }
  if (reticle?.active) {
    reticle.setTime(now / 1000);
    requestRender();
  }
  // 画质 governor：FPS EMA 连续低帧降档 / 连续高帧升档（不越初始档顶）
  if (rawDt > 0 && engine) {
    fpsEma = fpsEma * 0.92 + (1 / rawDt) * 0.08;
    if (fpsEma < GOVERN_DOWN_FPS) {
      lowFrames++;
      highFrames = 0;
    } else if (fpsEma > GOVERN_UP_FPS) {
      highFrames++;
      lowFrames = 0;
    } else {
      lowFrames = 0;
      highFrames = 0;
    }
    const next = governTier(tier, lowFrames, highFrames, tierCeiling);
    if (next !== tier) applyQuality(next);
  }
  // lazy-render：needsRender || 活跃动画源 才过 GPU
  if (needsRender && bloom && camera && engine) {
    labelLayer?.update(engine.positions, camera, labelVis);
    bloom.render();
    needsRender = false;
  }
}

function startLoop(): void {
  if (rafId === null) {
    lastFrameT = performance.now();
    rafId = requestAnimationFrame(frame);
  }
}

function stopLoop(): void {
  if (rafId !== null) {
    cancelAnimationFrame(rafId);
    rafId = null;
  }
}

// ---------------------------------------------------------------- 生命周期

onMounted(() => {
  const el = containerEl.value;
  if (!el) return;
  if (!initScene(el)) {
    webglFailed.value = true;
    return;
  }
  const canvas = renderer!.domElement;
  canvas.addEventListener('pointerdown', onPointerDown);
  canvas.addEventListener('pointermove', onPointerMove);
  canvas.addEventListener('pointerup', onPointerUp);
  canvas.addEventListener('pointerleave', onPointerLeave);
  canvas.addEventListener('wheel', onWheel, { passive: false });
  canvas.addEventListener('dblclick', onDblClick);

  resizeObserver = new ResizeObserver(() => {
    if (!renderer || !camera || !bloom) return;
    const w = el.clientWidth;
    const h = el.clientHeight;
    if (w === 0 || h === 0) return;
    lastW = w;
    lastH = h;
    renderer.setSize(w, h, false);
    camera.aspect = w / h;
    camera.updateProjectionMatrix();
    bloom.setSize(w, h);
    nodeLayer?.setPointScale(pointScale());
    requestRender();
  });
  resizeObserver.observe(el);

  // 离屏暂停（IntersectionObserver）
  intersectionObserver = new IntersectionObserver((entries) => {
    const visible = entries[0]?.isIntersecting ?? true;
    paused = !visible;
    if (visible) startLoop();
    else stopLoop();
  });
  intersectionObserver.observe(el);

  rebuildGraph();
  startLoop();
});

onBeforeUnmount(() => {
  stopLoop();
  resizeObserver?.disconnect();
  intersectionObserver?.disconnect();
  resizeObserver = null;
  intersectionObserver = null;
  disposeGraph();
  picker = null;
  controls?.dispose();
  controls = null;
  particleLayer.dispose();
  backdrop?.dispose();
  backdrop = null;
  bloom?.dispose();
  bloom = null;
  if (renderer) {
    renderer.dispose();
    renderer.forceContextLoss();
    renderer.domElement.remove();
    renderer = null;
  }
  scene = null;
  camera = null;
});

// ---------------------------------------------------------------- 数据/信号监听

watch(
  () => [props.nodes, props.edges],
  () => rebuildGraph(),
);

watch(
  () => props.generation,
  () => {
    if (engine?.settled) zoomToFit(600);
    else pendingFit = true;
  },
);

watch(
  () => props.selectedNodeId,
  (id) => {
    if (!model) return;
    if (interaction.setSelected(id ? (model.docIdToIndex.get(id) ?? null) : null)) applyHighlight();
  },
);

watch(
  () => props.focusSignal,
  () => {
    if (!engine || !model || !camera || !controls || !props.selectedNodeId) return;
    const idx = model.docIdToIndex.get(props.selectedNodeId);
    if (idx === undefined) return;
    const p = engine.positions;
    tmpV1.set(p[idx * 3], p[idx * 3 + 1], p[idx * 3 + 2]);
    // 沿当前视角方向飞到距节点 120 处（沿用 G4 聚焦距离）
    tmpV2.copy(camera.position).sub(controls.target);
    if (tmpV2.lengthSq() < 1e-6) tmpV2.set(0, 0, 1);
    tmpV2.normalize().multiplyScalar(120);
    flyTo(tmpV1.clone().add(tmpV2), tmpV1.clone(), 900);
  },
);

watch(
  () => props.autoRotate,
  (v) => {
    if (controls) controls.autoRotate = v;
    requestRender();
  },
);

watch(
  () => props.showLabels,
  (v) => {
    labelLayer?.setLabelsEnabled(v);
    requestRender();
  },
);

// M2：布局切换——边层重建（弧线/直线）+ 物理参数预设 morph（相机不动，pendingFit 不重置）
watch(
  () => props.layout,
  (v) => {
    if (v === currentLayout) return;
    currentLayout = v;
    rebuildEdges();
    engine?.setLayout(v);
    requestRender();
  },
);

/** M4：解除聚焦锁定（FocusCard 关闭按钮经 KnowledgeGraph3D 调用）。 */
function clearFocusLock(): void {
  if (interaction.focused === null) return;
  interaction.clearFocus();
  applyHighlight();
  emit('focus-change', '');
}

/** M5 透镜：按 doc_type 组临时提亮/dim（null 解除，恢复 hover/selected 驱动）。
 *  互斥纪律：聚焦锁定时透镜不生效；透镜激活时清除 hover。 */
function setLens(docType: string | null): void {
  if (!model || !nodeLayer || !edgeLayer) return;
  if (interaction.focused !== null) return; // 聚焦锁定优先
  if (docType === null) {
    applyHighlight();
    return;
  }
  interaction.setHover(null);
  const gid = model.groups.indexOf(docType);
  if (gid < 0) return; // 组被隐藏（不在当前模型）
  const nodes = new Set<number>();
  for (let i = 0; i < model.count; i++) if (model.groupId[i] === gid) nodes.add(i);
  const edges = new Set<number>();
  for (let e = 0; e < model.edgeCount; e++) {
    if (nodes.has(model.edges[e * 2]) && nodes.has(model.edges[e * 2 + 1])) edges.add(e);
  }
  nodeLayer.setHighlight(nodes);
  edgeLayer.setHighlight(edges);
  requestRender();
}

defineExpose({ zoomToFit, clearFocus: clearFocusLock, setLens });
</script>

<style lang="scss" scoped>
.kg3d-canvas {
  position: relative;
  width: 100%;
  height: 100%;
  min-height: 420px;
  overflow: hidden;
  border-radius: 10px;
  background: #050810; // 不透明深空底（bloom 兼容）

  :deep(canvas) {
    display: block;
    width: 100%;
    height: 100%;
  }

  &__fallback {
    position: absolute;
    inset: 0;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 8px;
    font-size: 13px;
    color: var(--color-text-secondary);
  }

  &__quality {
    position: absolute;
    left: 10px;
    bottom: 8px;
    padding: 2px 8px;
    border-radius: 6px;
    font-size: 10px;
    font-family: 'JetBrains Mono', 'SFMono-Regular', Consolas, monospace;
    letter-spacing: 0.08em;
    color: rgba(159, 220, 255, 0.62);
    background: rgba(9, 13, 20, 0.42);
    border: 1px solid rgba(159, 220, 255, 0.16);
    pointer-events: none;
    user-select: none;
  }
}
</style>
