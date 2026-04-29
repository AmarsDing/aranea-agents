<template>
  <div v-if="tool" class="tool-detail-content column q-gutter-md">
    <q-banner rounded class="tool-detail-banner">{{ tool.description || "暂无描述" }}</q-banner>
    <div class="row q-col-gutter-sm text-body2">
      <div class="col-6">
        <span class="text-weight-medium">分类：</span>{{ tool.category }}
      </div>
      <div class="col-6">
        <span class="text-weight-medium">来源：</span>{{ tool.source }}
      </div>
      <div class="col-6">
        <span class="text-weight-medium">风险：</span>{{ riskLabel(tool.risk_level) }}
      </div>
      <div class="col-6">
        <span class="text-weight-medium">运行时：</span>{{ runtimeStatusLabel(tool.runtime_status) }}
      </div>
      <div class="col-6">
        <span class="text-weight-medium">调用次数：</span>{{ tool.invoke_count }}
      </div>
      <div class="col-6">
        <span class="text-weight-medium">成功 / 失败：</span>{{ tool.success_count }} / {{ tool.failure_count }}
      </div>
    </div>
    <q-expansion-item dense-toggle default-open label="参数 Schema">
      <tool-json-block class="q-mt-sm" :text="prettyJSON(tool.parameters_schema_json)" />
    </q-expansion-item>
    <q-expansion-item dense-toggle label="返回 Schema">
      <tool-json-block class="q-mt-sm" :text="prettyJSON(tool.result_schema_json)" />
    </q-expansion-item>
    <q-expansion-item dense-toggle label="配置 JSON">
      <tool-json-block class="q-mt-sm" :text="prettyJSON(tool.config_json)" />
    </q-expansion-item>
    <q-expansion-item dense-toggle label="默认配置">
      <tool-json-block class="q-mt-sm" :text="prettyJSON(tool.default_config_json)" />
    </q-expansion-item>
    <q-expansion-item dense-toggle label="元数据">
      <tool-json-block class="q-mt-sm" :text="prettyJSON(tool.metadata_json)" />
    </q-expansion-item>
    <q-btn
      flat
      no-caps
      class="tool-accent-btn"
      icon="history"
      label="查看调用记录"
      :to="{ name: 'tool-runs', query: { tool_key: tool.key } }"
      v-close-popup
    />
  </div>
</template>

<script setup lang="ts">
import type { Tool } from "../../features/tools/types";
import ToolJsonBlock from "./ToolJsonBlock.vue";
import { prettyJSON, riskLabel, runtimeStatusLabel } from "./toolUi";

defineProps<{
  tool: Tool | null;
}>();
</script>

<style scoped lang="sass">
.tool-detail-banner
  color: var(--color-text-primary)
  border: 1px solid var(--glass-border)
  background: var(--glass-surface)
  backdrop-filter: blur(var(--glass-blur-default))
  -webkit-backdrop-filter: blur(var(--glass-blur-default))

body.body--dark .tool-detail-banner
  background: rgba(18, 24, 34, 0.55)

.tool-accent-btn
  color: var(--color-accent)
  align-self: flex-start

body:not(.body--dark) .tool-accent-btn:hover
  background: var(--interaction-surface-hover)
</style>
