/**
 * PGO-2: FieldGuide schema — TypeScript mirror of internal/biz/field_guides.go.
 * Keep in sync with the Go registry. Drives UI placeholders, char-budget indicators
 * and the AIRefineButton meta-prompt context.
 */

export type FieldScope =
  | 'category.industry'
  | 'category.department'
  | 'category.position'
  | 'agent.description'
  | 'agent.file';

export interface GuideExample {
  industry: string;
  body: string;
}

export interface GuideBudget {
  soft: number; // chars above → yellow warning
  hard: number; // chars above → red / block (0 = no limit)
}

export interface FieldGuide {
  scope: FieldScope;
  fileName?: string; // only for scope='agent.file'
  titleZh: string;
  purpose: string;
  shouldWrite: string[];
  shouldAvoid: string[];
  examples: GuideExample[];
  budget: GuideBudget;
  placeholder: string;
  defaultStub: string;
}

// ──────────────────────────────────────────────────────────────────
// Registry
// ──────────────────────────────────────────────────────────────────

const guides: FieldGuide[] = [
  // ── Category guides ──────────────────────────────────────────
  {
    scope: 'category.industry',
    titleZh: '行业说明',
    purpose: '描述这个行业的特点、监管约束、客户特征，为所有子部门和岗位提供共同背景。',
    shouldWrite: ['行业核心特征（2–3 句）', '主要监管 / 合规约束', '典型客户画像', '与相邻行业的边界'],
    shouldAvoid: [
      '具体岗位职责（请在岗位层填写）',
      'Agent 话术与人设（请在 IDENTITY.md）',
      '流程 SOP（请在 AGENTS_TASK.md）',
    ],
    examples: [
      {
        industry: '电商',
        body: '面向 C 端消费者的在线零售平台，受《电子商务法》约束；用户对比价格敏感，退换货率较高。',
      },
      { industry: '医疗', body: '院内 / 院外健康信息服务，须遵循 HIPAA/互联网医疗相关法规，禁止替代医嘱。' },
    ],
    budget: { soft: 400, hard: 600 },
    placeholder: '请简述本行业的特点、监管约束与客户画像（建议 100–400 字）',
    defaultStub: '# 行业说明\n\n（请描述行业特点、监管约束与客户画像）',
  },
  {
    scope: 'category.department',
    titleZh: '部门职责',
    purpose: '描述部门的核心使命、团队协作边界与主要输入/输出，为所有子岗位提供上下文。',
    shouldWrite: [
      '部门使命（1–2 句）',
      '核心输入（谁传递任务给本部门）',
      '核心输出（本部门产出什么）',
      '与其他部门的协作边界',
    ],
    shouldAvoid: ['具体岗位的细粒度流程', 'Agent 个体的话术偏好'],
    examples: [
      {
        industry: '电商',
        body: '售后服务部：负责处理用户订单纠纷、退款退货及满意度管理；接受业务部门派单，输出已解决工单与用户评价。',
      },
    ],
    budget: { soft: 500, hard: 800 },
    placeholder: '请简述部门使命、核心输入输出与协作边界（建议 100–500 字）',
    defaultStub: '# 部门职责\n\n（请描述部门使命、输入输出与协作边界）',
  },
  {
    scope: 'category.position',
    titleZh: '岗位职责',
    purpose: '这个岗位的核心职责清单、能/不能做的边界、典型工作流、KPI。注入 system instruction 的 L1 层。',
    shouldWrite: [
      '主要职责：3–5 条 bullet',
      '工作边界：能做什么 / 不能做什么各 2–3 条',
      '典型流程：1–3 个常见 workflow 标题',
      '关键 KPI：1–3 条可衡量指标',
    ],
    shouldAvoid: [
      '复制 Agent 的话术（请去 agent_description 或 IDENTITY.md）',
      '长流程 SOP（请去 AGENTS_TASK.md）',
      '跨行业通用知识（请去行业说明）',
    ],
    examples: [
      {
        industry: '电商',
        body: '1) 处理售后退换货：核实订单→判定责任→执行方案\n2) 不可承诺超出政策的赔付\n3) KPI：首响应时长 < 3 min，解决率 > 85%',
      },
      { industry: '医疗', body: '1) 提供健康信息查询\n2) 不可给出诊断或处方建议\n3) 必要时引导用户就医' },
    ],
    budget: { soft: 800, hard: 1000 },
    placeholder: '示例：1) 处理售后退换货 ... 2) 不可承诺超出政策的赔付 ...',
    defaultStub: '# 岗位职责\n\n（请描述本岗位的主要职责、边界、流程与 KPI）',
  },

  // ── Agent description ─────────────────────────────────────────
  {
    scope: 'agent.description',
    titleZh: 'Agent 描述（个体定位）',
    purpose: '这个 Agent 个体的称呼、擅长场景、关键约束；是 L2 层，在 L1 岗位职责之后注入。',
    shouldWrite: ['称呼与身份（1 句）', '擅长的 3–5 个具体场景', '关键约束或前置条件（1–3 条）'],
    shouldAvoid: ['与岗位职责重复的内容（L1 已有）', '超过 600 字的长 SOP（请去 AGENTS_TASK.md）'],
    examples: [
      {
        industry: '通用',
        body: '我是「小虾米」，专注处理中文淘宝退换货纠纷。我会核实订单后快速判断责任方，给出最优解决方案。我不能替代人工处理升级投诉或超权限赔付。',
      },
    ],
    budget: { soft: 400, hard: 600 },
    placeholder: '请简述这个 Agent 的称呼、擅长场景与关键约束（建议 100–400 字）',
    defaultStub: '',
  },

  // ── Prompt files ──────────────────────────────────────────────
  {
    scope: 'agent.file',
    fileName: 'AGENTS_CORE.md',
    titleZh: '核心角色说明',
    purpose: '固化 Agent 的首要原则、服务宗旨与模型行为偏好；每次对话必读。',
    shouldWrite: ['首要使命（2–4 句）', '核心行为原则（3–6 条）', '模型推理偏好'],
    shouldAvoid: ['任务级 SOP（见 AGENTS_TASK.md）', '个性/话术（见 IDENTITY.md）', '工具白名单（见 CAPABILITIES.md）'],
    examples: [
      {
        industry: '通用',
        body: '# AGENTS_CORE\n我是高效、诚实的 AI 助手，核心使命是帮助用户解决问题并持续学习。\n\n## 原则\n1. 事实优先，避免幻觉\n2. 对不确定的内容说明置信度\n3. 用用户语言回复',
      },
    ],
    budget: { soft: 600, hard: 1200 },
    placeholder: '描述 Agent 的首要原则、服务宗旨与模型行为偏好',
    defaultStub: '# AGENTS_CORE\n\n（请描述 Agent 的核心角色、首要原则、模型偏好）',
  },
  {
    scope: 'agent.file',
    fileName: 'AGENTS_TASK.md',
    titleZh: '任务模式说明',
    purpose: '在 task 模式下需要的额外上下文：目标、输出格式、SOP、协作。',
    shouldWrite: ['任务目标与输出契约', '标准 SOP（2–5 步流程）', '跨 Agent 协作机制', '常见异常处理方式'],
    shouldAvoid: ['对话式互动话术（见 IDENTITY.md）', '核心原则（见 AGENTS_CORE.md）'],
    examples: [
      {
        industry: '通用',
        body: '# AGENTS_TASK\n## 目标\n完成用户指定的信息搜集任务并以结构化 markdown 输出。\n## SOP\n1. 理解需求\n2. 拆解子任务\n3. 并行搜集\n4. 汇总输出',
      },
    ],
    budget: { soft: 800, hard: 1600 },
    placeholder: '描述任务目标、输出格式、SOP 与协作机制',
    defaultStub: '# AGENTS_TASK\n\n（请描述任务目标、输出契约、协作机制与典型 SOP）',
  },
  {
    scope: 'agent.file',
    fileName: 'IDENTITY.md',
    titleZh: '身份与人设',
    purpose: '固化 Agent 的名字、语气、口头禅、不可妥协人设；Evolution 会更新 ## Persona 段。',
    shouldWrite: ['Agent 的称呼', '语气风格', '口头禅（可选）', '## Persona 段（Evolution 专属）'],
    shouldAvoid: ['岗位职责（L1 已注入）', '工具调用规则（见 CAPABILITIES.md）', '大段 SOP（见 AGENTS_TASK.md）'],
    examples: [
      {
        industry: '通用',
        body: '# IDENTITY\n我叫「小问」，是一名友善、专业的 AI 客服。\n语气：亲切但专业，使用中文。\n\n## Persona\n（由 Evolution 自动维护，请勿手动大幅修改此段）',
      },
    ],
    budget: { soft: 500, hard: 800 },
    placeholder: '描述 Agent 的名字、语气、口头禅与关键人设限制',
    defaultStub:
      '# IDENTITY\n\n（请描述 Agent 的称呼、语气、口头禅、不可妥协人设）\n\n## Persona\n\n（人设细节写在此处，由 Evolution 自动更新）',
  },
  {
    scope: 'agent.file',
    fileName: 'CAPABILITIES.md',
    titleZh: '能力与工具',
    purpose: '列出 Agent 有权调用的工具、Skill 与知识库；构建期压制运行时 cue 中的重复枚举。',
    shouldWrite: ['工具白名单', '禁用工具（如有）', '知识库范围', 'Skill 名称与用途'],
    shouldAvoid: ['工具调用规则（见 RULE.md）', '任务流程（见 AGENTS_TASK.md）'],
    examples: [
      {
        industry: '通用',
        body: '# CAPABILITIES\n## 可用工具\n- search: 网络检索\n- calculator: 四则运算\n\n## 禁用\n- file_write（只读模式）',
      },
    ],
    budget: { soft: 600, hard: 1000 },
    placeholder: '列出工具白名单、Skill 与知识库范围',
    defaultStub: '# CAPABILITIES\n\n（请列出工具白名单、Skill 列表、能力边界）',
  },
  {
    scope: 'agent.file',
    fileName: 'RULE.md',
    titleZh: '规则与合规',
    purpose: '列出禁止行为、安全边界、合规要求与降级策略；每次对话必读。',
    shouldWrite: ['绝对禁止列表', '合规要求', '违规降级策略', '数据隐私边界（如有）'],
    shouldAvoid: ['推荐行为（去 AGENTS_CORE.md）', '任务 SOP（去 AGENTS_TASK.md）'],
    examples: [
      {
        industry: '通用',
        body: '# RULE\n## 禁止\n- 提供医疗诊断\n- 泄露用户隐私\n- 绕过平台安全策略\n\n## 违规处理\n遇到违禁请求，礼貌拒绝并提示用户联系人工。',
      },
    ],
    budget: { soft: 500, hard: 800 },
    placeholder: '列出禁止行为、合规要求与降级策略',
    defaultStub: '# RULE\n\n（请列出禁止行为、合规要求、降级策略）',
  },
  {
    scope: 'agent.file',
    fileName: 'USER_CONTEXT.md',
    titleZh: '用户上下文（可选）',
    purpose: '记录用户的长期偏好、历史与个性化设置；替代已废弃的 USER.md + USER_PREDEFINED.md。',
    shouldWrite: ['用户的称呼偏好', '已知偏好或习惯', '持久化的个性化配置'],
    shouldAvoid: ['本轮对话信息（应在 message history 中）', 'Agent 规则（去 RULE.md）'],
    examples: [
      {
        industry: '通用',
        body: '# USER_CONTEXT\n用户偏好使用简体中文；对技术细节有较强理解力；禁止在回复中使用表情符号。',
      },
    ],
    budget: { soft: 400, hard: 600 },
    placeholder: '记录用户长期偏好、历史与个性化设置（可选文件）',
    defaultStub: '# USER_CONTEXT\n\n（记录用户的长期偏好、历史与个性化设置）',
  },
];

