/**
 * useMediaUrl resolves MediaArtifact.url for rendering.
 *
 * Media tools persist generated media into the artifact store and emit
 * "artifact://<artifact_id>" URLs (see internal/provider/media/persist.go).
 * Signed download URLs have a TTL, so the scheme keeps a stable reference in
 * historical messages; the frontend exchanges it for a fresh signed URL right
 * before rendering. Remote http(s) URLs pass through unchanged (covers
 * best-effort degraded results and legacy data).
 */
import { reactive } from 'vue';
import { artifactDownloadHref, signDownloadUrl } from '../artifact/api';
import type { MediaArtifact } from './mediaTypes';

/** URL scheme marking persisted media artifacts. */
export const ARTIFACT_URL_SCHEME = 'artifact://';

export function isArtifactUrl(url: string): boolean {
  return url.startsWith(ARTIFACT_URL_SCHEME);
}

/** Transparent 1x1 placeholder shown while an artifact:// URL is being signed. */
const PLACEHOLDER_SRC = 'data:image/gif;base64,R0lGODlhAQABAAAAACH5BAEKAAEALAAAAAABAAEAAAICTAEAOw==';

export type MediaSignFn = (artifactId: string) => Promise<string>;

async function defaultSign(artifactId: string): Promise<string> {
  const signed = await signDownloadUrl(artifactId);
  return signed.url ? artifactDownloadHref(signed.url) : '';
}

function keyOf(art: MediaArtifact): string {
  if (art.artifact_id) return art.artifact_id;
  return art.url.slice(ARTIFACT_URL_SCHEME.length);
}

export function useMediaUrl(sign: MediaSignFn = defaultSign) {
  const resolved = reactive(new Map<string, string>());
  const inflight = new Map<string, Promise<void>>();

  /**
   * Resolve an artifact:// URL to a signed URL (cached per artifact id).
   * No-op for http(s) URLs. Failures fall back to the original URL.
   */
  function resolve(art: MediaArtifact): Promise<void> {
    if (!isArtifactUrl(art.url)) return Promise.resolve();
    const key = keyOf(art);
    if (!key || resolved.has(key)) return Promise.resolve();
    const existing = inflight.get(key);
    if (existing) return existing;
    const p = (async () => {
      try {
        const url = await sign(key);
        resolved.set(key, url || art.url);
      } catch {
        // Degrade to the original URL; the element renders broken media,
        // matching the behavior of any other dead link.
        resolved.set(key, art.url);
      } finally {
        inflight.delete(key);
      }
    })();
    inflight.set(key, p);
    return p;
  }

  /**
   * Template binding helper: returns the URL to render for an artifact.
   * http(s) URLs pass through; artifact:// URLs return the cached signed URL,
   * or a transparent placeholder while signing is in flight.
   */
  function mediaSrc(art: MediaArtifact): string {
    if (!isArtifactUrl(art.url)) return art.url;
    const hit = resolved.get(keyOf(art));
    if (hit) return hit;
    void resolve(art);
    return PLACEHOLDER_SRC;
  }

  return { mediaSrc, resolve };
}
