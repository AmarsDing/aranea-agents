import type { useAvatarCatalogStore } from "../../stores/avatar";
import { isAvatarAssetRef } from "./iconModel";

type AvatarCatalogStore = ReturnType<typeof useAvatarCatalogStore>;

/**
 * Resolves agent.icon to a catalog asset id for thumbnail fetch.
 * Returns null for empty, external URLs, or unresolved keys.
 */
export async function resolveAvatarAssetFetchId(
  store: AvatarCatalogStore,
  icon: string | undefined | null,
): Promise<string | null> {
  const raw = String(icon ?? "").trim();
  if (!raw || /^(https?:|data:|blob:)/i.test(raw)) return null;
  if (isAvatarAssetRef(raw)) return raw;
  await store.ensureAgentsCatalog();
  const hit = store.agentsCatalog.find((a) => a.id === raw || (a.key && a.key === raw));
  return hit?.id ?? null;
}
