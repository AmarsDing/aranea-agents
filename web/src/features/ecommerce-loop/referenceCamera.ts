import type { ReferenceFocus, SceneId, StepId } from './types';

export interface ReferenceCameraTarget {
  selector: string;
  x: number;
  y: number;
  width: number;
}

export function referenceFocuses(scene: SceneId): ReferenceFocus[] {
  if (scene === 2) return ['ceo-summary', 'ceo-detail'];
  if (scene === 3) return ['funnel-overview', 'funnel', 'table', 'report', 'anomaly', 'suggestion'];
  if (scene === 4) return ['products'];
  return [];
}

// Native 1920×1080 source-frame anchors; uniform transforms never stretch text.
const loopSteps = {
  1: [194, 351, 964],
  2: [216, 83, 1072],
  3: [224, 265, 1112],
  4: [478, 193, 966],
  5: [434, 246, 1256],
  6: [728, 272, 1008],
  7: [605, 257, 1111],
} satisfies Record<StepId, number[]>;
const launchSteps = {
  1: [209, 975, 1177],
  2: [209, 270, 1177],
  3: [181, 230, 1248],
  4: [528, 28, 867],
  5: [686, 216, 1044],
  6: [615, 320, 1102],
  7: [686, 410, 1043],
} satisfies Record<StepId, number[]>;
const targets: Record<Exclude<ReferenceFocus, 'funnel-overview'>, ReferenceCameraTarget> = {
  'ceo-summary': { selector: '.ec-flow-ceo', x: 544, y: 371, width: 834 },
  'ceo-detail': { selector: '.ec-flow-ceo', x: 702, y: 276, width: 526 },
  products: { selector: '.ec-launch-products', x: 135, y: 337, width: 1653 },
  funnel: { selector: '.ec-funnel', x: 34, y: -6, width: 1816 },
  table: { selector: '.ec-breakdown', x: 44, y: 150, width: 1816 },
  report: { selector: '.ec-report', x: 19, y: 18, width: 1907 },
  anomaly: { selector: '[data-report-row="anomaly"]', x: 62, y: 403, width: 1838 },
  suggestion: { selector: '[data-report-row="suggestion"]', x: 142, y: 79, width: 1672 },
};

export function referenceTarget(scene: SceneId, step: StepId | null, focus: ReferenceFocus | null) {
  if (step && (scene === 2 || scene === 4)) {
    const [x, y, width] = (scene === 4 ? launchSteps : loopSteps)[step];
    return { selector: `[data-reference-step="${step}"]`, x: x!, y: y!, width: width! };
  }
  if (focus && focus !== 'funnel-overview' && referenceFocuses(scene).includes(focus)) return targets[focus];
  return null;
}

export function cameraTransform(source: { x: number; y: number; width: number }, target: ReferenceCameraTarget) {
  const scale = target.width / source.width;
  return `matrix(${scale}, 0, 0, ${scale}, ${target.x - source.x * scale}, ${target.y - source.y * scale})`;
}
