<template>
  <q-card flat class="overview-panel overview-panel--warning">
    <q-card-section>
      <div class="row items-center q-gutter-sm">
        <q-icon name="swap_horiz" class="overview-panel__alert-icon" />
        <div>
          <div class="text-h6 overview-section-title">模型降级事件</div>
          <div class="text-caption overview-section-caption">Failover / Hedge 切换与费用保护回退</div>
        </div>
      </div>
    </q-card-section>
    <q-separator class="overview-separator" />
    <q-card-section>
      <div v-if="!events.length" class="overview-empty overview-empty--compact">暂无降级事件</div>
      <q-list v-else dense class="overview-rank-list">
        <q-item v-for="(evt, idx) in events" :key="idx">
          <q-item-section avatar class="overview-rank-index">
            <q-icon :name="evt.icon" size="18px" :color="evt.iconColor" />
          </q-item-section>
          <q-item-section>
            <q-item-label>{{ evt.label }}</q-item-label>
            <q-item-label caption class="overview-item-caption">{{ evt.detail }}</q-item-label>
          </q-item-section>
          <q-item-section side>
            <span class="text-caption overview-item-caption">{{ evt.time }}</span>
          </q-item-section>
        </q-item>
      </q-list>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { ModelTokenUsageEvent } from "../../features/usage/types";

const props = defineProps<{
  anomalies: ModelTokenUsageEvent[];
}>();

type FallbackEvent = {
  icon: string;
  iconColor: string;
  label: string;
  detail: string;
  time: string;
};

const events = computed<FallbackEvent[]>(() => {
  return props.anomalies
    .filter((e) => e.status === "failed" || e.status === "timeout" || e.retry_count > 0)
    .slice(0, 10)
    .map((e) => {
      const isRetry = (e.retry_count ?? 0) > 0;
      const isTimeout = e.status === "timeout";
      return {
        icon: isRetry ? "autorenew" : isTimeout ? "schedule" : "error_outline",
        iconColor: isRetry ? "warning" : "negative",
        label: `${e.provider_code} / ${e.model_display_name || e.model_api_id}`,
        detail: isRetry
          ? `重试 ${e.retry_count} 次 · ${e.agent_key || "未知 Agent"}`
          : `${e.status} · ${e.error_message?.slice(0, 60) || "无错误信息"}`,
        time: formatTime(e.occurred_at)
      };
    });
});

function formatTime(value: string) {
  if (!value) return "—";
  try {
    return new Date(value).toLocaleString();
  } catch {
    return value;
  }
}
</script>
