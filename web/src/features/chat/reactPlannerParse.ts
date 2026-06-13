/**
 * Parse ReAct planner tags embedded in assistant content_markdown.
 *
 * @deprecated Activity-First architecture resolves ReAct labels at the backend
 * via ActivityProjector (label field on thinking activities). This module is
 * only needed for the legacy message-inference fallback path.
 */

import { i18n } from '../../i18n';
import type { ReactParsedContent, ReactStep, ReactStepKind } from './reactPlannerTypes';

export type { ReactParsedContent, ReactStep, ReactStepKind } from './reactPlannerTypes';

const TAG_DEFS: { tag: string; kind: ReactStepKind }[] = [
  { tag: '/*PLANNING*/', kind: 'planning' },
  { tag: '/*REASONING*/', kind: 'reasoning' },
  { tag: '/*ACTION*/', kind: 'action' },
  { tag: '/*REPLANNING*/', kind: 'replanning' },
];

function reactStepTitle(kind: ReactStepKind): string {
  return String(i18n.global.t(`chat.react.${kind}`));
}

const FINAL_TAG = '/*FINAL_ANSWER*/';

function findEarliestTag(
  text: string,
  from: number,
): { index: number; tag: string; kind?: ReactStepKind; isFinal?: boolean } | null {
  let best: { index: number; tag: string; kind?: ReactStepKind; isFinal?: boolean } | null = null;
  for (const def of TAG_DEFS) {
    const i = text.indexOf(def.tag, from);
    if (i >= 0 && (best === null || i < best.index)) {
      best = { index: i, tag: def.tag, kind: def.kind };
    }
  }
  const fi = text.indexOf(FINAL_TAG, from);
  if (fi >= 0 && (best === null || fi < best.index)) {
    best = { index: fi, tag: FINAL_TAG, isFinal: true };
  }
  return best;
}

export function contentHasReactTags(text: string): boolean {
  const t = text || '';
  return TAG_DEFS.some((d) => t.includes(d.tag)) || t.includes(FINAL_TAG);
}

export function parseReactPlannerContent(text: string): ReactParsedContent | null {
  const raw = (text || '').trim();
  if (!raw || !contentHasReactTags(raw)) {
    return null;
  }

  const steps: ReactStep[] = [];
  let pos = 0;
  let finalAnswer = '';
  let hasExplicitFinalAnswer = false;
  let sawTag = false;

  while (pos < raw.length) {
    const hit = findEarliestTag(raw, pos);
    if (!hit) break;
    sawTag = true;
    const contentStart = hit.index + hit.tag.length;
    if (hit.isFinal) {
      finalAnswer = raw.slice(contentStart).trim();
      hasExplicitFinalAnswer = true;
      break;
    }
    const next = findEarliestTag(raw, contentStart);
    const segmentEnd = next ? next.index : raw.length;
    const body = raw.slice(contentStart, segmentEnd).trim();
    if (hit.kind) {
      steps.push({ kind: hit.kind, title: reactStepTitle(hit.kind), body });
    }
    pos = segmentEnd;
  }

  if (!sawTag) {
    return null;
  }

  if (!finalAnswer && steps.length > 0) {
    finalAnswer = steps[steps.length - 1].body;
  }

  return {
    steps,
    finalAnswer,
    fallbackMarkdown: raw,
    hasExplicitFinalAnswer,
  };
}

export function shouldUseReactPlannerView(plannerKind: string, text: string): boolean {
  const k = plannerKind.trim().toLowerCase();
  if (k === 'react') return true;
  if (k === 'a2ui' || k === 'builtin') return false;
  return contentHasReactTags(text);
}

export function reactDisplayMarkdown(parsed: ReactParsedContent | null, raw: string): string {
  if (!parsed) return raw;
  if (parsed.finalAnswer.trim()) return parsed.finalAnswer;
  return parsed.fallbackMarkdown;
}
