/**
 * Shared helpers for tool-detail components (web/src/components/chat/tools/).
 *
 * Tool detail components receive an ActionEvent with `tool.arguments` /
 * `tool.result` as JSON strings. These helpers parse the JSON safely and
 * provide small typed accessors used across the detail components.
 */

/** Safely parse a JSON string. Returns undefined on null/empty/parse failure. */
export function tryParseJson(s: string | null | undefined): unknown {
  if (!s) return undefined;
  try {
    return JSON.parse(s);
  } catch {
    return undefined;
  }
}

/** Coerce to a plain object (Record). Returns undefined for non-objects/arrays. */
export function asRecord(v: unknown): Record<string, unknown> | undefined {
  if (v && typeof v === 'object' && !Array.isArray(v)) {
    return v as Record<string, unknown>;
  }
  return undefined;
}

/** Coerce to an array. Returns undefined for non-arrays. */
export function asArray(v: unknown): unknown[] | undefined {
  return Array.isArray(v) ? v : undefined;
}

/** Coerce to a string. Returns undefined for non-strings. */
export function asString(v: unknown): string | undefined {
  return typeof v === 'string' ? v : undefined;
}

/** Coerce to a finite number. Returns undefined for non-numbers. */
export function asNumber(v: unknown): number | undefined {
  return typeof v === 'number' && Number.isFinite(v) ? v : undefined;
}

/** Truncate a string to `max` chars, appending an ellipsis when truncated. */
export function truncate(s: string, max = 80): string {
  return s.length > max ? `${s.slice(0, max)}…` : s;
}
