/** UX copy for Tools editor — single source for hints, policy cards, help drawer. */

export type ToolPolicyToggleId =
  | "enabled"
  | "readonly"
  | "requires_confirmation"
  | "supports_streaming"
  | "supports_concurrency";

export type ToolPolicyToggleCopy = {
  id: ToolPolicyToggleId;
  title: string;
  summary: string;
  impact: string;
  note?: string;
  /** When true, disabled for builtin readonly tools (registry sync). */
  registryLocked?: boolean;
};

export const TOOL_POLICY_TOGGLES: ToolPolicyToggleCopy[] = [
  {
    id: "enabled",
    title: "全局启用",
    summary: "关闭后所有 Agent 默认不可用（除非策略 allow 显式点名）。",
    impact: "写入目录 enabled；列表页开关与此一致。"
  },
  {
    id: "readonly",
    title: "系统维护（只读）",
    summary: "内置工具由平台维护，Key 与 Schema 不可改。",
    impact: "仅 custom / external 工具可关闭此项。",
    note: "builtin 工具创建后始终为只读。"
  },
  {
    id: "requires_confirmation",
    title: "执行前需确认",
    summary: "Agent 调用前暂停，等待你在对话中确认（删文件、发邮件、跑命令等）。",
    impact: "运行时走 ToolRequiresConfirmation 确认门禁。",
    note: "与 Agent 覆盖里的「需确认」可叠加；不等于 Agent 工具总开关。",
    registryLocked: true
  },
  {
    id: "supports_streaming",
    title: "支持流式返回",
    summary: "工具可分段返回结果（长输出、进度类）。",
    impact: "目录标记 runtime_kind=streaming。",
    note: "是否走 StreamableCall 还取决于 Agent「流式工具」总开关。",
    registryLocked: true
  },
  {
    id: "supports_concurrency",
    title: "适合并行调用",
    summary: "标记为只读/幂等类，可与其他工具同轮并行。",
    impact: "目录元数据；实际并行由 Agent「并行工具调用」+ concurrent_allow 决定。",
    registryLocked: true
  }
];

export const TOOL_EDITOR_TABS = [
  { name: "basic", label: "基础" },
  { name: "policy", label: "运行策略" },
  { name: "schema", label: "参数与配置" },
  { name: "advanced", label: "高级" }
] as const;

export type ToolEditorTab = (typeof TOOL_EDITOR_TABS)[number]["name"];

export const TOOL_FIELD_HINTS = {
  key: "全局唯一标识，Agent 策略 allow/deny 引用此值；创建后不可修改。格式：snake_case，如 web_research。",
  display_name: "列表与详情中展示的名称。",
  description: "会进入 Agent 上下文，写清楚能做什么、边界与限制。",
  category: "影响列表筛选与 profile 工具组，如 web、filesystem、integration。",
  source: "external=外部注册；builtin/mcp/system 由平台维护。",
  risk_level: "影响启用二次确认与审计展示：low / medium / high / critical。",
  parameters_schema_json:
    "LLM 调用时可传的参数字段（JSON Schema）。无参工具填 {}。",
  result_schema_json: "描述返回数据结构（可选），便于测试与文档。",
  config_schema_json: "管理员配置项定义（API Key、超时等）；定义后下方出现可视化表单。",
  config_json: "当前生效的配置值；须符合配置 Schema。敏感字段不会明文出现在调用记录。",
  default_config_json: "重置时恢复的出厂默认；通常与当前配置相同或为其子集。",
  metadata_json: "扩展元数据（OpenAPI URL、MCP 信息等）；普通用户可留 {}。"
} as const;

export const TOOL_CREATE_TEMPLATES = [
  {
    id: "blank",
    label: "空白 Tool",
    caption: "自行填写 Schema 与配置",
    apply: null as null
  },
  {
    id: "rest_query",
    label: "REST 查询（单参数）",
    caption: "query 字符串参数 + 可选超时配置",
    apply: {
      category: "integration",
      source: "external",
      risk_level: "medium",
      parameters_schema_json: JSON.stringify(
        {
          type: "object",
          properties: {
            query: { type: "string", description: "请求参数或查询体" }
          },
          required: ["query"]
        },
        null,
        2
      ),
      config_schema_json: JSON.stringify(
        {
          type: "object",
          properties: {
            base_url: { type: "string", title: "API 基址" },
            timeout_sec: { type: "integer", title: "超时 (秒)", default: 30 }
          }
        },
        null,
        2
      ),
      config_json: JSON.stringify({ timeout_sec: 30 }, null, 2),
      default_config_json: JSON.stringify({ timeout_sec: 30 }, null, 2),
      metadata_json: JSON.stringify({ kind: "rest" }, null, 2)
    }
  },
  {
    id: "openapi",
    label: "OpenAPI / REST",
    caption: "metadata 中填写 openapi_spec_url",
    apply: {
      category: "integration",
      source: "external",
      risk_level: "medium",
      parameters_schema_json: "{}",
      metadata_json: JSON.stringify(
        { kind: "openapi", openapi_spec_url: "https://example.com/openapi.json" },
        null,
        2
      )
    }
  }
] as const;

