import { computed, nextTick, onMounted, reactive, ref, watch } from "vue";
import { copyToClipboard, useQuasar } from "quasar";
import { useRoute, useRouter } from "vue-router";
import { storeToRefs } from "pinia";
import type { Agent, AgentPromptFile, AgentRuntimeSettings } from "./api";
import { useAgentDetailStore } from "../../stores/agents";
import {
  defaultAgentFiles,
  promptModes,
  statusOptions,
  tokenEstimateFor,
  type AgentFile,
  type EvolutionKey,
  type PromptMode
} from "../../components/agents/agentUi";
import { listPlatformResources, type PlatformResource } from "../platform/api";
import { listSkills } from "../skills/api";
import { listTools } from "../tools/api";
import { isAvatarAssetRef } from "../avatar/iconModel";
import { useAppStore } from "../../stores/app";
import { useAvatarCatalogStore } from "../../stores/avatar";

export function useAgentSettingsPage() {
  const $q = useQuasar();
  const route = useRoute();
  const router = useRouter();
  const store = useAppStore();
  const avatarCatalogStore = useAvatarCatalogStore();
  const detailStore = useAgentDetailStore();
  const agentId = computed(() => String(route.params.id ?? "").trim());
  const { saving } = storeToRefs(detailStore);
  const tab = ref("agent");
  const promptDialog = ref(false);
  const advancedDialog = ref(false);
  const previewMode = ref<PromptMode>("complete");
  const promptPreview = ref("");
  const fileSplitter = ref(28);
  const activeFile = ref("AGENTS_CORE.md");
  const initialFileBodies = ref<Record<string, string>>({});
  const aiEditOpen = ref(false);
  const avatarPickerOpen = ref(false);
  const aiInstruction = ref("");
  const evolutionRange = ref("30d");
  const providerModels = ref<PlatformResource[]>([]);
  const providerModelSearch = ref("");
  const loadingProviderModels = ref(false);
  const advancedState = reactive({
    channel_id: "",
    chat_id: "",
    workspace: "",
    reasoning_mode: "provider_default",
    reasoning_level: "off",
    context_compaction_enabled: false,
    session_summary_enabled: false,
    truncate_strategy: "sliding",
    recent_window_turns: 20,
    recent_window_tokens: 0,
    summary_keep_turns: 4
  });

  const form = reactive<Agent>({
    id: "",
    agent_key: "",
    display_name: "",
    provider: "",
    model: "",
    agent_kind: "",
    a2a_proxy_config: undefined,
    status: "active",
    is_default: false,
    is_favorite: false,
    icon: "",
    agent_description: "",
    category_position_id: "",
    system_prompt_mode: "complete",
    context_window: 0,
    budget_monthly_cents: 0,
    config_json: "",
    created_at: "",
    updated_at: "",
    deleted_at: ""
  });

  const config = reactive({
    self_evolve: true,
    subagents: {
      enabled: true,
      max_concurrency: 20,
      max_generation_depth: 1,
      max_children_per_agent: 5,
      archive_after_minutes: 60,
      max_retries: 2,
      model_override: ""
    },
    tools: {
      enabled: true,
      profile: "chat_only",
      tool_call_prefix: "",
      allow: [] as string[],
      deny: [] as string[],
      concurrent_allow: [] as string[],
      retry: {
        enabled: false,
        max_attempts: 2,
        initial_interval_ms: 500,
        backoff_factor: 2.0,
        max_interval_ms: 5000,
        jitter: true
      },
      parallel_enabled: false,
      streaming_enabled: false
    },
    memory: {
      enabled: true,
      max_chunk_length: 1000,
      max_results: 6,
      min_score: 0.35
    },
    memoryL0: {
      recent_window_turns: 12,
      recent_window_tokens: 0,
      summary_threshold: 0.6,
      summary_keep_turns: 4,
      truncate_strategy: "summary",
      inject_l1: true,
      inject_l3: true,
      inject_l4: false,
      l3_max_chunks: 5,
      l4_max_paths: 3,
      snapshot_mode: "on_warning"
    },
    memoryL1: {
      enabled: true,
      budget_tokens: 8192,
      field_max_tokens: 2048,
      history_keep_revisions: 10,
      default_schema_id: "",
      archive_on_idle_minutes: 60
    },
    memoryL2: {
      episode_enabled: true,
      episode_min_importance: 0.3,
      index_enabled: true,
      index_embedding_model: "",
      recall_enabled: false,
      recall_max: 3,
      retention_days: 90,
      archive_after_days: 30
    },
    memoryL3: {
      enabled: true,
      recall_top_k: 5,
      recall_min_score: 0.55,
      recall_scopes: ["agent", "user", "team", "workspace"] as string[],
      embedding_model: "",
      decay_interval_hours: 24,
      archive_threshold: 0.2,
      max_per_recall_chars: 1500
    },
    memoryL4: {
      enabled: true,
      graph_inject_neighbors: true,
      graph_max_neighbors: 6,
      graph_max_hops: 2,
      identity_inject: true,
      strategy_inject: false
    },
    evolutionSettings: {
      enabled: false,
      auto_apply: false,
      min_episodes: 20,
      min_negative_feedback: 3,
      throttle_hours: 24,
      proposal_ttl_days: 14,
      persona_max_chars: 1500,
      system_prompt_max_appends: 5
    },
    heartbeat: {
      enabled: false,
      interval_minutes: 30
    },
    evolution: {
      self_evolve: true,
      skill_evolve: true,
      evolution_metrics_enabled: true,
      evolution_suggestions_enabled: true
    } as Record<EvolutionKey, boolean>,
    evolution_guardrails: {
      max_change_per_period: 0.1,
      min_data_points: 100,
      rollback_on_decline_percent: 20
    },
    skillRuntime: {
      intent_routing_enabled: true,
      intent_max_paths: 3,
      max_skills_in_toolset: 32,
      allowed_slugs: [] as string[],
      denied_slugs: [] as string[],
      allowed_tags: [] as string[]
    },
    intent_pass: {
      enabled: true
    }
  });

  const files = reactive<AgentFile[]>(defaultAgentFiles.map((file) => ({ ...file })));

  const heartbeatFile = computed(() => files.find((file) => file.name === "HEARTBEAT.md") ?? files[0]);
  const activeFileMeta = computed(() => files.find((file) => file.name === activeFile.value) ?? files[0]);
  const activeFileBody = computed({
    get: () => activeFileMeta.value.body,
    set: (value: string) => {
      activeFileMeta.value.body = value;
    }
  });
  const fileDirty = computed(() => activeFileBody.value !== (initialFileBodies.value[activeFile.value] ?? ""));
  const budgetUSD = computed({
    get: () => Math.round((form.budget_monthly_cents || 0) / 100),
    set: (value: number) => {
      form.budget_monthly_cents = Math.round((Number(value) || 0) * 100);
    }
  });
  const providerModelOptions = computed(() =>
    providerModels.value
      .filter((row) => row.enabled && row.provider && row.model)
      .map((row) => {
        const contextWindowK = providerContextWindowK(row);
        return {
          label: row.name || row.model,
          value: row.id,
          caption: `${row.provider} / ${row.model}${contextWindowK ? ` · ${contextWindowK}K ctx` : ""}`,
          provider: row.provider,
          model: row.model
        };
      })
  );
  const filteredProviderModelOptions = computed(() => {
    const keyword = providerModelSearch.value.trim().toLowerCase();
    if (!keyword) return providerModelOptions.value;
    return providerModelOptions.value.filter((option) =>
      [option.label, option.caption, option.provider, option.model].some((value) => value.toLowerCase().includes(keyword))
    );
  });
  const selectedProviderModelID = computed(() => providerModelOptions.value.find((row) => row.provider === form.provider && row.model === form.model)?.value ?? "");

  /** Native / legacy tool keys always listed so older agents keep working without catalog rows. */
  const defaultNativeToolKeys = [
    "datetime",
    "web_search",
    "web_fetch",
    "list_files",
    "read_file",
    "write_file",
    "edit_file",
    "shell_exec"
  ];

  const catalogTools = ref<{ key: string; display_name: string }[]>([]);
  const loadingCatalogTools = ref(false);

  /** 平台 Skill 下拉：名称 + slug + 状态（Agent 策略仍存 slug 列表）。 */
  const skillSlugOptions = ref<{ label: string; value: string }[]>([]);
  const loadingSkillSlugs = ref(false);

  async function loadCatalogTools() {
    loadingCatalogTools.value = true;
    try {
      const res = await listTools({ page: 1, page_size: 500 });
      catalogTools.value = (res.items ?? [])
        .map((t) => ({ key: String(t.key ?? "").trim(), display_name: String(t.display_name ?? "").trim() || String(t.key ?? "").trim() }))
        .filter((t) => t.key !== "");
    } catch {
      catalogTools.value = [];
    } finally {
      loadingCatalogTools.value = false;
    }
  }

  async function loadSkillSlugOptions() {
    loadingSkillSlugs.value = true;
    try {
      const data = await listSkills({ page: 1, page_size: 500 });
      const seen = new Set<string>();
      const next: { label: string; value: string }[] = [];
      for (const s of data.items) {
        const slug = String(s.slug ?? "").trim();
        if (!slug || seen.has(slug)) continue;
        seen.add(slug);
        const statusTip =
          s.status === "published" ? "已发布" : s.status === "draft" ? "草稿" : s.status === "archived" ? "已归档" : s.status;
        next.push({
          label: `${s.name || slug} · ${slug} · ${statusTip}`,
          value: slug
        });
      }
      skillSlugOptions.value = next;
    } catch {
      skillSlugOptions.value = [];
    } finally {
      loadingSkillSlugs.value = false;
    }
  }

  const toolSelectOptions = computed(() => {
    const byKey = new Map<string, { label: string; value: string }>();
    for (const k of defaultNativeToolKeys) {
      byKey.set(k, { label: `${k} · 内置`, value: k });
    }
    for (const t of catalogTools.value) {
      const label =
        t.display_name && t.display_name !== t.key ? `${t.display_name} (${t.key})` : t.key;
      byKey.set(t.key, { label, value: t.key });
    }
    const extra = [...config.tools.allow, ...config.tools.deny, ...config.tools.concurrent_allow];
    for (const raw of extra) {
      const key = String(raw ?? "").trim();
      if (key && !byKey.has(key)) {
        byKey.set(key, { label: `${key} · 已保存`, value: key });
      }
    }
    return Array.from(byKey.values()).sort((a, b) => a.label.localeCompare(b.label, "zh-CN"));
  });

  const toolConflicts = computed(() => config.tools.allow.filter((tool) => config.tools.deny.includes(tool)));
  // Profile options surface the current canonical names to the user.
  // Backend still accepts legacy values (minimal/safe/system_admin) so
  // existing agents keep their behaviour even before they are re-saved.
  const toolProfileOptions = [
    { label: "chat_only · 仅对话（无工具）", value: "chat_only" },
    { label: "read_only · 只读 + 时间", value: "read_only" },
    { label: "coding · 文件读写 + 网页 + 技能", value: "coding" },
    { label: "research · 网页 + 检索 + 技能", value: "research" },
    { label: "full · 全工具（高权限，慎用）", value: "full" }
  ];
  const truncateStrategyOptions = ["summary", "drop_oldest", "drop_tool_results", "hybrid"].map((value) => ({ label: value, value }));
  const snapshotModeOptions = ["always", "on_warning", "off"].map((value) => ({ label: value, value }));
  const memoryScopeOptions = ["agent", "user", "team", "workspace", "global"].map((value) => ({ label: value, value }));

  /** 打开设置页时强制预热缩略图（支持 icon 为 asset id 或 asset_key） */
  async function primeAvatarThumbnailCacheForAgentIcon() {
    const raw = String(form.icon ?? "").trim();
    if (!raw || /^(https?:|data:|blob:)/i.test(raw)) return;
    let fetchId = raw;
    if (!isAvatarAssetRef(raw)) {
      await avatarCatalogStore.ensureAgentsCatalog();
      const hit = avatarCatalogStore.agentsCatalog.find((a) => a.id === raw || (a.key && a.key === raw));
      if (!hit?.id) return;
      fetchId = hit.id;
    }
    avatarCatalogStore.forgetThumbnail(fetchId);
    await avatarCatalogStore.ensureThumbnail(fetchId);
  }

  async function applyLoadedAgent(agent: Agent | null | undefined) {
    if (!agent?.id) {
      $q.notify({ type: "warning", message: "未找到该 Agent" });
      router.back();
      return false;
    }
    Object.assign(form, agent);
    hydrateSettings(agent);
    store.upsertAgent(agent);
    snapshotFiles();
    previewMode.value = (form.system_prompt_mode as PromptMode) || "complete";
    await loadPromptPreview();
    await primeAvatarThumbnailCacheForAgentIcon();
    return true;
  }

  onMounted(async () => {
    const id = String(route.params.id ?? "").trim();
    if (!id) {
      $q.notify({ type: "negative", message: "缺少 Agent ID" });
      router.back();
      return;
    }
    try {
      const [agent] = await Promise.all([
        detailStore.fetchById(id),
        loadProviderModels(),
        loadCatalogTools(),
        loadSkillSlugOptions()
      ]);
      await applyLoadedAgent(agent);
    } catch (e) {
      $q.notify({ type: "negative", message: e instanceof Error ? e.message : "加载 Agent 失败" });
      router.back();
    }
  });

  watch(
    () => String(route.params.id ?? "").trim(),
    async (newId, prevId) => {
      if (!newId || newId === prevId) return;
      try {
        const agent = await detailStore.fetchById(newId);
        await applyLoadedAgent(agent);
      } catch (e) {
        $q.notify({ type: "negative", message: e instanceof Error ? e.message : "加载 Agent 失败" });
        router.back();
      }
    }
  );

  watch(previewMode, () => void loadPromptPreview());
  watch(
    () => form.system_prompt_mode,
    (value) => {
      previewMode.value = (value as PromptMode) || "complete";
      config.evolution.self_evolve = config.self_evolve;
    }
  );
  watch(
    () => config.evolution.self_evolve,
    (value) => {
      config.self_evolve = value;
    }
  );

  /** Skill slug 比较规则（与后端 Layer A 一致：小写 + trim） */
  function normSkillSlug(s: string): string {
    return String(s ?? "").trim().toLowerCase();
  }

  /** 与运行时一致：拒绝列表优先，去掉与白名单重叠项后再去掉多余的拒绝项，保证两侧无交集 */
  function reconcileSkillSlugListsDenyWins() {
    const rt = config.skillRuntime;
    const denySet = new Set(rt.denied_slugs.map(normSkillSlug).filter(Boolean));
    rt.allowed_slugs = rt.allowed_slugs.filter((a) => !denySet.has(normSkillSlug(a)));
    const allowSet = new Set(rt.allowed_slugs.map(normSkillSlug).filter(Boolean));
    rt.denied_slugs = rt.denied_slugs.filter((d) => !allowSet.has(normSkillSlug(d)));
  }

  /** 编辑中双向同步时抑制循环触发 */
  const skillSlugListsSyncing = ref(false);

  watch(
    () => config.skillRuntime.allowed_slugs,
    (allowed) => {
      if (skillSlugListsSyncing.value) return;
      skillSlugListsSyncing.value = true;
      try {
        const allowSet = new Set((allowed ?? []).map(normSkillSlug).filter(Boolean));
        config.skillRuntime.denied_slugs = config.skillRuntime.denied_slugs.filter((d) => !allowSet.has(normSkillSlug(d)));
      } finally {
        void nextTick(() => {
          skillSlugListsSyncing.value = false;
        });
      }
    },
    { deep: true }
  );

  watch(
    () => config.skillRuntime.denied_slugs,
    (denied) => {
      if (skillSlugListsSyncing.value) return;
      skillSlugListsSyncing.value = true;
      try {
        const denySet = new Set((denied ?? []).map(normSkillSlug).filter(Boolean));
        config.skillRuntime.allowed_slugs = config.skillRuntime.allowed_slugs.filter((a) => !denySet.has(normSkillSlug(a)));
      } finally {
        void nextTick(() => {
          skillSlugListsSyncing.value = false;
        });
      }
    },
    { deep: true }
  );

  function parseSkillRuntimeForm(raw?: string) {
    const out = {
      intent_routing_enabled: true,
      intent_max_paths: 3,
      max_skills_in_toolset: 32,
      allowed_slugs: [] as string[],
      denied_slugs: [] as string[],
      allowed_tags: [] as string[]
    };
    try {
      const o = JSON.parse(String(raw ?? "{}").trim() || "{}");
      if (typeof o.intent_routing_enabled === "boolean") out.intent_routing_enabled = o.intent_routing_enabled;
      if (typeof o.intent_max_paths === "number" && Number.isFinite(o.intent_max_paths)) {
        const n = Math.floor(o.intent_max_paths);
        if (n >= 1 && n <= 32) out.intent_max_paths = n;
      }
      if (typeof o.max_skills_in_toolset === "number" && Number.isFinite(o.max_skills_in_toolset)) {
        const n = Math.floor(o.max_skills_in_toolset);
        if (n >= 1 && n <= 256) out.max_skills_in_toolset = n;
      }
      const strList = (v: unknown) =>
        Array.isArray(v) ? v.map((x) => String(x).trim()).filter(Boolean) : [];
      out.allowed_slugs = strList(o.allowed_slugs);
      out.denied_slugs = strList(o.denied_slugs);
      out.allowed_tags = strList(o.allowed_tags);
    } catch {
      /* keep defaults */
    }
    return out;
  }

  function normalizeSkillRuntimeState() {
    const rt = config.skillRuntime;
    rt.intent_max_paths = Math.min(32, Math.max(1, Math.floor(Number(rt.intent_max_paths) || 3)));
    rt.max_skills_in_toolset = Math.min(256, Math.max(1, Math.floor(Number(rt.max_skills_in_toolset) || 32)));
    for (const key of ["allowed_slugs", "denied_slugs", "allowed_tags"] as const) {
      if (!Array.isArray(rt[key])) rt[key] = [];
      rt[key] = rt[key].map((x) => String(x).trim()).filter(Boolean);
    }
    skillSlugListsSyncing.value = true;
    try {
      reconcileSkillSlugListsDenyWins();
    } finally {
      void nextTick(() => {
        skillSlugListsSyncing.value = false;
      });
    }
  }

  function stringifySkillRuntimeJSON(): string {
    normalizeSkillRuntimeState();
    return JSON.stringify({
      intent_routing_enabled: config.skillRuntime.intent_routing_enabled,
      intent_max_paths: config.skillRuntime.intent_max_paths,
      max_skills_in_toolset: config.skillRuntime.max_skills_in_toolset,
      allowed_slugs: [...config.skillRuntime.allowed_slugs],
      denied_slugs: [...config.skillRuntime.denied_slugs],
      allowed_tags: [...config.skillRuntime.allowed_tags]
    });
  }

  function resetSkillRuntimeDefaults() {
    Object.assign(config.skillRuntime, parseSkillRuntimeForm("{}"));
    normalizeSkillRuntimeState();
    $q.notify({ type: "info", message: "Skill 策略已恢复默认（尚未保存）" });
  }

  function hydrateConfig(raw: string) {
    try {
      const parsed = JSON.parse(raw || "{}");
      Object.assign(config, {
        ...config,
        ...parsed,
        subagents: { ...config.subagents, ...(parsed.subagents || {}) },
        tools: { ...config.tools, ...(parsed.tools || {}), retry: { ...config.tools.retry, ...((parsed.tools || {}).retry || {}) } },
        memory: { ...config.memory, ...(parsed.memory || {}) },
        memoryL0: { ...config.memoryL0, ...(parsed.memoryL0 || {}) },
        memoryL1: { ...config.memoryL1, ...(parsed.memoryL1 || {}) },
        memoryL2: { ...config.memoryL2, ...(parsed.memoryL2 || {}) },
        memoryL3: { ...config.memoryL3, ...(parsed.memoryL3 || {}) },
        memoryL4: { ...config.memoryL4, ...(parsed.memoryL4 || {}) },
        evolutionSettings: { ...config.evolutionSettings, ...(parsed.evolutionSettings || {}) },
        heartbeat: { ...config.heartbeat, ...(parsed.heartbeat || {}) },
        evolution: { ...config.evolution, ...(parsed.evolution || {}), self_evolve: parsed.self_evolve ?? config.self_evolve },
        evolution_guardrails: { ...config.evolution_guardrails, ...(parsed.evolution_guardrails || {}) },
        skillRuntime: { ...config.skillRuntime, ...(parsed.skillRuntime || {}) },
        intent_pass: { ...config.intent_pass, ...(parsed.intent_pass || {}) }
      });
      if (Array.isArray(parsed.files)) {
        for (const saved of parsed.files) {
          const file = files.find((item) => item.name === saved.name);
          if (file) file.body = saved.body;
        }
      }
    } catch {
      // Legacy config can be plain text; keep defaults.
    }
  }

  function hydrateSettings(agent: Agent) {
    if (agent.settings) {
      Object.assign(config, {
        ...config,
        self_evolve: agent.settings.self_evolve,
        subagents: {
          enabled: agent.settings.subagents_enabled,
          max_concurrency: agent.settings.subagents_max_concurrency,
          max_generation_depth: agent.settings.subagents_max_generation_depth,
          max_children_per_agent: agent.settings.subagents_max_children_per_agent,
          archive_after_minutes: agent.settings.subagents_archive_after_minutes,
          max_retries: agent.settings.subagents_max_retries,
          model_override: agent.settings.subagents_model_override
        },
        tools: {
          enabled: agent.settings.tools_enabled,
          profile: agent.settings.tools_profile,
          tool_call_prefix: agent.settings.tools_tool_call_prefix,
          allow: parseJSONList(agent.settings.tools_allow_json),
          deny: parseJSONList(agent.settings.tools_deny_json),
          concurrent_allow: parseJSONList(agent.settings.tools_concurrent_allow_json),
          retry: {
            enabled: agent.settings.tools_retry_enabled ?? config.tools.retry.enabled,
            max_attempts: agent.settings.tools_retry_max_attempts ?? config.tools.retry.max_attempts,
            initial_interval_ms: agent.settings.tools_retry_initial_interval_ms ?? config.tools.retry.initial_interval_ms,
            backoff_factor: agent.settings.tools_retry_backoff_factor ?? config.tools.retry.backoff_factor,
            max_interval_ms: agent.settings.tools_retry_max_interval_ms ?? config.tools.retry.max_interval_ms,
            jitter: agent.settings.tools_retry_jitter ?? config.tools.retry.jitter
          },
          parallel_enabled: agent.settings.tools_parallel_enabled ?? config.tools.parallel_enabled,
          streaming_enabled: agent.settings.tools_streaming_enabled ?? config.tools.streaming_enabled
        },
        memory: {
          enabled: agent.settings.memory_enabled,
          max_chunk_length: agent.settings.memory_max_chunk_length,
          max_results: agent.settings.memory_max_results,
          min_score: agent.settings.memory_min_score
        },
        memoryL0: {
          recent_window_turns: agent.settings.l0_recent_window_turns ?? config.memoryL0.recent_window_turns,
          recent_window_tokens: agent.settings.l0_recent_window_tokens ?? config.memoryL0.recent_window_tokens,
          summary_threshold: agent.settings.l0_summary_threshold ?? config.memoryL0.summary_threshold,
          summary_keep_turns: agent.settings.l0_summary_keep_turns ?? config.memoryL0.summary_keep_turns,
          truncate_strategy: agent.settings.l0_truncate_strategy || config.memoryL0.truncate_strategy,
          inject_l1: agent.settings.l0_inject_l1 ?? config.memoryL0.inject_l1,
          inject_l3: agent.settings.l0_inject_l3 ?? config.memoryL0.inject_l3,
          inject_l4: agent.settings.l0_inject_l4 ?? config.memoryL0.inject_l4,
          l3_max_chunks: agent.settings.l0_l3_max_chunks ?? config.memoryL0.l3_max_chunks,
          l4_max_paths: agent.settings.l0_l4_max_paths ?? config.memoryL0.l4_max_paths,
          snapshot_mode: agent.settings.l0_snapshot_mode || config.memoryL0.snapshot_mode
        },
        memoryL1: {
          enabled: agent.settings.l1_enabled ?? config.memoryL1.enabled,
          budget_tokens: agent.settings.l1_budget_tokens ?? config.memoryL1.budget_tokens,
          field_max_tokens: agent.settings.l1_field_max_tokens ?? config.memoryL1.field_max_tokens,
          history_keep_revisions: agent.settings.l1_history_keep_revisions ?? config.memoryL1.history_keep_revisions,
          default_schema_id: agent.settings.l1_default_schema_id || config.memoryL1.default_schema_id,
          archive_on_idle_minutes: agent.settings.l1_archive_on_idle_minutes ?? config.memoryL1.archive_on_idle_minutes
        },
        memoryL2: {
          episode_enabled: agent.settings.l2_episode_enabled ?? config.memoryL2.episode_enabled,
          episode_min_importance: agent.settings.l2_episode_min_importance ?? config.memoryL2.episode_min_importance,
          index_enabled: agent.settings.l2_index_enabled ?? config.memoryL2.index_enabled,
          index_embedding_model: agent.settings.l2_index_embedding_model || config.memoryL2.index_embedding_model,
          recall_enabled: agent.settings.l2_recall_enabled ?? config.memoryL2.recall_enabled,
          recall_max: agent.settings.l2_recall_max ?? config.memoryL2.recall_max,
          retention_days: agent.settings.l2_retention_days ?? config.memoryL2.retention_days,
          archive_after_days: agent.settings.l2_archive_after_days ?? config.memoryL2.archive_after_days
        },
        memoryL3: {
          enabled: agent.settings.l3_enabled ?? config.memoryL3.enabled,
          recall_top_k: agent.settings.l3_recall_top_k ?? config.memoryL3.recall_top_k,
          recall_min_score: agent.settings.l3_recall_min_score ?? config.memoryL3.recall_min_score,
          recall_scopes: parseJSONList(agent.settings.l3_recall_scopes_json || JSON.stringify(config.memoryL3.recall_scopes)),
          embedding_model: agent.settings.l3_embedding_model || config.memoryL3.embedding_model,
          decay_interval_hours: agent.settings.l3_decay_interval_hours ?? config.memoryL3.decay_interval_hours,
          archive_threshold: agent.settings.l3_archive_threshold ?? config.memoryL3.archive_threshold,
          max_per_recall_chars: agent.settings.l3_max_per_recall_chars ?? config.memoryL3.max_per_recall_chars
        },
        memoryL4: {
          enabled: agent.settings.l4_enabled ?? config.memoryL4.enabled,
          graph_inject_neighbors: agent.settings.l4_graph_inject_neighbors ?? config.memoryL4.graph_inject_neighbors,
          graph_max_neighbors: agent.settings.l4_graph_max_neighbors ?? config.memoryL4.graph_max_neighbors,
          graph_max_hops: agent.settings.l4_graph_max_hops ?? config.memoryL4.graph_max_hops,
          identity_inject: agent.settings.l4_identity_inject ?? config.memoryL4.identity_inject,
          strategy_inject: agent.settings.l4_strategy_inject ?? config.memoryL4.strategy_inject
        },
        evolutionSettings: {
          enabled: agent.settings.evo_enabled ?? config.evolutionSettings.enabled,
          auto_apply: agent.settings.evo_auto_apply ?? config.evolutionSettings.auto_apply,
          min_episodes: agent.settings.evo_min_episodes ?? config.evolutionSettings.min_episodes,
          min_negative_feedback: agent.settings.evo_min_negative_feedback ?? config.evolutionSettings.min_negative_feedback,
          throttle_hours: agent.settings.evo_throttle_hours ?? config.evolutionSettings.throttle_hours,
          proposal_ttl_days: agent.settings.evo_proposal_ttl_days ?? config.evolutionSettings.proposal_ttl_days,
          persona_max_chars: agent.settings.evo_persona_max_chars ?? config.evolutionSettings.persona_max_chars,
          system_prompt_max_appends: agent.settings.evo_system_prompt_max_appends ?? config.evolutionSettings.system_prompt_max_appends
        },
        heartbeat: {
          enabled: agent.settings.heartbeat_enabled,
          interval_minutes: agent.settings.heartbeat_interval_minutes
        },
        evolution: {
          self_evolve: agent.settings.evolution_self_evolve,
          skill_evolve: agent.settings.evolution_skill_evolve,
          evolution_metrics_enabled: agent.settings.evolution_metrics_enabled,
          evolution_suggestions_enabled: agent.settings.evolution_suggestions_enabled
        },
        evolution_guardrails: {
          max_change_per_period: agent.settings.guardrail_max_change_per_period,
          min_data_points: agent.settings.guardrail_min_data_points,
          rollback_on_decline_percent: agent.settings.guardrail_rollback_on_decline_percent
        },
        skillRuntime: parseSkillRuntimeForm(agent.settings.skill_runtime_json),
        intent_pass: {
          enabled: agent.settings.intent_pass_enabled !== false
        }
      });
      Object.assign(advancedState, {
        channel_id: agent.settings.channel_id || "",
        chat_id: agent.settings.chat_id || "",
        workspace: agent.settings.workspace || "",
        reasoning_mode: agent.settings.reasoning_mode || "provider_default",
        reasoning_level: agent.settings.reasoning_level || "off",
        context_compaction_enabled: agent.settings.context_compaction_enabled ?? false,
        session_summary_enabled: agent.settings.session_summary_enabled ?? false,
        truncate_strategy: agent.settings.l0_truncate_strategy || "sliding",
        recent_window_turns: agent.settings.l0_recent_window_turns ?? 20,
        recent_window_tokens: agent.settings.l0_recent_window_tokens ?? 0,
        summary_keep_turns: agent.settings.l0_summary_keep_turns ?? 4
      });
    } else {
      hydrateConfig(agent.config_json);
    }
    normalizeSkillRuntimeState();
    if (agent.files?.length) {
      hydrateFiles(agent.files);
    }
  }

  async function saveAgent() {
    if (!selectedProviderModelID.value) {
      $q.notify({ type: "negative", message: "请选择已录入且启用的模型" });
      return;
    }
    try {
      const updated = await detailStore.patch(form.id, {
        ...form,
        settings: buildSettingsPayload(),
        files: files.map((file, index) => ({ name: file.name, body: file.body, sort_order: (index + 1) * 10 })),
        config_json: JSON.stringify({
          self_evolve: config.self_evolve,
          subagents: config.subagents,
          tools: config.tools,
          memory: config.memory,
          memoryL0: config.memoryL0,
          memoryL1: config.memoryL1,
          memoryL2: config.memoryL2,
          memoryL3: config.memoryL3,
          memoryL4: config.memoryL4,
          evolutionSettings: config.evolutionSettings,
          heartbeat: config.heartbeat,
          evolution: config.evolution,
          evolution_guardrails: config.evolution_guardrails,
          skillRuntime: config.skillRuntime,
          intent_pass: config.intent_pass,
          files: files.map((file) => ({ name: file.name, body: file.body }))
        })
      });
      Object.assign(form, updated);
      hydrateSettings(updated);
      store.upsertAgent(updated);
      snapshotFiles();
      await loadPromptPreview();
      await primeAvatarThumbnailCacheForAgentIcon();
      $q.notify({ type: "positive", message: "已保存" });
    } catch (e) {
      $q.notify({ type: "negative", message: e instanceof Error ? e.message : "保存失败" });
    }
  }

  function onAdvancedSave(payload: {
    channel_id: string;
    chat_id: string;
    workspace: string;
    reasoning_mode: string;
    reasoning_level: string;
    context_compaction_enabled: boolean;
    session_summary_enabled: boolean;
    truncate_strategy: string;
    recent_window_turns: number;
    recent_window_tokens: number;
    summary_keep_turns: number;
  }) {
    Object.assign(advancedState, payload);
    config.memoryL0.recent_window_turns = payload.recent_window_turns;
    config.memoryL0.recent_window_tokens = payload.recent_window_tokens;
    config.memoryL0.summary_keep_turns = payload.summary_keep_turns;
    config.memoryL0.truncate_strategy = payload.truncate_strategy;
    saveAgent();
  }

  async function toggleFavorite() {
    const next = !form.is_favorite;
    form.is_favorite = next;
    try {
      const updated = await detailStore.patch(form.id, {
        ...form,
        is_favorite: next,
        settings: buildSettingsPayload(),
        files: files.map((file, index) => ({ name: file.name, body: file.body, sort_order: (index + 1) * 10 }))
      });
      Object.assign(form, updated);
      hydrateSettings(updated);
      store.upsertAgent(updated);
    } catch (error) {
      form.is_favorite = !next;
      $q.notify({ type: "negative", message: error instanceof Error ? error.message : "收藏保存失败" });
    }
  }

  async function loadPromptPreview() {
    if (!form.id) return;
    promptPreview.value = await detailStore.fetchPromptPreview(form.id, previewMode.value);
  }

  async function loadProviderModels() {
    loadingProviderModels.value = true;
    try {
      providerModels.value = await listPlatformResources("llm-provider-models");
    } finally {
      loadingProviderModels.value = false;
    }
  }

  function selectProviderModel(value: string | null) {
    const selected = providerModels.value.find((row) => row.id === value);
    if (!selected) {
      form.provider = "";
      form.model = "";
      return;
    }
    form.provider = selected.provider;
    form.model = selected.model;
  }

  function filterProviderModels(value: string, update: (callback: () => void) => void) {
    update(() => {
      providerModelSearch.value = value;
    });
  }

  function providerContextWindowK(row: PlatformResource) {
    try {
      const parsed = JSON.parse(row.config_json || "{}") as { context_window_k?: number | string | null };
      const value = Number(parsed.context_window_k);
      return Number.isFinite(value) && value > 0 ? value : null;
    } catch {
      return null;
    }
  }

  function reloadActiveFile() {
    activeFileBody.value = initialFileBodies.value[activeFile.value] ?? activeFileBody.value;
  }

  function updateFileBody(name: string, body: string) {
    const file = files.find((item) => item.name === name);
    if (file) file.body = body;
  }

  function snapshotFiles() {
    initialFileBodies.value = Object.fromEntries(files.map((file) => [file.name, file.body]));
  }

  function hydrateFiles(savedFiles: AgentPromptFile[]) {
    const byName = new Map(savedFiles.map((file) => [file.name, file]));
    for (const file of files) {
      const saved = byName.get(file.name);
      if (saved) file.body = saved.body;
    }
    for (const saved of savedFiles) {
      if (!files.some((file) => file.name === saved.name)) {
        files.push({ name: saved.name, caption: "自定义 Prompt 文件", body: saved.body });
      }
    }
  }

  function buildSettingsPayload(): AgentRuntimeSettings {
    return {
      self_evolve: config.self_evolve,
      subagents_enabled: config.subagents.enabled,
      subagents_max_concurrency: config.subagents.max_concurrency,
      subagents_max_generation_depth: config.subagents.max_generation_depth,
      subagents_max_children_per_agent: config.subagents.max_children_per_agent,
      subagents_archive_after_minutes: config.subagents.archive_after_minutes,
      subagents_max_retries: config.subagents.max_retries,
      subagents_model_override: config.subagents.model_override,
      tools_enabled: config.tools.enabled,
      tools_profile: config.tools.profile,
      tools_tool_call_prefix: config.tools.tool_call_prefix,
      tools_allow_json: JSON.stringify(config.tools.allow),
      tools_deny_json: JSON.stringify(config.tools.deny),
      tools_concurrent_allow_json: JSON.stringify(config.tools.concurrent_allow),
      tools_retry_enabled: config.tools.retry.enabled,
      tools_retry_max_attempts: config.tools.retry.max_attempts,
      tools_retry_initial_interval_ms: config.tools.retry.initial_interval_ms,
      tools_retry_backoff_factor: config.tools.retry.backoff_factor,
      tools_retry_max_interval_ms: config.tools.retry.max_interval_ms,
      tools_retry_jitter: config.tools.retry.jitter,
      tools_parallel_enabled: config.tools.parallel_enabled,
      tools_streaming_enabled: config.tools.streaming_enabled,
      memory_enabled: config.memory.enabled,
      memory_max_chunk_length: config.memory.max_chunk_length,
      memory_max_results: config.memory.max_results,
      memory_min_score: config.memory.min_score,
      l0_recent_window_turns: config.memoryL0.recent_window_turns,
      l0_recent_window_tokens: config.memoryL0.recent_window_tokens,
      l0_summary_threshold: config.memoryL0.summary_threshold,
      l0_summary_keep_turns: config.memoryL0.summary_keep_turns,
      l0_truncate_strategy: config.memoryL0.truncate_strategy,
      l0_inject_l1: config.memoryL0.inject_l1,
      l0_inject_l3: config.memoryL0.inject_l3,
      l0_inject_l4: config.memoryL0.inject_l4,
      l0_l3_max_chunks: config.memoryL0.l3_max_chunks,
      l0_l4_max_paths: config.memoryL0.l4_max_paths,
      l0_snapshot_mode: config.memoryL0.snapshot_mode,
      l1_enabled: config.memoryL1.enabled,
      l1_budget_tokens: config.memoryL1.budget_tokens,
      l1_field_max_tokens: config.memoryL1.field_max_tokens,
      l1_history_keep_revisions: config.memoryL1.history_keep_revisions,
      l1_default_schema_id: config.memoryL1.default_schema_id,
      l1_archive_on_idle_minutes: config.memoryL1.archive_on_idle_minutes,
      l2_episode_enabled: config.memoryL2.episode_enabled,
      l2_episode_min_importance: config.memoryL2.episode_min_importance,
      l2_index_enabled: config.memoryL2.index_enabled,
      l2_index_embedding_model: config.memoryL2.index_embedding_model,
      l2_recall_enabled: config.memoryL2.recall_enabled,
      l2_recall_max: config.memoryL2.recall_max,
      l2_retention_days: config.memoryL2.retention_days,
      l2_archive_after_days: config.memoryL2.archive_after_days,
      l3_enabled: config.memoryL3.enabled,
      l3_recall_top_k: config.memoryL3.recall_top_k,
      l3_recall_min_score: config.memoryL3.recall_min_score,
      l3_recall_scopes_json: JSON.stringify(config.memoryL3.recall_scopes),
      l3_embedding_model: config.memoryL3.embedding_model,
      l3_decay_interval_hours: config.memoryL3.decay_interval_hours,
      l3_archive_threshold: config.memoryL3.archive_threshold,
      l3_max_per_recall_chars: config.memoryL3.max_per_recall_chars,
      l4_enabled: config.memoryL4.enabled,
      l4_graph_inject_neighbors: config.memoryL4.graph_inject_neighbors,
      l4_graph_max_neighbors: config.memoryL4.graph_max_neighbors,
      l4_graph_max_hops: config.memoryL4.graph_max_hops,
      l4_identity_inject: config.memoryL4.identity_inject,
      l4_strategy_inject: config.memoryL4.strategy_inject,
      evo_enabled: config.evolutionSettings.enabled,
      evo_auto_apply: config.evolutionSettings.auto_apply,
      evo_min_episodes: config.evolutionSettings.min_episodes,
      evo_min_negative_feedback: config.evolutionSettings.min_negative_feedback,
      evo_throttle_hours: config.evolutionSettings.throttle_hours,
      evo_proposal_ttl_days: config.evolutionSettings.proposal_ttl_days,
      evo_persona_max_chars: config.evolutionSettings.persona_max_chars,
      evo_system_prompt_max_appends: config.evolutionSettings.system_prompt_max_appends,
      heartbeat_enabled: config.heartbeat.enabled,
      heartbeat_interval_minutes: config.heartbeat.interval_minutes,
      evolution_self_evolve: config.evolution.self_evolve,
      evolution_skill_evolve: config.evolution.skill_evolve,
      evolution_metrics_enabled: config.evolution.evolution_metrics_enabled,
      evolution_suggestions_enabled: config.evolution.evolution_suggestions_enabled,
      guardrail_max_change_per_period: config.evolution_guardrails.max_change_per_period,
      guardrail_min_data_points: config.evolution_guardrails.min_data_points,
      guardrail_rollback_on_decline_percent: config.evolution_guardrails.rollback_on_decline_percent,
      skill_runtime_json: stringifySkillRuntimeJSON(),
      intent_pass_enabled: config.intent_pass.enabled,
      channel_id: advancedState.channel_id,
      chat_id: advancedState.chat_id,
      workspace: advancedState.workspace,
      reasoning_mode: advancedState.reasoning_mode,
      reasoning_level: advancedState.reasoning_level,
      context_compaction_enabled: advancedState.context_compaction_enabled,
      session_summary_enabled: advancedState.session_summary_enabled
    };
  }

  function parseJSONList(raw: string) {
    try {
      const parsed = JSON.parse(raw || "[]");
      return Array.isArray(parsed) ? parsed.map(String) : [];
    } catch {
      return [];
    }
  }

  function applyAiEditPlaceholder() {
    if (!aiInstruction.value.trim()) return;
    activeFileBody.value = `${activeFileBody.value}\n\n<!-- AI edit instruction: ${aiInstruction.value.trim()} -->`;
    aiInstruction.value = "";
    aiEditOpen.value = false;
  }

  async function copyKey() {
    await copyToClipboard(form.agent_key);
    $q.notify({ type: "positive", message: "Agent 标识已复制" });
  }

  async function reloadAgent() {
    const id = String(route.params.id ?? "").trim();
    if (!id) return;
    try {
      const agent = await detailStore.fetchById(id);
      await applyLoadedAgent(agent);
    } catch (e) {
      $q.notify({ type: "negative", message: e instanceof Error ? e.message : "刷新 Agent 失败" });
    }
  }

  return {
    tab,
    form,
    config,
    saving,
    router,
    avatarPickerOpen,
    promptDialog,
    advancedDialog,
    toggleFavorite,
    reloadAgent,
    saveAgent,
    promptModes,
    statusOptions,
    copyKey,
    selectedProviderModelID,
    filteredProviderModelOptions,
    loadingProviderModels,
    filterProviderModels,
    selectProviderModel,
    budgetUSD,
    toolProfileOptions,
    toolSelectOptions,
    loadingCatalogTools,
    toolConflicts,
    agentId,
    heartbeatFile,
    activeFile,
    fileSplitter,
    files,
    fileDirty,
    updateFileBody,
    reloadActiveFile,
    aiEditOpen,
    truncateStrategyOptions,
    snapshotModeOptions,
    memoryScopeOptions,
    loadSkillSlugOptions,
    resetSkillRuntimeDefaults,
    loadingSkillSlugs,
    skillSlugOptions,
    evolutionRange,
    tokenEstimateFor,
    previewMode,
    promptPreview,
    aiInstruction,
    applyAiEditPlaceholder,
    advancedState,
    onAdvancedSave
  };
}
