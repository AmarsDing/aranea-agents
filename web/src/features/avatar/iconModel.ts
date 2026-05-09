/** 可作为 Quasar Material icon `name` 的字段（排除路径、URL 片段等误写入）。 */
function isPlausibleMaterialIconName(v: string): boolean {
  if (!v || v.length > 96) return false;
  if (/^(https?:|data:|blob:)/i.test(v)) return false;
  // 库内头像 key 常为 avatar_* / snake_case，勿当成 Material 图标名
  if (/^avatar_/i.test(v)) return false;
  return /^[a-z0-9]+(_[a-z0-9]+)*$/i.test(v);
}

/** Agent `icon` 库内资源引用判定：`avatar_*`、24 位 hex、标准 UUID 形（与 avatar_assets.id 对齐）。 */

export function isAvatarAssetRef(value: string) {
  const trimmed = String(value || "").trim();
  if (!trimmed) return false;
  if (/^(https?:|data:|blob:)/i.test(trimmed)) return true;
  if (/^avatar_/i.test(trimmed)) return true;
  if (/^[a-f0-9]{24}$/i.test(trimmed)) return true;
  if (/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(trimmed)) return true;
  return false;
}

/** 服务端存储的头像 asset id（走 Store 拉缩略图），不含 http(s)/data/blob，也不是纯 Quasar icon 名。 */
export function isStoredAvatarAssetId(icon: string | undefined | null): boolean {
  const v = String(icon ?? "").trim();
  if (!v || /^(https?:|data:|blob:)/i.test(v)) return false;
  return isAvatarAssetRef(v);
}

/** `q-avatar` 的 `icon`：有库内头像时留空用 `<img>`，否则用 Material 名或默认。 */
export function quasarAvatarIconForAgentField(icon: string | undefined | null): string | undefined {
  if (isStoredAvatarAssetId(icon)) return undefined;
  const v = String(icon ?? "").trim();
  if (!v || !isPlausibleMaterialIconName(v)) return "smart_toy";
  return v;
}

/** 会话气泡等：是否挂载 ResolvedAvatarImg（与 useAvatarThumbnailSrc 解析策略对齐）。 */
export function shouldRenderAgentAvatarImage(icon: string | undefined | null): boolean {
  const v = String(icon ?? "").trim();
  if (!v) return false;
  if (/^(https?:|data:|blob:)/i.test(v)) return true;
  if (isStoredAvatarAssetId(v)) return true;
  if (/^avatar_/i.test(v)) return true;
  return !isPlausibleMaterialIconName(v);
}
