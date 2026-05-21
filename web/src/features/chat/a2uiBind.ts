/** Resolve A2UI literalString / path bindings against a surface data model. */

export type A2UIBoundValue = string | number | boolean | undefined;

export function resolveA2UIBind(
  bind: unknown,
  dataModel: Record<string, unknown>
): A2UIBoundValue {
  if (!bind || typeof bind !== "object") return undefined;
  const o = bind as Record<string, unknown>;
  if (typeof o.literalString === "string") return o.literalString;
  if (typeof o.literalNumber === "number") return o.literalNumber;
  if (typeof o.literalBoolean === "boolean") return o.literalBoolean;
  if (typeof o.path === "string") return resolveDataPath(dataModel, o.path);
  return undefined;
}

export function resolveDataPathValue(model: Record<string, unknown>, path: string): unknown {
  const p = path.trim();
  if (!p || p === "/") return model;
  const parts = p.split("/").filter(Boolean);
  let cur: unknown = model;
  for (const part of parts) {
    if (cur == null || typeof cur !== "object") return undefined;
    cur = (cur as Record<string, unknown>)[part];
  }
  return cur;
}

export function resolveDataPath(model: Record<string, unknown>, path: string): A2UIBoundValue {
  const cur = resolveDataPathValue(model, path);
  if (typeof cur === "string" || typeof cur === "number" || typeof cur === "boolean") {
    return cur;
  }
  return undefined;
}

/** Build action.context object from Button action.context array. */
export function resolveActionContext(
  contextArr: unknown,
  dataModel: Record<string, unknown>
): Record<string, unknown> {
  if (!Array.isArray(contextArr)) return {};
  const out: Record<string, unknown> = {};
  for (const entry of contextArr) {
    if (!entry || typeof entry !== "object") continue;
    const e = entry as { key?: string; value?: unknown };
    const key = String(e.key ?? "").trim();
    if (!key) continue;
    const v = resolveA2UIBind(e.value, dataModel);
    if (v !== undefined) out[key] = v;
  }
  return out;
}
