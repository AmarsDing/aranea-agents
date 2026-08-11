/** Minimal catalog shape for platform field definitions (decoupled from feature types). */
type ChannelTypeCatalog = {
  credential_schema?: {
    required?: string[];
    [key: string]: unknown;
  };
  receive_modes?: string[];
  supports_webhook?: boolean;
};

import { FIRST_BYTE_TIMEOUT_OPTIONS, PROGRESS_QUIET_OPTIONS, TURN_TIMEOUT_OPTIONS } from './channelLongTaskDefaults';

/** MuseBot ConfigForm 风格：snake_case 字段名 + 分区标题 */
export type ChannelPlatformFieldKind = 'text' | 'password' | 'select' | 'toggle' | 'textarea';

export type ChannelPlatformFieldBind =
  | { source: 'form'; key: 'name' | 'key' | 'description' | 'enabled' }
  | { source: 'credential'; key: string }
  | { source: 'config'; key: string }
  | { source: 'routing'; key: 'external_id' }
  | { source: 'webhook'; key: 'receive_mode' | 'path' | 'public_origin' | 'preview' }
  | { source: 'feishu'; key: 'app_id' | 'region' }
  | { source: 'icon'; key: 'asset_id' };

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

export type ChannelFieldHelp = {
  description: string;
  example?: string;
};

const FEISHU_REGION_OPTIONS = [
  { label: 'channelEditor.feishuRegion.feishu', value: 'feishu' },
  { label: 'channelEditor.feishuRegion.lark', value: 'lark' },
];

const RECEIVE_MODE_LABELS: Record<string, string> = {
  webhook: 'webhook',
  websocket: 'websocket',
  stream: 'stream',
  event: 'event',
  socket_mode: 'socket_mode',
  polling: 'polling',
  gateway: 'gateway',
  onebot: 'onebot',
};

function credentialFieldsFromCatalog(catalog: ChannelTypeCatalog | null): ChannelPlatformField[] {
  const schema = catalog?.credential_schema;
  const props = schema?.properties as
    | Record<string, { title?: string; format?: string; 'x-required'?: boolean }>
    | undefined;
  if (!props) return [];
  const required = new Set(schema?.required ?? []);
  return Object.entries(props).map(([key, meta]) => ({
    museKey: meta.title ?? key,
    bind: { source: 'credential' as const, key },
    kind: (meta.format === 'password' ? 'password' : 'text') as ChannelPlatformFieldKind,
    required: required.has(key) || Boolean(meta['x-required']),
  }));
}

