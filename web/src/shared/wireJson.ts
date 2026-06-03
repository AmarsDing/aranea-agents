/** Kratos 返回的 proto JSON 多为 snake_case；兼容 camelCase（若生成器或网关切换命名）。 */

export function asRecord(o: unknown): Record<string, unknown> {
  return o != null && typeof o === 'object' && !Array.isArray(o) ? (o as Record<string, unknown>) : {};
}

export function pickStr(r: Record<string, unknown>, snake: string, camel: string): string {
  const v = r[snake] ?? r[camel];
  return v == null ? '' : String(v);
}

export function pickNum(r: Record<string, unknown>, snake: string, camel: string): number {
  const v = r[snake] ?? r[camel];
  if (typeof v === 'number' && !Number.isNaN(v)) return v;
  if (typeof v === 'boolean') return v ? 1 : 0;
  if (typeof v === 'string' && v.trim() !== '') return Number(v);
  return 0;
}

export function pickOptionalI32(r: Record<string, unknown>, snake: string, camel: string): number | undefined {
  const v = r[snake] ?? r[camel];
  if (v === undefined || v === null) return undefined;
  if (typeof v === 'number' && !Number.isNaN(v)) return Math.trunc(v);
  if (typeof v === 'string' && v.trim() !== '') return Math.trunc(Number(v));
  return undefined;
}

export function pickI32(r: Record<string, unknown>, snake: string, camel: string): number {
  return Math.trunc(pickNum(r, snake, camel));
}

export function pickBool(r: Record<string, unknown>, snake: string, camel: string): boolean {
  const v = r[snake] ?? r[camel];
  return Boolean(v);
}

/** Like pickBool, but omitted / null counts as true (proto JSON often omits default true). */
export function pickOptionalBoolDefaultTrue(r: Record<string, unknown>, snake: string, camel: string): boolean {
  const v = r[snake] ?? r[camel];
  if (v === undefined || v === null) return true;
  return Boolean(v);
}

export function pickStrArray(r: Record<string, unknown>, snake: string, camel: string): string[] {
  const v = r[snake] ?? r[camel];
  if (!Array.isArray(v)) return [];
  return v.filter((x): x is string => typeof x === 'string');
}

export function pickI64(r: Record<string, unknown>, snake: string, camel: string): number {
  const v = r[snake] ?? r[camel];
  if (typeof v === 'number' && !Number.isNaN(v)) return v;
  if (typeof v === 'string' && v.trim() !== '') return Number(v);
  return 0;
}

export function parseJsonArray(json: string): string[] {
  if (!json || json === '[]' || json === '') return [];
  try {
    const parsed = JSON.parse(json);
    if (Array.isArray(parsed)) return parsed.filter((x): x is string => typeof x === 'string');
  } catch {
    /* ignore */
  }
  return [];
}

export function mapStringFloat(raw: unknown): Record<string, number> {
  const o = asRecord(raw);
  const out: Record<string, number> = {};
  for (const [k, v] of Object.entries(o)) {
    if (typeof v === 'number' && !Number.isNaN(v)) out[k] = v;
    else if (typeof v === 'string' && v.trim() !== '') out[k] = Number(v);
    else if (typeof v === 'number') out[k] = 0;
  }
  return out;
}
