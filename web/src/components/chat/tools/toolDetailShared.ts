/**
 * Shared helpers for tool-detail components (web/src/components/chat/tools/).
 *
 * Tool detail components receive a v2 `Step` whose `ToolArgs` / `ToolResult`
 * are already-parsed JSON values (`unknown | null`). These helpers provide
 * small typed accessors and a formatter for display.
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

/**
 * Format a parsed tool argument/result value for display as a JSON string.
 * Returns empty string for null/undefined; passes strings through unchanged.
 */
export function formatToolData(v: unknown): string {
  if (v == null) return '';
  if (typeof v === 'string') return v;
  try {
    return JSON.stringify(v, null, 2);
  } catch {
    return String(v);
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

/**
 * P1 会话产物点击查看（2026-09-01）：从工具结果中提取 artifact 引用。
 * officecli_render 等工具在结果里输出 artifact_id / file（产物落盘后
 * 的下载指针）；媒体工具结果由 MediaToolDetail 内联预览，调用方负责排除。
 */
export function extractArtifactRef(toolResult: unknown): { id: string; name?: string } | null {
  const result = asRecord(toolResult);
  if (!result) return null;
  const id = asString(result.artifact_id)?.trim();
  if (!id) return null;
  const name = asString(result.file)?.trim();
  return name ? { id, name } : { id };
}

/** Coerce to a finite number. Returns undefined for non-numbers. */
export function asNumber(v: unknown): number | undefined {
  return typeof v === 'number' && Number.isFinite(v) ? v : undefined;
}

/** Truncate a string to `max` chars, appending an ellipsis when truncated. */
export function truncate(s: string, max = 80): string {
  return s.length > max ? `${s.slice(0, max)}…` : s;
}