// ──────────────────────────────────────────────────────────────────
// Lookup helpers
// ──────────────────────────────────────────────────────────────────

/** Look up a guide by scope (and optionally fileName for agent.file). */
export function getFieldGuide(scope: FieldScope, fileName?: string): FieldGuide | undefined {
  return guides.find((g) => g.scope === scope && (scope !== 'agent.file' || g.fileName === fileName));
}

/** All registered guides (for admin / lint tooling). */
export function listFieldGuides(): FieldGuide[] {
  return guides;
}

/** All guides for a specific scope. */
export function getFieldGuidesForScope(scope: FieldScope): FieldGuide[] {
  return guides.filter((g) => g.scope === scope);
}

/**
 * Returns the label for the description textarea based on category level.
 * level 1 = industry, 2 = department, 3 = position.
 */
export function categoryDescriptionLabel(level: 1 | 2 | 3): string {
  const scopeMap: Record<number, FieldScope> = {
    1: 'category.industry',
    2: 'category.department',
    3: 'category.position',
  };
  return getFieldGuide(scopeMap[level])?.titleZh ?? '描述';
}

/**
 * Returns the placeholder for the description textarea based on category level.
 */
export function categoryDescriptionPlaceholder(level: 1 | 2 | 3): string {
  const scopeMap: Record<number, FieldScope> = {
    1: 'category.industry',
    2: 'category.department',
    3: 'category.position',
  };
  return getFieldGuide(scopeMap[level])?.placeholder ?? '';
}

export default guides;
