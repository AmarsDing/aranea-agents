/** 平台默认头像 asset_key，与后端 `channel_avatar_specs.go` 一致：channel_<type>（- → _） */
export function defaultChannelAvatarKey(type: string): string {
  const normalized = String(type || "").trim().replace(/-/g, "_");
  return normalized ? `channel_${normalized}` : "";
}
