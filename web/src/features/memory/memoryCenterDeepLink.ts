import { resolveMemoryCenterTab } from './memoryCenterTabs';

export type MemoryCenterDeepLink = {
  tab: string;
  layer: '' | 'L0' | 'L1' | 'L2' | 'L3' | 'L4';
  agentId: string | null;
  agentKey: string | null;
  sessionId: string | null;
  factId: string | null;
  keyword: string;
  clearFactStatus: boolean;
};

function queryString(query: Record<string, unknown>, key: string): string {
  const value = query[key];
  return typeof value === 'string' ? value.trim() : '';
}

/** Parse `/memory` query into selection + tab. factId / q win over tab. */
export function parseMemoryCenterDeepLink(
  query: Record<string, unknown>,
  isPlatformAdmin: boolean,
): MemoryCenterDeepLink {
  const tabQ = queryString(query, 'tab');
  let tab = tabQ ? resolveMemoryCenterTab(tabQ, isPlatformAdmin) : 'panorama';
  const layerQ = queryString(query, 'layer').toUpperCase();
  let layer: MemoryCenterDeepLink['layer'] = '';
  if (layerQ === 'L4') {
    tab = 'graph';
    layer = 'L4';
  } else if (layerQ === 'L0' || layerQ === 'L1' || layerQ === 'L2' || layerQ === 'L3') {
    tab = tabQ ? resolveMemoryCenterTab(tabQ, isPlatformAdmin) : 'browse';
    layer = layerQ;
  }

  const factId = queryString(query, 'factId');
  const keywordQ = queryString(query, 'q');
  let keyword = '';
  let clearFactStatus = false;
  if (factId) {
    keyword = factId;
    clearFactStatus = true;
    tab = 'browse';
  } else if (keywordQ) {
    keyword = keywordQ;
    tab = 'browse';
  }

  return {
    tab,
    layer,
    agentId: queryString(query, 'agentId') || null,
    agentKey: queryString(query, 'agentKey') || null,
    sessionId: queryString(query, 'sessionId') || null,
    factId: factId || null,
    keyword,
    clearFactStatus,
  };
}
