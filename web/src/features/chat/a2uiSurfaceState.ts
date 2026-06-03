/** Accumulate A2UI surface state from server-to-client JSONL messages. */

import type { A2UIParseLine } from './a2uiParse';

export type A2UIComponentRecord = {
  id: string;
  component: Record<string, unknown>;
  weight?: number;
};

export type A2UISurfaceState = {
  surfaceId: string;
  rootId: string;
  components: Record<string, A2UIComponentRecord>;
  dataModel: Record<string, unknown>;
  ready: boolean;
  deleted: boolean;
};

const emptySurface = (): A2UISurfaceState => ({
  surfaceId: '',
  rootId: '',
  components: {},
  dataModel: {},
  ready: false,
  deleted: false,
});

function applyDataModelContents(
  model: Record<string, unknown>,
  path: string | undefined,
  contents: unknown,
): Record<string, unknown> {
  const next = { ...model };
  const p = (path ?? '').trim();
  if (!Array.isArray(contents)) return next;
  const map: Record<string, unknown> = {};
  for (const item of contents) {
    if (!item || typeof item !== 'object') continue;
    const row = item as Record<string, unknown>;
    const key = String(row.key ?? '').trim();
    if (!key) continue;
    if (row.valueString !== undefined) map[key] = row.valueString;
    else if (row.valueNumber !== undefined) map[key] = row.valueNumber;
    else if (row.valueBoolean !== undefined) map[key] = row.valueBoolean;
    else if (Array.isArray(row.valueMap)) {
      const nested: Record<string, unknown> = {};
      for (const ent of row.valueMap) {
        if (!ent || typeof ent !== 'object') continue;
        const e = ent as Record<string, unknown>;
        const k = String(e.key ?? '').trim();
        if (!k) continue;
        if (e.valueString !== undefined) nested[k] = e.valueString;
        else if (e.valueNumber !== undefined) nested[k] = e.valueNumber;
        else if (e.valueBoolean !== undefined) nested[k] = e.valueBoolean;
      }
      map[key] = nested;
    }
  }
  if (!p || p === '/') {
    return { ...next, ...map };
  }
  const parts = p.split('/').filter(Boolean);
  if (parts.length === 0) return { ...next, ...map };
  let cursor: Record<string, unknown> = next;
  for (let i = 0; i < parts.length - 1; i++) {
    const seg = parts[i];
    const child = cursor[seg];
    if (!child || typeof child !== 'object' || Array.isArray(child)) {
      cursor[seg] = {};
    }
    cursor = cursor[seg] as Record<string, unknown>;
  }
  cursor[parts[parts.length - 1]] = map;
  return next;
}

function applyServerMessage(state: A2UISurfaceState, payload: Record<string, unknown>): A2UISurfaceState {
  if (payload.beginRendering && typeof payload.beginRendering === 'object') {
    const br = payload.beginRendering as Record<string, unknown>;
    const surfaceId = String(br.surfaceId ?? state.surfaceId);
    const rootId = String(br.root ?? state.rootId);
    return {
      ...state,
      surfaceId,
      rootId,
      ready: Boolean(rootId && surfaceId),
      deleted: false,
    };
  }
  if (payload.surfaceUpdate && typeof payload.surfaceUpdate === 'object') {
    const su = payload.surfaceUpdate as Record<string, unknown>;
    const surfaceId = String(su.surfaceId ?? state.surfaceId);
    const components = { ...state.components };
    const list = su.components;
    if (Array.isArray(list)) {
      for (const item of list) {
        if (!item || typeof item !== 'object') continue;
        const row = item as Record<string, unknown>;
        const id = String(row.id ?? '').trim();
        const comp = row.component;
        if (!id || !comp || typeof comp !== 'object') continue;
        components[id] = {
          id,
          component: comp as Record<string, unknown>,
          weight: typeof row.weight === 'number' ? row.weight : undefined,
        };
      }
    }
    return { ...state, surfaceId, components };
  }
  if (payload.dataModelUpdate && typeof payload.dataModelUpdate === 'object') {
    const dm = payload.dataModelUpdate as Record<string, unknown>;
    const surfaceId = String(dm.surfaceId ?? state.surfaceId);
    return {
      ...state,
      surfaceId,
      dataModel: applyDataModelContents(
        state.dataModel,
        typeof dm.path === 'string' ? dm.path : undefined,
        dm.contents,
      ),
    };
  }
  if (payload.deleteSurface && typeof payload.deleteSurface === 'object') {
    const ds = payload.deleteSurface as Record<string, unknown>;
    return { ...emptySurface(), surfaceId: String(ds.surfaceId ?? ''), deleted: true };
  }
  return state;
}

/** Fold parsed JSONL lines into a single renderable surface (last surface wins). */
export function reduceA2UISurface(lines: A2UIParseLine[]): A2UISurfaceState {
  let state = emptySurface();
  for (const line of lines) {
    if (!line.ok) continue;
    state = applyServerMessage(state, line.payload);
  }
  return state;
}
