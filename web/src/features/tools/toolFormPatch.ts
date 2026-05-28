import type { ToolUpsertInput } from "./types";

const BOOL_FIELDS = new Set<keyof ToolUpsertInput>([
  "enabled",
  "readonly",
  "requires_confirmation",
  "supports_streaming",
  "supports_concurrency"
]);

/** Apply partial updates to reactive tool form; preserves boolean types. */
export function patchToolForm(form: ToolUpsertInput, p: Partial<ToolUpsertInput>): void {
  for (const [k, v] of Object.entries(p) as [keyof ToolUpsertInput, ToolUpsertInput[keyof ToolUpsertInput]][]) {
    if (BOOL_FIELDS.has(k)) {
      (form as Record<string, unknown>)[k] = Boolean(v);
      continue;
    }
    (form as Record<string, unknown>)[k] = v == null ? "" : String(v);
  }
}
