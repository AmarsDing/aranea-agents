<template>
  <div class="evolution-panel">
    <section class="settings-section">
      <div class="section-title-row">
        <div>
          <div class="text-subtitle1 text-weight-bold">进化</div>
          <div class="text-caption text-grey-7">控制风格进化、技能进化、指标采集与建议生成。</div>
        </div>
      </div>
      <q-list separator class="evolution-list">
        <q-item v-for="item in evolutionToggles" :key="item.key" class="evolution-item">
          <q-item-section>
            <q-item-label>{{ item.title }}</q-item-label>
            <q-item-label caption>{{ item.caption }}</q-item-label>
          </q-item-section>
          <q-item-section side><q-toggle v-model="evolution[item.key]" color="primary" /></q-item-section>
        </q-item>
      </q-list>
      <q-banner rounded class="evolution-info-banner q-mt-md">
        仅允许进化 SOUL.md 中的沟通风格与语调；身份、核心目的、AGENTS* 操作规则保持锁定。
      </q-banner>
    </section>

    <section class="settings-section q-mt-md">
      <div class="row items-center justify-between">
        <div>
          <div class="text-subtitle1 text-weight-bold">指标与建议</div>
          <div class="text-caption text-grey-7">时间范围只影响看板读取，不写入 Agent 配置。</div>
        </div>
        <q-btn-toggle v-model="rangeModel" rounded unelevated toggle-color="primary" :options="rangeOptions" />
      </div>
      <q-inner-loading :showing="metricsLoading" label="加载指标..." />
      <div v-if="!metricsLoading" class="row q-col-gutter-md q-mt-sm">
        <div class="col-12 col-md-4">
          <q-card flat bordered class="metric-card">
            <q-card-section>
              <div class="row items-center q-gutter-sm">
                <q-icon name="query_stats" color="primary" size="26px" />
                <div class="text-subtitle2">工具成功率</div>
              </div>
              <div class="text-h5 q-mt-sm">{{ formatPercent(metrics?.tool_success_rate) }}</div>
              <div class="text-caption text-grey-7">共 {{ metrics?.total_episodes ?? 0 }} 个会话</div>
            </q-card-section>
          </q-card>
        </div>
        <div class="col-12 col-md-4">
          <q-card flat bordered class="metric-card">
            <q-card-section>
              <div class="row items-center q-gutter-sm">
                <q-icon name="travel_explore" color="primary" size="26px" />
                <div class="text-subtitle2">检索质量</div>
              </div>
              <div class="text-h5 q-mt-sm">{{ formatPercent(metrics?.retrieval_quality) }}</div>
              <div class="text-caption text-grey-7">记忆工具调用成功率</div>
            </q-card-section>
          </q-card>
        </div>
        <div class="col-12 col-md-4">
          <q-card flat bordered class="metric-card">
            <q-card-section>
              <div class="row items-center q-gutter-sm">
                <q-icon name="tips_and_updates" color="primary" size="26px" />
                <div class="text-subtitle2">建议</div>
              </div>
              <div class="text-h5 q-mt-sm">{{ pendingSuggestionsCount }}</div>
              <div class="text-caption text-grey-7">待处理改进建议</div>
            </q-card-section>
          </q-card>
        </div>
      </div>
    </section>

    <section v-if="suggestions.length > 0" class="settings-section q-mt-md">
      <div class="section-title-row">
        <div>
          <div class="text-subtitle1 text-weight-bold">进化建议列表</div>
          <div class="text-caption text-grey-7">基于指标自动生成的改进建议，可应用或拒绝。</div>
        </div>
      </div>
      <q-list separator class="suggestion-list">
        <q-item v-for="s in suggestions" :key="s.id" class="suggestion-item">
          <q-item-section>
            <q-item-label class="text-weight-medium">
              <q-badge :color="suggestionTypeColor(s.type)" class="q-mr-sm" :label="s.type" />
              {{ s.title }}
            </q-item-label>
            <q-item-label caption class="q-mt-xs">{{ s.content }}</q-item-label>
            <q-item-label caption class="q-mt-xs text-grey-5">{{ formatDate(s.created_at) }}</q-item-label>
          </q-item-section>
          <q-item-section side>
            <div v-if="s.status === 'pending'" class="row q-gutter-xs">
              <q-btn flat round dense icon="check" color="positive" size="sm" :loading="applyingId === s.id" @click="onApply(s.id)">
                <q-tooltip>应用</q-tooltip>
              </q-btn>
              <q-btn flat round dense icon="close" color="negative" size="sm" :loading="rejectingId === s.id" @click="onReject(s.id)">
                <q-tooltip>拒绝</q-tooltip>
              </q-btn>
            </div>
            <q-badge v-else :color="s.status === 'applied' ? 'positive' : 'grey'" :label="s.status" />
          </q-item-section>
        </q-item>
      </q-list>
    </section>

    <section class="settings-section guardrails-section q-mt-md">
      <div class="row items-center q-gutter-sm">
        <q-avatar rounded color="green-1" text-color="green-8" icon="hexagon" />
        <div>
          <div class="text-subtitle1 text-weight-bold">适应护栏</div>
          <div class="text-caption text-grey-7">限制自动调整幅度，样本不足或表现下降时回滚。</div>
        </div>
      </div>
      <div class="row q-col-gutter-md q-mt-xs">
        <q-input v-model.number="guardrails.max_change_per_period" class="col-12 col-md-4" dense outlined type="number" step="0.01" label="每周期最大变化" />
        <q-input v-model.number="guardrails.min_data_points" class="col-12 col-md-4" dense outlined type="number" label="最少数据点" />
        <q-input v-model.number="guardrails.rollback_on_decline_percent" class="col-12 col-md-4" dense outlined type="number" suffix="%" label="下降时回滚" />
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import type { EvolutionKey } from "./agentUi";
import type { EvolutionMetrics, EvolutionSuggestion } from "../../features/agents/api";
import { useAgentDetailStore } from "../../stores/agents/detail";