export type ToolHelpSection = {
  title: string;
  /** Bullet list (preferred for readability). */
  items?: readonly string[];
  /** Fallback paragraph when items not used. */
  body?: string;
  /** Optional JSON / code sample. */
  code?: string;
};

export const TOOL_HELP_SECTIONS: ToolHelpSection[] = [
  {
    title: "配置分层",
    items: [
      "系统默认（builtin seed）",
      "全局 config_json（Tools 详情 → 配置 Tab）",
      "Agent 策略（profile / allow / deny）",
      "Agent 覆盖（tool_agent_overrides）",
      "单次调用上下文（session 等系统注入）"
    ],
    body: "模型只能看到 parameters_schema 里声明的字段。"
  },
  {
    title: "日常运维去哪改？",
    items: [
      "启用 / 停用 → 列表页开关",
      "API Key、超时 → 详情「配置」Tab 或编辑「参数与配置」",
      "改契约（Schema）→ 编辑弹窗",
      "Agent 级策略 → Agent 列表 → 能力 Tab"
    ]
  },
  {
    title: "运行策略：目录标记 ≠ 运行时生效",
    items: [
      "「需确认 / 流式 / 可并行」是目录元数据，chip 带「标记：」前缀",
      "实际是否流式 / 并行 → Agent「流式工具」「并行工具调用」总开关",
      "内置工具后三项由 registry 维护，重启可能恢复默认"
    ]
  },
  {
    title: "Schema 字段视图边界",
    items: [
      "字段视图仅支持扁平 object.properties（string / number / integer / boolean）",
      "嵌套 object、array、oneOf → 请用 JSON 模式",
      "从 JSON 切回字段视图可能丢失复杂结构"
    ]
  },
  {
    title: "JSON Schema 格式",
    body: "使用 JSON Schema Draft 7，根节点 type 为 object，properties 定义字段。",
    code: `{
  "type": "object",
  "properties": {
    "query": { "type": "string" }
  },
  "required": ["query"]
}`
  }
];

/** Field quick reference rows — readable label + optional technical key. */
export const TOOL_FIELD_HINT_ENTRIES: { key: keyof typeof TOOL_FIELD_HINTS; label: string }[] = [
  { key: "key", label: "Tool Key" },
  { key: "display_name", label: "显示名称" },
  { key: "description", label: "描述" },
  { key: "category", label: "分类" },
  { key: "source", label: "来源" },
  { key: "risk_level", label: "风险级别" },
  { key: "parameters_schema_json", label: "模型参数 Schema" },
  { key: "result_schema_json", label: "返回结构 Schema" },
  { key: "config_schema_json", label: "配置项 Schema" },
  { key: "config_json", label: "当前配置值" },
  { key: "default_config_json", label: "出厂默认配置" },
  { key: "metadata_json", label: "扩展元数据" }
];

export function isRegistryLockedTool(form: { readonly?: boolean; source?: string }): boolean {
  return Boolean(form.readonly) || form.source === "builtin";
}

/** Detail / list chip labels — directory marks, not runtime guarantees. */
export const TOOL_POLICY_CHIP_COPY = {
  requires_confirmation: {
    label: "标记：需确认",
    tooltip: "目录标记：调用前可能需用户确认。实际还取决于 Agent 覆盖与运行时门禁。"
  },
  supports_streaming: {
    label: "标记：流式",
    tooltip: "目录标记：工具支持 StreamableCall。实际流式还取决于 Agent「流式工具」总开关。"
  },
  supports_concurrency: {
    label: "标记：可并行",
    tooltip: "目录标记：适合与其他只读工具同轮并行。实际并行取决于 Agent「并行工具调用」与 allow 列表。"
  }
} as const;
