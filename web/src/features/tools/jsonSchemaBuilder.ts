/** Lightweight JSON Schema object builder for Tool parameters/config definitions. */

export type SchemaFieldType = 'string' | 'number' | 'integer' | 'boolean';

export type SchemaFieldRow = {
  key: string;
  type: SchemaFieldType;
  title: string;
  description: string;
  required: boolean;
  enumValues: string;
};

export type JsonSchemaObject = {
  type?: string;
  properties?: Record<string, Record<string, unknown>>;
  required?: string[];
};

export function emptySchemaField(): SchemaFieldRow {
  return { key: '', type: 'string', title: '', description: '', required: false, enumValues: '' };
}

export function parseSchemaFields(schemaJson: string): SchemaFieldRow[] {
  let schema: JsonSchemaObject;
  try {
    schema = JSON.parse(schemaJson || '{}') as JsonSchemaObject;
  } catch {
    return [];
  }
  const props = schema.properties ?? {};
  const required = new Set(schema.required ?? []);
  return Object.entries(props).map(([key, def]) => {
    const row: SchemaFieldRow = {
      key,
      type: (def.type as SchemaFieldType) ?? 'string',
      title: String(def.title ?? ''),
      description: String(def.description ?? ''),
      required: required.has(key),
      enumValues: Array.isArray(def.enum) ? (def.enum as string[]).join(', ') : '',
    };
    return row;
  });
}

export function buildSchemaFromFields(rows: SchemaFieldRow[]): string {
  const properties: Record<string, Record<string, unknown>> = {};
  const required: string[] = [];
  for (const row of rows) {
    const key = row.key.trim();
    if (!key) continue;
    const def: Record<string, unknown> = { type: row.type };
    if (row.title.trim()) def.title = row.title.trim();
    if (row.description.trim()) def.description = row.description.trim();
    if (row.enumValues.trim()) {
      def.enum = row.enumValues
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean);
    }
    properties[key] = def;
    if (row.required) required.push(key);
  }
  const schema: JsonSchemaObject = { type: 'object', properties };
  if (required.length) schema.required = required;
  return JSON.stringify(schema, null, 2);
}

/** Keys in config_json not declared in config_schema properties. */
export function configExtraKeys(configJson: string, schemaJson: string): string[] {
  let config: Record<string, unknown> = {};
  let schema: JsonSchemaObject = {};
  try {
    config = JSON.parse(configJson || '{}') as Record<string, unknown>;
  } catch {
    return [];
  }
  try {
    schema = JSON.parse(schemaJson || '{}') as JsonSchemaObject;
  } catch {
    return Object.keys(config);
  }
  const allowed = new Set(Object.keys(schema.properties ?? {}));
  return Object.keys(config).filter((k) => !allowed.has(k));
}

/** Shallow diff labels for default vs current config. */
export function configDiffSummary(currentJson: string, defaultJson: string): string[] {
  let current: Record<string, unknown> = {};
  let defaults: Record<string, unknown> = {};
  try {
    current = JSON.parse(currentJson || '{}') as Record<string, unknown>;
  } catch {
    return ['当前配置 JSON 无效'];
  }
  try {
    defaults = JSON.parse(defaultJson || '{}') as Record<string, unknown>;
  } catch {
    return ['默认配置 JSON 无效'];
  }
  const lines: string[] = [];
  const keys = new Set([...Object.keys(current), ...Object.keys(defaults)]);
  for (const k of keys) {
    const a = JSON.stringify(current[k]);
    const b = JSON.stringify(defaults[k]);
    if (a !== b) lines.push(`${k}: 默认 ${b ?? '—'} → 当前 ${a ?? '—'}`);
  }
  return lines.length ? lines : ['与出厂默认一致'];
}