const agentDetailStore = useAgentDetailStore();

const props = defineProps<{
  agentId: string;
  evolution: Record<EvolutionKey, boolean>;
  guardrails: {
    max_change_per_period: number;
    min_data_points: number;
    rollback_on_decline_percent: number;
  };
  range: string;
}>();

const emit = defineEmits<{
  "update:range": [value: string];
}>();

const rangeModel = computed({
  get: () => props.range,
  set: (value: string) => emit("update:range", value)
});

const rangeOptions = ["7d", "30d", "90d"].map((value) => ({ label: value, value }));
const evolutionToggles: Array<{ key: EvolutionKey; title: string; caption: string }> = [
  { key: "self_evolve", title: "允许 Agent 进化其沟通风格", caption: "允许随时间更新 SOUL.md 中的语调与风格。" },
  { key: "skill_evolve", title: "允许从经验中创建和管理技能", caption: "提示用户将有效工作流保存为 Skill。" },
  { key: "evolution_metrics_enabled", title: "进化指标", caption: "记录工具效果、检索质量、反馈等。" },
  { key: "evolution_suggestions_enabled", title: "进化建议", caption: "基于指标生成改进建议。" }
];

const metricsLoading = ref(false);
const metrics = ref<EvolutionMetrics | null>(null);
const suggestions = ref<EvolutionSuggestion[]>([]);
const applyingId = ref<string | null>(null);
const rejectingId = ref<string | null>(null);

const pendingSuggestionsCount = computed(() => suggestions.value.filter((s) => s.status === "pending").length);

function formatPercent(v: number | undefined): string {
  if (v === undefined || v === null) return "—";
  return (v * 100).toFixed(1) + "%";
}

function formatDate(iso: string): string {
  if (!iso) return "";
  try {
    return new Date(iso).toLocaleDateString();
  } catch {
    return iso;
  }
}

