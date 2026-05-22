export type ChannelSessionMeta = {
  source?: string;
  channel_id?: string;
  channel_key?: string;
  platform?: string;
  peer_id?: string;
  peer_key?: string;
  receive_mode?: string;
};

export function parseChannelSessionMeta(metadataJson?: string): ChannelSessionMeta | null {
  const raw = (metadataJson ?? "").trim();
  if (!raw) return null;
  try {
    const meta = JSON.parse(raw) as ChannelSessionMeta;
    if (meta?.source !== "channel") return null;
    return meta;
  } catch {
    return null;
  }
}

export function isChannelSession(metadataJson?: string, title?: string): boolean {
  if (parseChannelSessionMeta(metadataJson)) return true;
  const t = (title ?? "").trim().toLowerCase();
  return t.startsWith("feishu:") || t.startsWith("lark:") || t.startsWith("dingtalk:");
}
