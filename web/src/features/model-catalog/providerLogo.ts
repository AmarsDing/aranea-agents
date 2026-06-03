import { kratosApi } from '../../services/axiosHandler';
import { createModelCatalogService } from '../../services/index';

const api = createModelCatalogService();

const svgCache = new Map<string, string>();

/** Local API path for a cached provider logo (requires auth via kratosApi for fetch). */
export function catalogProviderLogoPath(providerId: string): string {
  const id = providerId.trim();
  if (!id) return '';
  return `/v1/model-catalog/logos/${encodeURIComponent(id)}`;
}

export async function fetchCatalogProviderLogoSvg(providerId: string): Promise<string> {
  const id = providerId.trim();
  if (!id) return '';
  const hit = svgCache.get(id);
  if (hit !== undefined) return hit;
  try {
    const res = await api.GetCatalogProviderLogo({ providerId: id });
    const svg = res.cached && res.svg ? res.svg : '';
    svgCache.set(id, svg);
    return svg;
  } catch {
    svgCache.set(id, '');
    return '';
  }
}

/** Fetch logo SVG via axios (includes session credentials). */
export async function fetchProviderLogoSvg(providerId: string): Promise<string> {
  const id = providerId.trim();
  if (!id) return '';
  const hit = svgCache.get(id);
  if (hit !== undefined) return hit;
  const path = catalogProviderLogoPath(id);
  if (!path) return '';
  try {
    const res = await kratosApi.get<{ svg?: string; cached?: boolean }>(path);
    const svg = res.data?.cached && res.data?.svg ? res.data.svg : '';
    svgCache.set(id, svg);
    return svg;
  } catch {
    svgCache.set(id, '');
    return '';
  }
}

export function clearProviderLogoCache() {
  svgCache.clear();
}
