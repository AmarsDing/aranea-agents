/** Agent planner_kind / planner_config_json — form state, parse, serialize (API contract). */

export type PlannerKindValue = "" | "builtin" | "react" | "a2ui";

export type PlannerBuiltinForm = {
  reasoning_effort: string;
  /** null = omit field (provider default) */
  thinking_enabled: boolean | null;
  thinking_tokens: number | null;
};

export type PlannerA2UIForm = {
  instruction: string;
  server_to_client_with_standard_catalog_schema_json: string;
  client_to_server_schema_json: string;
  client_capabilities_schema_json: string;
  server_to_client_only_schema_json: string;
  standard_catalog_definition_json: string;
  catalog_description_schema_json: string;
};

export type PlannerFormState = {
  kind: PlannerKindValue;
  builtin: PlannerBuiltinForm;
  a2ui: PlannerA2UIForm;
};

export const PLANNER_KIND_OPTIONS: { label: string; value: PlannerKindValue; caption: string }[] = [
  {
    label: "无规划（继承对话模式）",
    value: "",
    caption: "Chat 选「深思考」时仍可能启用 Builtin；显式选 builtin/react/a2ui 时以本处为准。",
  },
  { label: "内置思维 (Builtin)", value: "builtin", caption: "o 系 / DeepSeek v4 / Claude / Gemini 内置推理参数。" },
  { label: "ReAct 结构化规划", value: "react", caption: "正文含 /*PLANNING*/ 等标签；Chat 展示步骤卡片。" },
  { label: "A2UI 协议规划", value: "a2ui", caption: "输出 JSONL；Chat 展示结构化预览。" },
];

export const REASONING_EFFORT_OPENAI = [
  { label: "未设置", value: "" },
  { label: "low", value: "low" },
  { label: "medium", value: "medium" },
  { label: "high", value: "high" },
];

/** Aligns with internal/biz/planner.go validReasoningEfforts (empty = omit on save). */
export const VALID_REASONING_EFFORTS = new Set(["low", "medium", "high", "max"]);

export const REASONING_EFFORT_DEEPSEEK = [
  { label: "未设置", value: "" },
  { label: "high", value: "high" },
  { label: "max", value: "max" },
];

export function defaultPlannerForm(): PlannerFormState {
  return {
    kind: "",
    builtin: { reasoning_effort: "", thinking_enabled: null, thinking_tokens: null },
    a2ui: {
      instruction: "",
      server_to_client_with_standard_catalog_schema_json: "",
      client_to_server_schema_json: "",
      client_capabilities_schema_json: "",
      server_to_client_only_schema_json: "",
      standard_catalog_definition_json: "",
      catalog_description_schema_json: "",
    },
  };
}

function emptyA2UI(): PlannerA2UIForm {
  return { ...defaultPlannerForm().a2ui };
}

export function plannerFormFromSettings(
  kind?: string,
  configJson?: string
): PlannerFormState {
  const form = defaultPlannerForm();
  const k = String(kind ?? "")
    .trim()
    .toLowerCase() as PlannerKindValue;
  if (k === "builtin" || k === "react" || k === "a2ui") {
    form.kind = k;
  }
  const raw = String(configJson ?? "").trim();
  if (!raw || raw === "{}") {
    return form;
  }
  try {
    const obj = JSON.parse(raw) as Record<string, unknown>;
    if (form.kind === "builtin") {
      if (typeof obj.reasoning_effort === "string") {
        const effort = obj.reasoning_effort.trim().toLowerCase();
        form.builtin.reasoning_effort =
          effort && VALID_REASONING_EFFORTS.has(effort) ? effort : "";
      }
      if (typeof obj.thinking_enabled === "boolean") {
        form.builtin.thinking_enabled = obj.thinking_enabled;
      }
      if (typeof obj.thinking_tokens === "number") {
        form.builtin.thinking_tokens = obj.thinking_tokens;
      }
    }
    if (form.kind === "a2ui") {
      const map: (keyof PlannerA2UIForm)[] = [
        "instruction",
        "server_to_client_with_standard_catalog_schema_json",
        "client_to_server_schema_json",
        "client_capabilities_schema_json",
        "server_to_client_only_schema_json",
        "standard_catalog_definition_json",
        "catalog_description_schema_json",
      ];
      for (const key of map) {
        if (typeof obj[key] === "string") {
          form.a2ui[key] = obj[key];
        }
      }
    }
  } catch {
    /* keep defaults */
  }
  return form;
}

function isValidJsonObject(raw: string): boolean {
  const t = raw.trim();
  if (!t) return true;
  try {
    const v = JSON.parse(t);
    return v !== null && typeof v === "object" && !Array.isArray(v);
  } catch {
    return false;
  }
}

export function validatePlannerForm(form: PlannerFormState): string | null {
  if (form.kind === "builtin") {
    const effort = form.builtin.reasoning_effort.trim().toLowerCase();
    if (effort && !VALID_REASONING_EFFORTS.has(effort)) {
      return "reasoning_effort 必须是 low、medium、high 或 max";
    }
  }
  if (form.kind === "a2ui") {
    const jsonFields: (keyof PlannerA2UIForm)[] = [
      "server_to_client_with_standard_catalog_schema_json",
      "client_to_server_schema_json",
      "client_capabilities_schema_json",
      "server_to_client_only_schema_json",
      "standard_catalog_definition_json",
      "catalog_description_schema_json",
    ];
    for (const key of jsonFields) {
      const v = form.a2ui[key].trim();
      if (v && !isValidJsonObject(v)) {
        return `${key} 必须是合法 JSON 对象`;
      }
    }
  }
  if (form.builtin.thinking_tokens != null && form.builtin.thinking_tokens < 0) {
    return "thinking_tokens 不能为负数";
  }
  return null;
}

export function serializePlannerForm(form: PlannerFormState): {
  planner_kind: string;
  planner_config_json: string;
} {
  const kind = form.kind;
  if (kind === "react" || kind === "") {
    return { planner_kind: kind, planner_config_json: "{}" };
  }
  if (kind === "builtin") {
    const payload: Record<string, unknown> = {};
    const effort = form.builtin.reasoning_effort.trim();
    if (effort) payload.reasoning_effort = effort;
    if (form.builtin.thinking_enabled !== null) {
      payload.thinking_enabled = form.builtin.thinking_enabled;
    }
    if (form.builtin.thinking_tokens != null && form.builtin.thinking_tokens > 0) {
      payload.thinking_tokens = form.builtin.thinking_tokens;
    }
    return { planner_kind: kind, planner_config_json: JSON.stringify(payload) };
  }
  if (kind === "a2ui") {
    const payload: Record<string, string> = {};
    const entries = Object.entries(form.a2ui) as [keyof PlannerA2UIForm, string][];
    for (const [key, val] of entries) {
      const v = val.trim();
      if (v) payload[key] = v;
    }
    return { planner_kind: kind, planner_config_json: JSON.stringify(payload) };
  }
  return { planner_kind: "", planner_config_json: "{}" };
}

/** Provider hint for reasoning_effort options (UI only). */
export function reasoningEffortOptions(provider: string): { label: string; value: string }[] {
  const p = provider.trim().toLowerCase();
  if (p.includes("deepseek")) return REASONING_EFFORT_DEEPSEEK;
  return REASONING_EFFORT_OPENAI;
}

export { emptyA2UI };
