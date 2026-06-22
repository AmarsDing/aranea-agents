import type { HookActionType } from '../../features/hooks/types';

export function isToolCallbackPoint(point: string) {
  return point === 'before_tool' || point === 'after_tool';
}

export function isOnEventPoint(point: string) {
  return point === 'on_event';
}

const ACTION_TYPE_I18N_KEYS: Record<HookActionType, string> = {
  log: 'hooksPage.actionTypes.log',
  notify: 'hooksPage.actionTypes.notify',
  block: 'hooksPage.actionTypes.block',
  modify: 'hooksPage.actionTypes.modify',
};

export function actionTypeLabel(type: HookActionType, t: (key: string) => string) {
  return t(ACTION_TYPE_I18N_KEYS[type]);
}

export function actionTagClass(type: HookActionType) {
  if (type === 'block') return 'hook-tag hook-tag--block';
  if (type === 'notify') return 'hook-tag hook-tag--notify';
  if (type === 'modify') return 'hook-tag hook-tag--modify';
  return 'hook-tag hook-tag--log';
}

export function stringifyModifyPatch(patch: Record<string, unknown> | undefined) {
  return JSON.stringify(patch ?? {}, null, 2);
}

export function parseModifyPatchText(
  raw: string | number | null,
  t: (key: string) => string,
): { patch: Record<string, unknown>; error: string } {
  const text = String(raw ?? '').trim();
  if (!text) {
    return { patch: {}, error: '' };
  }
  try {
    const parsed = JSON.parse(text);
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return { patch: {}, error: t('hooksPage.callbackEditor.modifyPatchInvalidObject') };
    }
    return { patch: parsed as Record<string, unknown>, error: '' };
  } catch {
    return { patch: {}, error: t('hooksPage.callbackEditor.modifyPatchInvalidJson') };
  }
}
