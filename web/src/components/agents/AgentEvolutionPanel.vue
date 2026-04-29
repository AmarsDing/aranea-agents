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
      <q-banner rounded class="bg-blue-1 text-blue-10 q-mt-md">
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
      <div class="row q-col-gutter-md q-mt-sm">
        <div v-for="card in metricCards" :key="card.title" class="col-12 col-md-4">
          <q-card flat bordered class="metric-empty">
            <q-card-section>
              <q-icon :name="card.icon" color="primary" size="26px" />
              <div class="text-subtitle2 q-mt-sm">{{ card.title }}</div>
              <div class="text-caption text-grey-7">{{ card.caption }}</div>
            </q-card-section>
          </q-card>
        </div>
      </div>
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
import { computed } from "vue";
import type { EvolutionKey } from "./agentUi";

const props = defineProps<{
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
const metricCards = [
  { title: "工具成功率", icon: "query_stats", caption: "在处理足够请求后展示工具调用成功率。" },
  { title: "检索质量", icon: "travel_explore", caption: "记忆检索与向量命中质量将在这里展示。" },
  { title: "建议", icon: "tips_and_updates", caption: "每日分析任务生成后展示改进建议。" }
];
</script>

<style scoped>
.evolution-panel {
  display: grid;
  gap: 16px;
}

.settings-section {
  padding: 20px;
  border: 1px solid rgba(0, 0, 0, 0.08);
  border-radius: 22px;
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.98), rgba(255, 255, 255, 0.92)),
    radial-gradient(circle at top right, rgba(25, 118, 210, 0.06), transparent 32%);
  box-shadow: 0 14px 36px rgba(16, 24, 40, 0.045);
}

.section-title-row {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 14px;
}

.evolution-list {
  border: 1px solid rgba(15, 23, 42, 0.06);
  border-radius: 16px;
  overflow: hidden;
  background: #fbfcff;
}

.evolution-item {
  min-height: 68px;
}

.metric-empty {
  min-height: 136px;
  border-color: rgba(15, 23, 42, 0.08);
  border-radius: 18px;
  background: linear-gradient(180deg, #ffffff, #fbfcff);
  transition:
    transform 180ms ease,
    box-shadow 180ms ease,
    border-color 180ms ease;
}

.metric-empty:hover {
  transform: translateY(-2px);
  border-color: rgba(25, 118, 210, 0.24);
  box-shadow: 0 16px 34px rgba(16, 24, 40, 0.08);
}

.guardrails-section {
  border-color: rgba(34, 197, 94, 0.18);
  background: linear-gradient(135deg, #f7fff8, #ffffff);
}
</style>
