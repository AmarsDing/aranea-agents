<template>
  <q-card flat bordered class="monitor-card">
    <q-card-section class="row items-center q-gutter-sm">
      <div class="text-h6 text-weight-bold">Usage 总览</div>
      <q-space />
      <q-select v-model="range" dense outlined emit-value map-options :options="rangeOptions" label="时间范围" style="min-width: 140px" />
      <q-btn flat rounded icon="refresh" label="刷新" :loading="loading" @click="loadOverview" />
    </q-card-section>
    <q-separator />
    <q-card-section v-if="overview">
      <div class="row q-col-gutter-md q-mb-md">
        <div class="col-6 col-md-3">
          <q-card flat bordered class="q-pa-sm">
            <div class="text-caption text-grey">今日请求</div>
            <div class="text-h5 text-weight-bold">{{ formatCount(overview.today.request_count) }}</div>
            <div class="text-caption">成功 {{ formatCount(overview.today.success_count) }} / 失败 {{ formatCount(overview.today.failed_count) }}</div>
          </q-card>
        </div>
        <div class="col-6 col-md-3">
          <q-card flat bordered class="q-pa-sm">
            <div class="text-caption text-grey">成功率</div>
            <div class="text-h5 text-weight-bold">{{ formatPercent(overview.today.success_rate) }}</div>
            <div class="text-caption">平均延迟 {{ formatLatency(overview.today.avg_latency_ms) }}</div>
          </q-card>
        </div>
        <div class="col-6 col-md-3">
          <q-card flat bordered class="q-pa-sm">
            <div class="text-caption text-grey">今日 Token</div>
            <div class="text-h5 text-weight-bold">{{ formatCount(overview.today.total_tokens) }}</div>
            <div class="text-caption">输入 {{ formatCount(overview.today.input_tokens) }} / 输出 {{ formatCount(overview.today.output_tokens) }}</div>
          </q-card>
        </div>
        <div class="col-6 col-md-3">
          <q-card flat bordered class="q-pa-sm">
            <div class="text-caption text-grey">今日费用</div>
            <div class="text-h5 text-weight-bold">{{ formatMoney(overview.today.total_cost_micro_usd) }}</div>
            <div class="text-caption">本月 {{ formatMoney(overview.month.total_cost_micro_usd) }}</div>
          </q-card>
        </div>
      </div>

      <div class="row q-col-gutter-md q-mb-md">
        <div class="col-12 col-md-6">
          <div class="text-subtitle2 text-weight-bold q-mb-sm">Top 模型</div>
          <q-list dense separator>
            <q-item v-for="model in overview.top_models" :key="model.provider_code + model.model_api_id">
              <q-item-section>
                <q-item-label>{{ model.provider_code }} / {{ model.model_display_name || model.model_api_id }}</q-item-label>
                <q-item-label caption>{{ formatCount(model.call_count) }} 次调用 · {{ formatMoney(model.total_cost_micro_usd) }}</q-item-label>
              </q-item-section>
              <q-item-section side>
                <q-badge :color="model.success_rate >= 0.95 ? 'positive' : model.success_rate >= 0.8 ? 'orange' : 'negative'">
                  {{ formatPercent(model.success_rate) }}
                </q-badge>
              </q-item-section>
            </q-item>
            <q-item v-if="!overview.top_models.length">
              <q-item-section class="text-grey">暂无数据</q-item-section>
            </q-item>
          </q-list>
        </div>
        <div class="col-12 col-md-6">
          <div class="text-subtitle2 text-weight-bold q-mb-sm">Top Agent</div>
          <q-list dense separator>
            <q-item v-for="agent in overview.top_agents" :key="agent.agent_id + agent.agent_key">
              <q-item-section>
                <q-item-label>{{ agent.agent_key || agent.agent_id }}</q-item-label>
                <q-item-label caption>{{ formatCount(agent.call_count) }} 次调用 · {{ formatCount(agent.total_tokens) }} tokens</q-item-label>
              </q-item-section>
              <q-item-section side>
                <q-badge :color="agent.success_rate >= 0.95 ? 'positive' : agent.success_rate >= 0.8 ? 'orange' : 'negative'">
                  {{ formatPercent(agent.success_rate) }}
                </q-badge>
              </q-item-section>
            </q-item>
            <q-item v-if="!overview.top_agents.length">
              <q-item-section class="text-grey">暂无数据</q-item-section>
            </q-item>
          </q-list>
        </div>
      </div>

      <div v-if="overview.anomalies.length" class="q-mb-md">
        <div class="text-subtitle2 text-weight-bold q-mb-sm">最近异常</div>
        <q-list dense separator>
          <q-item v-for="anomaly in overview.anomalies.slice(0, 5)" :key="anomaly.id">
            <q-item-section avatar>
              <q-icon name="warning" color="negative" />
            </q-item-section>
            <q-item-section>
              <q-item-label>{{ anomaly.agent_key || anomaly.agent_id }} · {{ anomaly.provider_code }} / {{ anomaly.model_api_id }}</q-item-label>
              <q-item-label caption class="text-negative">{{ anomaly.error_message || anomaly.status }}</q-item-label>
            </q-item-section>
            <q-item-section side class="text-caption">{{ formatDate(anomaly.occurred_at) }}</q-item-section>
          </q-item>
        </q-list>
      </div>
    </q-card-section>
    <q-card-section v-else-if="loading">
      <q-skeleton type="rect" height="200px" />
    </q-card-section>
    <q-card-section v-else>
      <div class="text-grey text-center q-pa-lg">暂无 Usage 数据</div>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { onMounted, ref, watch } from "vue";
import { getModelUsageOverview } from "../../features/usage/api";
import type { ModelUsageOverview } from "../../features/usage/types";
import { formatCount, formatLatency, formatMoney, formatPercent, formatDate } from "../../features/monitor/utils";

const range = ref("30d");
const loading = ref(false);
const overview = ref<ModelUsageOverview | null>(null);

const rangeOptions = [
  { label: "今日", value: "today" },
  { label: "7 天", value: "7d" },
  { label: "30 天", value: "30d" },
  { label: "本月", value: "month" }
];

onMounted(loadOverview);
watch(range, loadOverview);

async function loadOverview() {
  loading.value = true;
  try {
    overview.value = await getModelUsageOverview({ range: range.value });
  } catch {
    overview.value = null;
  } finally {
    loading.value = false;
  }
}
</script>
