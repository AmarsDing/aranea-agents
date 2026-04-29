/** Agent `icon` 字段与头像资源 id 的纯规则（无 HTTP、无 Store） */

export function isAvatarAssetRef(value: string) {
  const trimmed = String(value || "").trim();
  if (!trimmed) return false;
  if (/^(https?:|data:|blob:)/i.test(trimmed)) return true;
  return /^avatar_/i.test(trimmed) || /^[a-f0-9]{24}$/i.test(trimmed);
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
  return v || "smart_toy";
}

/** 是否应用 `<img>` / ResolvedAvatarImg（外链、data URL、blob 或库内 asset id）。 */
export function shouldRenderAgentAvatarImage(icon: string | undefined | null): boolean {
  const v = String(icon ?? "").trim();
  if (!v) return false;
  if (/^(https?:|data:|blob:)/i.test(v)) return true;
  return isStoredAvatarAssetId(v);
}
