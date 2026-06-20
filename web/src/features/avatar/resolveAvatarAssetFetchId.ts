import type { useAvatarCatalogStore } from '../../stores/avatar';

type AvatarCatalogStore = ReturnType<typeof useAvatarCatalogStore>;

// 24 位 hex（MongoDB ObjectId 风格，avatar_assets.id 的格式）
const HEX_ID_RE = /^[a-f0-9]{24}$/i;
// UUID 格式
const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

/**
 * Resolves agent.icon to a catalog asset id for thumbnail fetch.
 * Returns null for empty, external URLs, or unresolved keys.
 *
 * 内置头像（avatar_career_01 等）的 id 与 asset_key 相同，可直接使用；
 * 上传头像的 id 是 24 位 hex，asset_key 是 `upload-<id>`，需查 catalog 转换。
 * 查 catalog 失败时回退到原值，避免阻塞内置头像显示。
 */
export async function resolveAvatarAssetFetchId(
  store: AvatarCatalogStore,
  icon: string | undefined | null,
): Promise<string | null> {
  const raw = String(icon ?? '').trim();
  if (!raw || /^(https?:|data:|blob:)/i.test(raw)) return null;
  // 24 位 hex / UUID / avatar_* / channel_* 均可直接作为 id（内置头像 id = key）
  if (HEX_ID_RE.test(raw) || UUID_RE.test(raw) || /^avatar_/i.test(raw) || /^channel_/i.test(raw)) {
    return raw;
  }
  // 其他格式（如 upload-<id>）：尝试查 catalog 获取真正的 id
  try {
    await store.ensureAgentsCatalog();
    const hit = store.agentsCatalog.find((a) => a.id === raw || (a.key && a.key === raw));
    return hit?.id ?? null;
  } catch {
    return null;
  }
}
