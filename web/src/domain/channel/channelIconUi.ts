/** 平台默认头像 asset_key，与后端 embed 的 PNG 文件名对齐（- → _，部分别名需要显式映射）。 */
const CHANNEL_AVATAR_ALIASES: Record<string, string> = {
  teams: 'channel_msteams',
  'wecom-app': 'channel_wecom_app',
  personal_qq: 'channel_qqbot',
};

export function defaultChannelAvatarKey(type: string): string {
  const normalized = String(type || '')
    .trim()
    .replace(/-/g, '_');
  if (!normalized) return '';
  return CHANNEL_AVATAR_ALIASES[normalized] ?? `channel_${normalized}`;
}
