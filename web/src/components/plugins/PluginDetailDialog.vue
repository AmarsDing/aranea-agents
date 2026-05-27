<template>
  <q-dialog :model-value="open" @update:model-value="$emit('update:open', $event)">
    <q-card class="plugin-detail-card app-dialog-card app-dialog-card--md app-glass-dialog">
      <q-card-section class="app-glass-dialog__head row items-start justify-between q-gutter-md">
        <div class="min-width-0">
          <div class="app-glass-dialog__title">{{ target?.name }}</div>
          <div class="app-glass-dialog__subtitle">{{ target?.key }}</div>
        </div>
        <q-btn flat dense round icon="close" aria-label="关闭详情" v-close-popup />
      </q-card-section>
      <q-separator />
      <q-card-section v-if="target" class="app-dialog-body q-gutter-md">
        <p class="plugin-detail-desc">{{ target.description || "暂无说明" }}</p>

        <div class="plugin-detail-metrics row q-col-gutter-sm">
          <div v-for="metric in metrics" :key="metric.label" class="col-6 col-sm-4">
            <div class="plugin-detail-metric">
              <span class="plugin-detail-metric__label">{{ metric.label }}</span>
              <span v-if="metric.tagClass" class="plugin-tag" :class="metric.tagClass">{{ metric.value }}</span>
              <span v-else class="plugin-detail-metric__value">{{ metric.value }}</span>
            </div>
          </div>
        </div>

        <div v-if="target.permissions?.can_edit_config" class="row q-gutter-sm">
          <q-btn outline dense no-caps icon="arrow_upward" label="上移顺序" @click="$emit('bumpSort', -10)" />
          <q-btn outline dense no-caps icon="arrow_downward" label="下移顺序" @click="$emit('bumpSort', 10)" />
        </div>

        <q-expansion-item dense-toggle label="Agent 绑定">
          <div class="q-gutter-sm">
            <q-radio :model-value="scopeMode" val="global" label="全局生效" @update:model-value="$emit('update:scopeMode', $event as 'global' | 'agent')" />
            <q-radio :model-value="scopeMode" val="agent" label="指定 Agent" @update:model-value="$emit('update:scopeMode', $event as 'global' | 'agent')" />
            <q-input
              v-if="scopeMode === 'agent'"
              :model-value="scopeAgentId"
              dense
              outlined
              label="Agent ID"
              @update:model-value="$emit('update:scopeAgentId', String($event ?? ''))"
            />
            <q-btn color="primary" rounded unelevated no-caps label="保存作用域" :loading="savingScope" @click="$emit('saveScope')" />
          </div>
        </q-expansion-item>

        <q-expansion-item dense-toggle default-opened label="Callback">
          <div class="app-registry-chip-wrap">
            <span v-for="point in target.callback_points" :key="point" class="plugin-tag plugin-tag--callback">{{ point }}</span>
            <span v-if="!target.callback_points.length" class="text-grey-7">暂无 Callback</span>
          </div>
        </q-expansion-item>

        <q-expansion-item dense-toggle label="配置 JSON">
          <pre class="app-code-block app-code-block--compact">{{ prettyJSON(target.config_json, "暂无配置") }}</pre>
        </q-expansion-item>
        <q-expansion-item dense-toggle label="默认配置">
          <pre class="app-code-block app-code-block--compact">{{ prettyJSON(target.default_config_json, "暂无默认配置") }}</pre>
        </q-expansion-item>
        <q-expansion-item dense-toggle label="配置 Schema">
          <pre class="app-code-block app-code-block--compact">{{ prettyJSON(target.config_schema_json, "暂无 Schema") }}</pre>
        </q-expansion-item>
      </q-card-section>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { Plugin } from "../../features/plugins/types";
import { formatPluginDate, lastStatusLabel, prettyJSON, riskTagClass } from "./pluginUi";

const props = defineProps<{
  open: boolean;
  target: Plugin | null;
  scopeMode: "global" | "agent";
  scopeAgentId: string;
  savingScope: boolean;
}>();

defineEmits<{
  "update:open": [value: boolean];
  "update:scopeMode": [value: "global" | "agent"];
  "update:scopeAgentId": [value: string];
  bumpSort: [delta: number];
  saveScope: [];
}>();

const metrics = computed(() => {
  const target = props.target;
  if (!target) return [];
  return [
    { label: "类型", value: target.category },
    { label: "风险", value: target.risk_level, tagClass: riskTagClass(target.risk_level) },
    { label: "作用域", value: target.scope || "global" },
    { label: "排序", value: String(target.sort_order) },
    { label: "调用次数", value: String(target.invoke_count) },
    { label: "阻断 / 错误", value: `${target.block_count} / ${target.error_count}` },
    { label: "最近状态", value: lastStatusLabel(target) },
    { label: "最近调用", value: formatPluginDate(target.last_invoked_at) }
  ];
});
</script>
