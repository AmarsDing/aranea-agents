import type { ChannelCatalogItem } from "./types";

/** MuseBot ConfigForm 风格：snake_case 字段名 + 分区标题 */
export type ChannelPlatformFieldKind =
  | "text"
  | "password"
  | "select"
  | "toggle"
  | "textarea";

export type ChannelPlatformFieldBind =
  | { source: "form"; key: "name" | "key" | "description" | "enabled" }
  | { source: "credential"; key: string }
  | { source: "config"; key: string }
  | { source: "routing"; key: "external_id" }
  | { source: "webhook"; key: "receive_mode" | "path" | "public_origin" | "preview" }
  | { source: "feishu"; key: "app_id" | "region" }
  | { source: "icon"; key: "asset_id" };

export type ChannelPlatformField = {
  museKey: string;
  bind: ChannelPlatformFieldBind;
  kind: ChannelPlatformFieldKind;
  required?: boolean;
  placeholder?: string;
  hint?: string;
  options?: { label: string; value: string }[];
  /** 仅在特定 receive_mode 下显示 */
  showWhenReceiveMode?: string[];
  /** 编辑模式隐藏（如 name/key 仍显示） */
  hideOnCreate?: boolean;
};

export type ChannelPlatformSection = {
  id: string;
  /** MuseBot 大写分区名 */
  title: string;
  hint?: string;
  fields: ChannelPlatformField[];
};

const FEISHU_REGION_OPTIONS = [
  { label: "飞书（国内 open.feishu.cn）", value: "feishu" },
  { label: "Lark（国际 open.larksuite.com）", value: "lark" }
];

const RECEIVE_MODE_LABELS: Record<string, string> = {
  webhook: "webhook",
  websocket: "websocket",
  stream: "stream",
  event: "event",
  socket_mode: "socket_mode",
  polling: "polling",
  gateway: "gateway",
  onebot: "onebot"
};

function credentialFieldsFromCatalog(catalog: ChannelCatalogItem | null): ChannelPlatformField[] {
  const schema = catalog?.credential_schema;
  const props = schema?.properties as Record<string, { title?: string; format?: string; "x-required"?: boolean }> | undefined;
  if (!props) return [];
  const required = new Set(schema?.required ?? []);
  return Object.entries(props).map(([key, meta]) => ({
    museKey: meta.title ?? key,
    bind: { source: "credential" as const, key },
    kind: (meta.format === "password" ? "password" : "text") as ChannelPlatformFieldKind,
    required: required.has(key) || Boolean(meta["x-required"])
  }));
}

function baseFields(type: string, catalog: ChannelCatalogItem | null): ChannelPlatformField[] {
  const creds = catalog?.credential_schema?.required ?? [];
  const fields: ChannelPlatformField[] = [
    { museKey: "channel_name", bind: { source: "form", key: "name" }, kind: "text", required: true },
    { museKey: "channel_key", bind: { source: "form", key: "key" }, kind: "text", required: true, placeholder: `${type}_main` },
    { museKey: "description", bind: { source: "form", key: "description" }, kind: "textarea" },
    { museKey: "enabled", bind: { source: "form", key: "enabled" }, kind: "toggle" }
  ];

  switch (type) {
    case "feishu":
      fields.push(
        { museKey: "lark_app_id", bind: { source: "feishu", key: "app_id" }, kind: "text", required: true, placeholder: "cli_xxxxxxxx" },
        { museKey: "lark_app_secret", bind: { source: "credential", key: "app_secret" }, kind: "password", required: true },
        { museKey: "lark_region", bind: { source: "feishu", key: "region" }, kind: "select", options: FEISHU_REGION_OPTIONS }
      );
      break;
    case "dingtalk":
      fields.push(
        { museKey: "ding_client_id", bind: { source: "config", key: "client_id" }, kind: "text" },
        { museKey: "ding_client_secret", bind: { source: "credential", key: "client_secret" }, kind: "password" },
        { museKey: "secret", bind: { source: "credential", key: "secret" }, kind: "password", required: creds.includes("secret") }
      );
      break;
    case "wecom":
    case "wecom-app":
      fields.push(
        { museKey: "com_wechat_token", bind: { source: "credential", key: "token" }, kind: "password", required: true },
        { museKey: "com_wechat_encoding_aes_key", bind: { source: "credential", key: "encoding_aes_key" }, kind: "password" },
        { museKey: "com_wechat_corp_id", bind: { source: "config", key: "corp_id" }, kind: "text" },
        { museKey: "com_wechat_secret", bind: { source: "credential", key: "corp_secret" }, kind: "password" },
        { museKey: "com_wechat_agent_id", bind: { source: "config", key: "agent_id" }, kind: "text" }
      );
      break;
    case "wechat":
      fields.push(
        { museKey: "wechat_app_id", bind: { source: "config", key: "app_id" }, kind: "text" },
        { museKey: "wechat_app_secret", bind: { source: "credential", key: "app_secret" }, kind: "password", required: true },
        { museKey: "wechat_token", bind: { source: "credential", key: "token" }, kind: "password" },
        { museKey: "wechat_encoding_aes_key", bind: { source: "credential", key: "encoding_aes_key" }, kind: "password" },
        { museKey: "wechat_active", bind: { source: "config", key: "active_mode" }, kind: "toggle", hint: "开启后走客服 API 主动回复" }
      );
      break;
    case "slack":
      fields.push(
        { museKey: "slack_bot_token", bind: { source: "credential", key: "bot_token" }, kind: "password", required: true },
        { museKey: "slack_app_token", bind: { source: "credential", key: "app_token" }, kind: "password", hint: "Socket Mode 必填" },
        { museKey: "signing_secret", bind: { source: "credential", key: "signing_secret" }, kind: "password", required: true }
      );
      break;
    case "telegram":
      fields.push(
        { museKey: "telegram_bot_token", bind: { source: "credential", key: "bot_token" }, kind: "password", required: true }
      );
      break;
    case "discord":
      fields.push(
        { museKey: "discord_bot_token", bind: { source: "credential", key: "bot_token" }, kind: "password", required: true }
      );
      break;
    case "qq":
      fields.push(
        { museKey: "qq_app_id", bind: { source: "config", key: "app_id" }, kind: "text" },
        { museKey: "qq_app_secret", bind: { source: "credential", key: "app_secret" }, kind: "password", required: true }
      );
      break;
    case "personal_qq":
      fields.push(
        { museKey: "qq_one_bot_receive_token", bind: { source: "credential", key: "receive_token" }, kind: "password", required: true },
        { museKey: "qq_one_bot_send_token", bind: { source: "credential", key: "send_token" }, kind: "password", required: true },
        { museKey: "qq_one_bot_http_server", bind: { source: "config", key: "onebot_http_server" }, kind: "text", placeholder: "http://127.0.0.1:3000" }
      );
      break;
    default:
      fields.push(...credentialFieldsFromCatalog(catalog));
      creds.forEach((key) => {
        if (fields.some((f) => f.bind.source === "credential" && f.bind.key === key)) return;
        fields.push({
          museKey: key,
          bind: { source: "credential", key },
          kind: "password",
          required: true
        });
      });
  }

  return fields;
}

