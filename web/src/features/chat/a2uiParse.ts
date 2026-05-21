/** Parse A2UI JSONL lines from assistant content. */

const ALLOWED_KEYS = new Set([
  "beginRendering",
  "surfaceUpdate",
  "dataModelUpdate",
  "deleteSurface",
]);

export type A2UIParseLine =
  | { ok: true; lineNumber: number; raw: string; key: string; payload: Record<string, unknown> }
  | { ok: false; lineNumber: number; raw: string; error: string };

export function contentLooksLikeA2UIJsonl(text: string): boolean {
  const lines = (text || "").split(/\r?\n/).map((l) => l.trim()).filter(Boolean);
  if (lines.length === 0) return false;
  let hits = 0;
  for (const line of lines.slice(0, 5)) {
    if (line.startsWith("```")) return false;
    try {
      const obj = JSON.parse(line) as Record<string, unknown>;
      if (obj && typeof obj === "object" && !Array.isArray(obj)) {
        const keys = Object.keys(obj);
        if (keys.some((k) => ALLOWED_KEYS.has(k))) hits++;
      }
    } catch {
      return false;
    }
  }
  return hits > 0;
}

export function parseA2UIJsonl(text: string): A2UIParseLine[] {
  const lines = (text || "").split(/\r?\n/);
  const out: A2UIParseLine[] = [];
  let n = 0;
  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("```")) continue;
    n += 1;
    try {
      const obj = JSON.parse(trimmed) as Record<string, unknown>;
      if (!obj || typeof obj !== "object" || Array.isArray(obj)) {
        out.push({ ok: false, lineNumber: n, raw: trimmed, error: "须为 JSON 对象" });
        continue;
      }
      const key = Object.keys(obj).find((k) => ALLOWED_KEYS.has(k)) ?? Object.keys(obj)[0] ?? "";
      if (!key || !ALLOWED_KEYS.has(key)) {
        out.push({
          ok: false,
          lineNumber: n,
          raw: trimmed,
          error: `未知消息键；允许: ${[...ALLOWED_KEYS].join(", ")}`,
        });
        continue;
      }
      out.push({ ok: true, lineNumber: n, raw: trimmed, key, payload: obj });
    } catch (e) {
      out.push({
        ok: false,
        lineNumber: n,
        raw: trimmed,
        error: e instanceof Error ? e.message : "JSON 解析失败",
      });
    }
  }
  return out;
}

export function shouldUseA2UIView(plannerKind: string, text: string): boolean {
  const k = plannerKind.trim().toLowerCase();
  if (k === "a2ui") return true;
  if (k === "react") return false;
  return contentLooksLikeA2UIJsonl(text);
}
