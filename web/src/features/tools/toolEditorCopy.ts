/** UX copy for Tools editor — single source for hints, policy cards, help drawer. */
import { i18n } from '../../i18n';

export type ToolPolicyToggleId =
  | 'enabled'
  | 'readonly'
  | 'requires_confirmation'
  | 'supports_streaming'
  | 'supports_concurrency';

export type ToolPolicyToggleCopy = {
  id: ToolPolicyToggleId;
  title: string;
  summary: string;
  impact: string;
  note?: string;
  /** When true, disabled for builtin readonly tools (registry sync). */
  registryLocked?: boolean;
};

/** 运行策略开关卡片（函数形式，随 locale 取值） */
export function toolPolicyToggles(): ToolPolicyToggleCopy[] {
  const t = i18n.global.t;
  return [
    {
      id: 'enabled',
      title: t('toolsPage.copy.toggle.enabled.title'),
      summary: t('toolsPage.copy.toggle.enabled.summary'),
      impact: t('toolsPage.copy.toggle.enabled.impact'),
    },
    {
      id: 'readonly',
      title: t('toolsPage.copy.toggle.readonly.title'),
      summary: t('toolsPage.copy.toggle.readonly.summary'),
      impact: t('toolsPage.copy.toggle.readonly.impact'),
      note: t('toolsPage.copy.toggle.readonly.note'),
    },
    {
      id: 'requires_confirmation',
      title: t('toolsPage.copy.toggle.requiresConfirmation.title'),
      summary: t('toolsPage.copy.toggle.requiresConfirmation.summary'),
      impact: t('toolsPage.copy.toggle.requiresConfirmation.impact'),
      note: t('toolsPage.copy.toggle.requiresConfirmation.note'),
      registryLocked: true,
    },
    {
      id: 'supports_streaming',
      title: t('toolsPage.copy.toggle.supportsStreaming.title'),
      summary: t('toolsPage.copy.toggle.supportsStreaming.summary'),
      impact: t('toolsPage.copy.toggle.supportsStreaming.impact'),
      note: t('toolsPage.copy.toggle.supportsStreaming.note'),
      registryLocked: true,
    },
    {
      id: 'supports_concurrency',
      title: t('toolsPage.copy.toggle.supportsConcurrency.title'),
      summary: t('toolsPage.copy.toggle.supportsConcurrency.summary'),
      impact: t('toolsPage.copy.toggle.supportsConcurrency.impact'),
      registryLocked: true,
    },
  ];
}

export type ToolFieldHintKey =
  | 'key'
  | 'display_name'
  | 'description'
  | 'category'
  | 'source'
  | 'risk_level'
  | 'parameters_schema_json'
  | 'result_schema_json'
  | 'config_schema_json'
  | 'config_json'
  | 'default_config_json'
  | 'metadata_json';

const TOOL_FIELD_HINT_I18N_KEYS: Record<ToolFieldHintKey, string> = {
  key: 'key',
  display_name: 'displayName',
  description: 'description',
  category: 'category',
  source: 'source',
  risk_level: 'riskLevel',
  parameters_schema_json: 'parametersSchemaJson',
  result_schema_json: 'resultSchemaJson',
  config_schema_json: 'configSchemaJson',
  config_json: 'configJson',
  default_config_json: 'defaultConfigJson',
  metadata_json: 'metadataJson',
};

export function toolFieldHints(): Record<ToolFieldHintKey, string> {
  const t = i18n.global.t;
  const out = {} as Record<ToolFieldHintKey, string>;
  for (const [field, key] of Object.entries(TOOL_FIELD_HINT_I18N_KEYS) as [ToolFieldHintKey, string][]) {
    out[field] = t(`toolsPage.copy.hints.${key}`);
  }
  return out;
}

export type ToolCreateTemplate = {
  id: 'blank' | 'rest_query' | 'openapi';
  label: string;
  caption: string;
  apply: Record<string, string> | null;
};

