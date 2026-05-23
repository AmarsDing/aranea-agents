/** Message / session source for UserBubble badges (M55 CC-B-07). */

export type MessageSourceKind = "web" | "channel" | "cron" | "a2a" | "api" | "";

export type MessageSourceMeta = {
  source: MessageSourceKind;
  platform?: string;
  channelKey?: string;
};

export function parseMessageSourceMeta(optionsJson?: string): MessageSourceMeta | null {
  const raw = (optionsJson ?? "").trim();
  if (!raw) return null;
  try {
    const o = JSON.parse(raw) as {
      source?: string;
      channel?: string;
      platform?: string;
      channel_key?: string;
    };
    const source = (o.source ?? "").trim().toLowerCase() as MessageSourceKind;
    if (!source) return null;
    return {
      source,
      platform: (o.platform ?? o.channel ?? "").trim() || undefined,
      channelKey: (o.channel_key ?? "").trim() || undefined,
    };
  } catch {
    return null;
  }
}

export function messageSourceChipKey(meta: MessageSourceMeta | null): string {
  if (!meta?.source) return "";
  if (meta.source === "channel") {
    const p = (meta.platform ?? "").toLowerCase();
    if (p === "feishu" || p === "lark") return "chat.source.feishu";
    if (p === "dingtalk") return "chat.source.dingtalk";
    if (p === "wecom") return "chat.source.wecom";
    return "chat.source.channel";
  }
  return `chat.source.${meta.source}`;
}

export function messageSourceChipFallback(meta: MessageSourceMeta | null): string {
  if (!meta?.source) return "";
  switch (meta.source) {
    case "web":
      return "Web";
    case "channel":
      return meta.platform ? `Channel · ${meta.platform}` : "Channel";
    case "cron":
      return "Cron";
    case "a2a":
      return "A2A";
    case "api":
      return "API";
    default:
      return meta.source;
  }
}