function baseFields(type: string, catalog: ChannelTypeCatalog | null): ChannelPlatformField[] {
  const creds = catalog?.credential_schema?.required ?? [];
  const fields: ChannelPlatformField[] = [
    { museKey: 'channel_name', bind: { source: 'form', key: 'name' }, kind: 'text', required: true },
    {
      museKey: 'channel_key',
      bind: { source: 'form', key: 'key' },
      kind: 'text',
      required: true,
      placeholder: `${type}_main`,
    },
    { museKey: 'description', bind: { source: 'form', key: 'description' }, kind: 'textarea' },
    { museKey: 'enabled', bind: { source: 'form', key: 'enabled' }, kind: 'toggle' },
  ];

  switch (type) {
    case 'feishu':
      fields.push(
        {
          museKey: 'lark_app_id',
          bind: { source: 'feishu', key: 'app_id' },
          kind: 'text',
          required: true,
          placeholder: 'cli_xxxxxxxx',
        },
        {
          museKey: 'lark_app_secret',
          bind: { source: 'credential', key: 'app_secret' },
          kind: 'password',
          required: true,
        },
        {
          museKey: 'lark_region',
          bind: { source: 'feishu', key: 'region' },
          kind: 'select',
          options: FEISHU_REGION_OPTIONS,
        },
      );
      break;
    case 'dingtalk':
      fields.push(
        { museKey: 'ding_client_id', bind: { source: 'config', key: 'client_id' }, kind: 'text' },
        { museKey: 'ding_client_secret', bind: { source: 'credential', key: 'client_secret' }, kind: 'password' },
        {
          museKey: 'secret',
          bind: { source: 'credential', key: 'secret' },
          kind: 'password',
          required: creds.includes('secret'),
        },
      );
      break;
    case 'wecom':
    case 'wecom-app':
      fields.push(
        { museKey: 'com_wechat_token', bind: { source: 'credential', key: 'token' }, kind: 'password', required: true },
        {
          museKey: 'com_wechat_encoding_aes_key',
          bind: { source: 'credential', key: 'encoding_aes_key' },
          kind: 'password',
        },
        { museKey: 'com_wechat_corp_id', bind: { source: 'config', key: 'corp_id' }, kind: 'text' },
        { museKey: 'com_wechat_secret', bind: { source: 'credential', key: 'corp_secret' }, kind: 'password' },
        { museKey: 'com_wechat_agent_id', bind: { source: 'config', key: 'agent_id' }, kind: 'text' },
      );
      break;
    case 'wechat':
      fields.push(
        { museKey: 'wechat_app_id', bind: { source: 'config', key: 'app_id' }, kind: 'text' },
        {
          museKey: 'wechat_app_secret',
          bind: { source: 'credential', key: 'app_secret' },
          kind: 'password',
          required: true,
        },
        { museKey: 'wechat_token', bind: { source: 'credential', key: 'token' }, kind: 'password' },
        {
          museKey: 'wechat_encoding_aes_key',
          bind: { source: 'credential', key: 'encoding_aes_key' },
          kind: 'password',
        },
        {
          museKey: 'wechat_active',
          bind: { source: 'config', key: 'active_mode' },
          kind: 'toggle',
          hint: 'channelEditor.hints.wechatActiveMode',
        },
      );
      break;
    case 'slack':
      fields.push(
        {
          museKey: 'slack_bot_token',
          bind: { source: 'credential', key: 'bot_token' },
          kind: 'password',
          required: true,
        },
    case 'telegram':
      fields.push({
        museKey: 'telegram_bot_token',
        bind: { source: 'credential', key: 'bot_token' },
        kind: 'password',
        required: true,
      });
      break;
    case 'discord':
      fields.push({
        museKey: 'discord_bot_token',
        bind: { source: 'credential', key: 'bot_token' },
        kind: 'password',
        required: true,
      });
      break;
    case 'qq':
      fields.push(
        { museKey: 'qq_app_id', bind: { source: 'config', key: 'app_id' }, kind: 'text' },
        {
          museKey: 'qq_app_secret',
          bind: { source: 'credential', key: 'app_secret' },
          kind: 'password',
          required: true,
        },
      );
      break;
    case 'personal_qq':
      fields.push(
        {
          museKey: 'qq_one_bot_receive_token',
          bind: { source: 'credential', key: 'receive_token' },
          kind: 'password',
          required: true,
        },
        {
          museKey: 'qq_one_bot_send_token',
          bind: { source: 'credential', key: 'send_token' },
          kind: 'password',
          required: true,
        },
        {
          museKey: 'qq_one_bot_http_server',
          bind: { source: 'config', key: 'onebot_http_server' },
          kind: 'text',
          placeholder: 'http://127.0.0.1:3000',
        },
      );
      break;
    default:
      fields.push(...credentialFieldsFromCatalog(catalog));
      creds.forEach((key) => {
        if (fields.some((f) => f.bind.source === 'credential' && f.bind.key === key)) return;
        fields.push({
          museKey: key,
          bind: { source: 'credential', key },
          kind: 'password',
          required: true,
        });
      });
  }

  return fields;
}

function connectionFields(catalog: ChannelTypeCatalog | null): ChannelPlatformField[] {
  if (!catalog?.receive_modes?.length) return [];

  const modeOptions = catalog.receive_modes.map((v) => ({
    label: RECEIVE_MODE_LABELS[v] ?? v,
    value: v,
  }));

  const fields: ChannelPlatformField[] = [
    {
      museKey: 'receive_mode',
      bind: { source: 'webhook', key: 'receive_mode' },
      kind: 'select',
      options: modeOptions,
    },
  ];

  if (catalog.supports_webhook) {
    fields.push(
      {
        museKey: 'webhook_path',
        bind: { source: 'webhook', key: 'path' },
        kind: 'text',
        showWhenReceiveMode: ['webhook', 'event', 'onebot'],
      },
      {
        museKey: 'public_webhook_origin',
        bind: { source: 'webhook', key: 'public_origin' },
        kind: 'text',
        placeholder: 'https://your-domain.com',
        showWhenReceiveMode: ['webhook', 'event', 'onebot'],
      },
      {
        museKey: 'webhook_url_preview',
        bind: { source: 'webhook', key: 'preview' },
        kind: 'text',
        showWhenReceiveMode: ['webhook', 'event', 'onebot'],
      },
    );
  }

  return fields;
}

function routingFields(type: string): ChannelPlatformField[] {
  const fields: ChannelPlatformField[] = [];
  if (type !== 'feishu') {
    fields.push({
      museKey: 'external_id',
      bind: { source: 'routing', key: 'external_id' },
      kind: 'text',
    });
  }
  fields.push(
    {
      museKey: 'allowed_user_ids',
      bind: { source: 'config', key: 'allowed_user_ids' },
      kind: 'textarea',
      hint: 'channelEditor.hints.allowedUserIds',
    },
    {
      museKey: 'allowed_group_ids',
      bind: { source: 'config', key: 'allowed_group_ids' },
      kind: 'textarea',
      hint: 'channelEditor.hints.allowedGroupIds',
    },
    {
      museKey: 'require_mention',
      bind: { source: 'config', key: 'require_mention' },
      kind: 'toggle',
      hint: 'channelEditor.hints.requireMention',
    },
  );
  return fields;
}

const EXECUTION_MODE_OPTIONS = [
  { label: 'channelEditor.executionMode.sync', value: 'sync' },
  { label: 'channelEditor.executionMode.auto', value: 'auto' },
  { label: 'channelEditor.executionMode.async', value: 'async' },
];

const PROGRESS_MODE_OPTIONS = [
  { label: 'channelEditor.progressMode.off', value: 'off' },
  { label: 'channelEditor.progressMode.text', value: 'text' },
  { label: 'channelEditor.progressMode.steps', value: 'steps' },
];

function longTaskFields(): ChannelPlatformField[] {
  return [
    {
      museKey: 'turn_timeout_sec',
      bind: { source: 'config', key: 'turn_timeout_sec' },
      kind: 'select',
      options: TURN_TIMEOUT_OPTIONS,
    },
    {
      museKey: 'first_byte_timeout_sec',
      bind: { source: 'config', key: 'first_byte_timeout_sec' },
      kind: 'select',
      options: FIRST_BYTE_TIMEOUT_OPTIONS,
    },
    {
      museKey: 'execution_mode',
      bind: { source: 'config', key: 'execution_mode' },
      kind: 'select',
      options: EXECUTION_MODE_OPTIONS,
    },
    {
      museKey: 'progress_mode',
      bind: { source: 'config', key: 'progress_mode' },
      kind: 'select',
      options: PROGRESS_MODE_OPTIONS,
    },
    {
      museKey: 'progress_quiet_sec',
      bind: { source: 'config', key: 'progress_quiet_sec' },
      kind: 'select',
      options: PROGRESS_QUIET_OPTIONS,
    },
    {
      museKey: 'ack_message',
      bind: { source: 'config', key: 'ack_message' },
      kind: 'text',
      placeholder: 'channelEditor.placeholders.ackMessage',
    },
    {
      museKey: 'heartbeat_message',
      bind: { source: 'config', key: 'heartbeat_message' },
      kind: 'text',
      placeholder: 'channelEditor.placeholders.heartbeatMessage',
    },
    {
      museKey: 'async_graph_id',
      bind: { source: 'config', key: 'async_graph_id' },
      kind: 'text',
      placeholder: 'graph-uuid',
    },
    {
      museKey: 'async_team_id',
      bind: { source: 'config', key: 'async_team_id' },
      kind: 'text',
      placeholder: 'team-uuid',
    },
    {
      museKey: 'async_cron_task_id',
      bind: { source: 'config', key: 'async_cron_task_id' },
      kind: 'text',
      placeholder: 'cron-task-uuid',
    },
    { museKey: 'streaming_enabled', bind: { source: 'config', key: 'streaming_enabled' }, kind: 'toggle' },
  ];
}

/** 按 MuseBot ConfigForm 分区返回当前平台的表单结构 */
export function buildPlatformSections(type: string, catalog: ChannelTypeCatalog | null): ChannelPlatformSection[] {
  const sections: ChannelPlatformSection[] = [
    {
      id: 'base',
      title: 'BASE',
      hint: 'channelEditor.sectionHints.base',
      fields: baseFields(type, catalog),
    },
  ];

  const conn = connectionFields(catalog);
  if (conn.length) {
    sections.push({
      id: 'connection',
      title: 'CONNECTION',
      hint: catalog?.supports_webhook
        ? 'channelEditor.sectionHints.connectionWebhook'
        : 'channelEditor.sectionHints.connectionLongPoll',
      fields: conn,
    });
  }

  sections.push({
    id: 'routing',
    title: 'ROUTING',
    hint: 'channelEditor.sectionHints.routing',
    fields: routingFields(type),
  });

  sections.push({
    id: 'long_task',
    title: 'LONG TASK',
    hint: 'channelEditor.sectionHints.longTask',
    fields: longTaskFields(),
  });

  sections.push({
    id: 'avatar',
    title: 'AVATAR',
    fields: [
      {
        museKey: 'icon_asset_id',
        bind: { source: 'icon', key: 'asset_id' },
        kind: 'text',
        hint: 'channelEditor.hints.avatarIcon',
      },
    ],
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
export function catalogCredentialFields(catalog: ChannelTypeCatalog | null): ChannelPlatformField[] {
  return credentialFieldsFromCatalog(catalog);
}