export function toolCreateTemplates(): ToolCreateTemplate[] {
  const t = i18n.global.t;
  return [
    {
      id: 'blank',
      label: t('toolsPage.copy.templates.blank.label'),
      caption: t('toolsPage.copy.templates.blank.caption'),
      apply: null,
    },
    {
      id: 'rest_query',
      label: t('toolsPage.copy.templates.restQuery.label'),
      caption: t('toolsPage.copy.templates.restQuery.caption'),
      apply: {
        category: 'integration',
        source: 'external',
        risk_level: 'medium',
        parameters_schema_json: JSON.stringify(
          {
            type: 'object',
            properties: {
              query: { type: 'string', description: t('toolsPage.copy.templates.restQuery.queryDesc') },
            },
            required: ['query'],
          },
          null,
          2,
        ),
        config_schema_json: JSON.stringify(
          {
            type: 'object',
            properties: {
              base_url: { type: 'string', title: t('toolsPage.copy.templates.restQuery.baseUrlTitle') },
              timeout_sec: { type: 'integer', title: t('toolsPage.copy.templates.restQuery.timeoutTitle'), default: 30 },
            },
          },
          null,
          2,
        ),
        config_json: JSON.stringify({ timeout_sec: 30 }, null, 2),
        default_config_json: JSON.stringify({ timeout_sec: 30 }, null, 2),
        metadata_json: JSON.stringify({ kind: 'rest' }, null, 2),
      },
    },
    {
      id: 'openapi',
      label: t('toolsPage.copy.templates.openapi.label'),
      caption: t('toolsPage.copy.templates.openapi.caption'),
      apply: {
        category: 'integration',
        source: 'external',
        risk_level: 'medium',
        parameters_schema_json: '{}',
        metadata_json: JSON.stringify({ kind: 'openapi', openapi_spec_url: 'https://example.com/openapi.json' }, null, 2),
      },
    },
  ];
}

export type ToolHelpSection = {
  title: string;
  /** Bullet list (preferred for readability). */
  items?: readonly string[];
  /** Fallback paragraph when items not used. */
  body?: string;
  /** Optional JSON / code sample. */
  code?: string;
};

export function toolHelpSections(): ToolHelpSection[] {
  const t = i18n.global.t;
  return [
    {
      title: t('toolsPage.copy.help.layering.title'),
      items: [
        t('toolsPage.copy.help.layering.item0'),
        t('toolsPage.copy.help.layering.item1'),
        t('toolsPage.copy.help.layering.item2'),
        t('toolsPage.copy.help.layering.item3'),
        t('toolsPage.copy.help.layering.item4'),
      ],
      body: t('toolsPage.copy.help.layering.body'),
    },
    {
      title: t('toolsPage.copy.help.ops.title'),
      items: [
        t('toolsPage.copy.help.ops.item0'),
        t('toolsPage.copy.help.ops.item1'),
        t('toolsPage.copy.help.ops.item2'),
        t('toolsPage.copy.help.ops.item3'),
      ],
    },
    {
      title: t('toolsPage.copy.help.policy.title'),
      items: [
        t('toolsPage.copy.help.policy.item0'),
        t('toolsPage.copy.help.policy.item1'),
        t('toolsPage.copy.help.policy.item2'),
      ],
    },
    {
      title: t('toolsPage.copy.help.schemaView.title'),
      items: [
        t('toolsPage.copy.help.schemaView.item0'),
        t('toolsPage.copy.help.schemaView.item1'),
        t('toolsPage.copy.help.schemaView.item2'),
      ],
    },
    {
      title: t('toolsPage.copy.help.jsonSchema.title'),
      body: t('toolsPage.copy.help.jsonSchema.body'),
      code: `{
  "type": "object",
  "properties": {
    "query": { "type": "string" }
  },
  "required": ["query"]
}`,
    },
  ];
}

/** Field quick reference rows — readable label + optional technical key. */
export function toolFieldHintEntries(): { key: ToolFieldHintKey; label: string }[] {
  const t = i18n.global.t;
  const hintKeys = Object.keys(TOOL_FIELD_HINT_I18N_KEYS) as ToolFieldHintKey[];
  return hintKeys.map((field) => ({
    key: field,
    label: t(`toolsPage.copy.hintLabels.${TOOL_FIELD_HINT_I18N_KEYS[field]}`),
  }));
}

export function isRegistryLockedTool(form: { readonly?: boolean; source?: string }): boolean {
  return Boolean(form.readonly) || form.source === 'builtin';
}

/** Detail / list chip labels — directory marks, not runtime guarantees. */
export function toolPolicyChipCopy(): Record<
  'requires_confirmation' | 'supports_streaming' | 'supports_concurrency',
  { label: string; tooltip: string }
> {
  const t = i18n.global.t;
  return {
    requires_confirmation: {
      label: t('toolsPage.copy.chips.requiresConfirmation.label'),
      tooltip: t('toolsPage.copy.chips.requiresConfirmation.tooltip'),
    },
    supports_streaming: {
      label: t('toolsPage.copy.chips.supportsStreaming.label'),
      tooltip: t('toolsPage.copy.chips.supportsStreaming.tooltip'),
    },
    supports_concurrency: {
      label: t('toolsPage.copy.chips.supportsConcurrency.label'),
      tooltip: t('toolsPage.copy.chips.supportsConcurrency.tooltip'),
    },
  };
}