function suggestionTypeColor(type: string): string {
  switch (type) {
    case "persona":
      return "purple";
    case "prompt":
      return "blue";
    case "skill":
      return "teal";
    default:
      return "grey";
  }
}

async function fetchMetrics() {
  if (!props.agentId) return;
  metricsLoading.value = true;
  try {
    metrics.value = await agentDetailStore.fetchEvolutionMetrics(props.agentId, props.range);
  } catch {
    metrics.value = null;
  } finally {
    metricsLoading.value = false;
  }
}

async function fetchSuggestions() {
  if (!props.agentId) return;
  try {
    suggestions.value = await agentDetailStore.fetchEvolutionSuggestions(props.agentId, "pending");
  } catch {
    suggestions.value = [];
  }
}

async function onApply(id: string) {
  applyingId.value = id;
  try {
    await agentDetailStore.applyEvolution(props.agentId, id);
    await fetchSuggestions();
    await fetchMetrics();
  } finally {
    applyingId.value = null;
  }
}

async function onReject(id: string) {
  rejectingId.value = id;
  try {
    await agentDetailStore.rejectEvolution(props.agentId, id);
    await fetchSuggestions();
  } finally {
    rejectingId.value = null;
  }
}

watch(
  () => [props.agentId, props.range],
  () => {
    fetchMetrics();
    fetchSuggestions();
  },
  { immediate: true }
);
</script>

<style scoped>
.evolution-panel {
  display: grid;
  gap: 16px;
}

.settings-section {
  padding: 20px;
  border: 1px solid var(--glass-border);
  border-radius: 24px;
  background: var(--glass-surface);
  backdrop-filter: blur(var(--glass-blur-default));
  -webkit-backdrop-filter: blur(var(--glass-blur-default));
  box-shadow: none;
}

.section-title-row {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 14px;
}

.evolution-info-banner {
  border: 1px solid var(--glass-border);
  background: var(--glass-elevated);
  color: var(--color-text-primary);
  box-shadow: var(--glass-inner-highlight);
}

.evolution-list {
  border: 1px solid var(--glass-border);
  border-radius: 16px;
  overflow: hidden;
  background: var(--glass-elevated);
  backdrop-filter: blur(var(--glass-blur-default));
  -webkit-backdrop-filter: blur(var(--glass-blur-default));
}

.evolution-item {
  min-height: 68px;
}

.metric-card {
  min-height: 136px;
  border-color: var(--glass-border);
  border-radius: 18px;
  background: var(--glass-elevated);
  backdrop-filter: blur(var(--glass-blur-default));
  -webkit-backdrop-filter: blur(var(--glass-blur-default));
  box-shadow: none;
  transition:
    transform 180ms ease,
    border-color 180ms ease,
    background 180ms ease;
}

.metric-card:hover {
  transform: translateY(-2px);
  border-color: var(--glass-border-hover, var(--glass-border));
  background: var(--glass-surface-hover);
}

.suggestion-list {
  border: 1px solid var(--glass-border);
  border-radius: 16px;
  overflow: hidden;
  background: var(--glass-elevated);
  backdrop-filter: blur(var(--glass-blur-default));
  -webkit-backdrop-filter: blur(var(--glass-blur-default));
}

.suggestion-item {
  min-height: 80px;
}

.guardrails-section {
  border-color: color-mix(in srgb, var(--color-success) 28%, var(--glass-border));
  background: color-mix(in srgb, var(--color-success) 9%, var(--glass-surface));
}

body.body--dark .settings-section {
  box-shadow: none;
}

body.body--dark .evolution-info-banner {
  background: var(--glass-elevated);
  border-color: var(--glass-border);
}

body.body--dark .guardrails-section {
  border-color: color-mix(in srgb, var(--color-success) 35%, var(--glass-border));
  background: color-mix(in srgb, var(--color-success) 12%, var(--glass-surface));
}
</style>
