<template>
  <q-card flat class="overview-panel overview-panel--warning">
    <q-card-section>
      <div class="row items-center q-gutter-sm">
        <q-icon name="speed" class="overview-panel__alert-icon" />
        <div>
          <div class="text-h6 overview-section-title">低性价比模型</div>
          <div class="text-caption overview-section-caption">高费用且低 TPS 或成功率偏低的模型</div>
        </div>
      </div>
    </q-card-section>
    <q-separator class="overview-separator" />
    <q-card-section>
      <div v-if="!rows.length" class="overview-empty overview-empty--compact">当前筛选范围内暂无标记</div>
      <q-list v-else dense class="overview-rank-list">
        <q-item v-for="row in rows" :key="`${row.provider_code}:${row.model_api_id}`">
          <q-item-section>
            <q-item-label>{{ row.model_display_name || row.model_api_id }}</q-item-label>
            <q-item-label caption class="overview-item-caption">
              {{ row.provider_code }} · {{ formatMoney(row.total_cost_micro_usd) }}
            </q-item-label>
          </q-item-section>
          <q-item-section side>
            <div class="row q-gutter-xs justify-end">
              <q-chip
                v-for="f in row.flags"
                :key="f"
                dense
                size="sm"
                outline
                class="overview-flag-chip"
              >
                {{ flagLabel(f) }}
              </q-chip>
            </div>
          </q-item-section>
        </q-item>
      </q-list>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import type { UsageModelInsight } from "../../features/usage/types";
import { formatUsdFromMicro } from "../../features/usage/moneyFormat";

defineProps<{
  rows: UsageModelInsight[];
}>();

function formatMoney(value?: number) {
  return formatUsdFromMicro(value);
}

function flagLabel(f: string) {
  switch (f) {
    case "low_tps":
      return "低 TPS";
    case "high_failure":
      return "高失败率";
    case "high_cost":
      return "高费用";
    default:
      return f;
  }
}
</script>
