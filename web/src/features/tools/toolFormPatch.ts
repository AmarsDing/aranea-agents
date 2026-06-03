import type { Tool, ToolUpsertInput } from './types';

const BOOL_FIELDS = new Set<keyof ToolUpsertInput>([
  'enabled',
  'readonly',
  'requires_confirmation',
  'supports_streaming',
  'supports_concurrency',
]);

export function toolToUpsertInput(tool: Tool, overrides?: Partial<ToolUpsertInput>): ToolUpsertInput {
  return {
    key: tool.key,
    display_name: tool.display_name,
    description: tool.description,
    category: tool.category,
    source: tool.source,
    risk_level: tool.risk_level,
    enabled: tool.enabled,
    readonly: tool.readonly,
    requires_confirmation: tool.requires_confirmation,
    supports_streaming: tool.supports_streaming,
    supports_concurrency: tool.supports_concurrency,
    parameters_schema_json: tool.parameters_schema_json || '{}',
    result_schema_json: tool.result_schema_json || '{}',
    config_schema_json: tool.config_schema_json || '{}',
    config_json: tool.config_json || '{}',
    default_config_json: tool.default_config_json || '{}',
    metadata_json: tool.metadata_json || '{}',
    ...overrides,
  };
}

/** Apply partial updates to reactive tool form; preserves boolean types. */
export function patchToolForm(form: ToolUpsertInput, p: Partial<ToolUpsertInput>): void {
  for (const [k, v] of Object.entries(p) as [keyof ToolUpsertInput, ToolUpsertInput[keyof ToolUpsertInput]][]) {
    if (BOOL_FIELDS.has(k)) {
      (form as Record<string, unknown>)[k] = Boolean(v);
      continue;
    }
    (form as Record<string, unknown>)[k] = v == null ? '' : String(v);
  }
}
