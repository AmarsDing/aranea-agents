/**
 * P3.1 (mobile pairing): QR payload format shared by the desktop pairing
 * dialog (encode) and the mobile server-setup page (decode).
 *
 * Canonical content is marker JSON: `{"aranea":"mobile-setup","v":1,"url":...}`.
 * The parser additionally accepts a plain plausible server URL so hand-rolled
 * QR codes keep working. Anything else is rejected so scanning an unrelated
 * QR cannot silently fill the server address.
 */

import { isPlausibleServerUrl } from '../../services/backendConfig';

// Re-exported so presentational components can validate without importing
// `services/` directly (frontend red line #2).
export { isPlausibleServerUrl };

export const PAIRING_QR_MARKER = 'mobile-setup';
export const PAIRING_QR_VERSION = 1;

export function buildPairingQrPayload(url: string): string {
  return JSON.stringify({
    aranea: PAIRING_QR_MARKER,
    v: PAIRING_QR_VERSION,
    url: url.trim(),
  });
}

/** Returns the paired server URL, or null when the scanned text is not ours. */
export function parsePairingQr(text: string): { url: string } | null {
  const trimmed = text.trim();
  if (!trimmed) return null;

  // Lenient path: a bare server URL (hand-rolled QR).
  if (isPlausibleServerUrl(trimmed)) return { url: trimmed };

  let parsed: unknown;
  try {
    parsed = JSON.parse(trimmed);
  } catch {
    return null;
  }
  if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) return null;
  const obj = parsed as Record<string, unknown>;
  if (obj.aranea !== PAIRING_QR_MARKER) return null;
  if (obj.v !== PAIRING_QR_VERSION) return null;
  if (typeof obj.url !== 'string') return null;
  const url = obj.url.trim();
  if (!isPlausibleServerUrl(url)) return null;
  return { url };
}
