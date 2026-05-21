/** Resolve A2UI container children (explicitList + template). */

import { resolveDataPathValue } from "./a2uiBind";
import type { A2UISurfaceState } from "./a2uiSurfaceState";

export function resolveA2UIChildIds(
  children: unknown,
  surface: A2UISurfaceState
): string[] {
  if (!children || typeof children !== "object") return [];
  const ch = children as Record<string, unknown>;
  const explicit = ch.explicitList;
  if (Array.isArray(explicit)) {
    return explicit.map(String).filter((id) => Boolean(surface.components[id]));
  }
  const template = ch.template;
  if (!template || typeof template !== "object") return [];
  const tpl = template as { componentId?: string; dataBinding?: string };
  const binding = String(tpl.dataBinding ?? "").trim();
  if (!binding) return [];
  const raw = resolveDataPathValue(surface.dataModel, binding);
  if (Array.isArray(raw)) {
    return raw.map(String).filter((id) => Boolean(surface.components[id]));
  }
  if (raw && typeof raw === "object" && !Array.isArray(raw)) {
    return Object.values(raw as Record<string, unknown>)
      .map(String)
      .filter((id) => Boolean(surface.components[id]));
  }
  return [];
}