function connectionFields(catalog: ChannelCatalogItem | null): ChannelPlatformField[] {
  if (!catalog?.receive_modes?.length) return [];

  const modeOptions = catalog.receive_modes.map((v) => ({
    label: RECEIVE_MODE_LABELS[v] ?? v,
    value: v
  }));

  const fields: ChannelPlatformField[] = [
    {
      museKey: "receive_mode",
      bind: { source: "webhook", key: "receive_mode" },
      kind: "select",
      options: modeOptions
    }
  ];

  if (catalog.supports_webhook) {
    fields.push(
      {
        museKey: "webhook_path",
        bind: { source: "webhook", key: "path" },
        kind: "text",
        showWhenReceiveMode: ["webhook", "event", "onebot"]
      },
      {
        museKey: "public_webhook_origin",
        bind: { source: "webhook", key: "public_origin" },
        kind: "text",
        placeholder: "https://your-domain.com",
        showWhenReceiveMode: ["webhook", "event", "onebot"]
      },
      {
        museKey: "webhook_url_preview",
        bind: { source: "webhook", key: "preview" },
        kind: "text",
        showWhenReceiveMode: ["webhook", "event", "onebot"]
      }
    );
  }

  return fields;
}

function routingFields(type: string): ChannelPlatformField[] {
  const fields: ChannelPlatformField[] = [];
  if (type !== "feishu") {
    fields.push({
      museKey: "external_id",
      bind: { source: "routing", key: "external_id" },
      kind: "text"
    });
  }
  fields.push(
    { museKey: "allowed_user_ids", bind: { source: "config", key: "allowed_user_ids" }, kind: "textarea", hint: "允许发消息的用户 ID，JSON 数组或逗号分隔；留空=不限制。飞书填 open_id（ou_xxx）或 user_id" },
    { museKey: "allowed_group_ids", bind: { source: "config", key: "allowed_group_ids" }, kind: "textarea", hint: "允许响应的群 chat_id（飞书 oc_xxx），JSON 或逗号分隔；留空=不限制。单聊不受此字段约束" },
    { museKey: "require_mention", bind: { source: "config", key: "require_mention" }, kind: "checkbox", hint: "群聊需 @ 机器人才响应（飞书/钉钉等）" }
  );
  return fields;
}

/** 按 MuseBot ConfigForm 分区返回当前平台的表单结构 */
export function buildPlatformSections(type: string, catalog: ChannelCatalogItem | null): ChannelPlatformSection[] {
  const sections: ChannelPlatformSection[] = [
    {
      id: "base",
      title: "BASE",
      hint: "实例标识与平台凭据（字段名对齐 MuseBot snake_case）",
      fields: baseFields(type, catalog)
    }
  ];

  const conn = connectionFields(catalog);
  if (conn.length) {
    sections.push({
      id: "connection",
      title: "CONNECTION",
      hint: catalog?.supports_webhook ? "Webhook / 长连接接入方式" : "长连接接入方式",
      fields: conn
    });
  }

  sections.push({
    id: "routing",
    title: "ROUTING",
    hint: "消息路由与访问控制",
    fields: routingFields(type)
  });

  sections.push({
    id: "avatar",
    title: "AVATAR",
    fields: [{ museKey: "icon_asset_id", bind: { source: "icon", key: "asset_id" }, kind: "text", hint: "留空使用平台默认图标" }]
  });

  return sections;
}

/** 首屏可见字段（不含高级 JSON） */
export function visibleFields(section: ChannelPlatformSection, receiveMode: string): ChannelPlatformField[] {
  return section.fields.filter((field) => {
    if (!field.showWhenReceiveMode?.length) return true;
    return field.showWhenReceiveMode.includes(receiveMode);
  });
}

/** Prefer catalog credential_schema.properties when platform switch lacks explicit credential rows. */
export function catalogCredentialFields(catalog: ChannelCatalogItem | null): ChannelPlatformField[] {
  return credentialFieldsFromCatalog(catalog);
}
