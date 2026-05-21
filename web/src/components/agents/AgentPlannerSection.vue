<template>
  <section class="settings-section">
    <div class="section-heading">
      <div>
        <div class="text-subtitle1 text-weight-bold">规划模式</div>
        <div class="text-caption text-grey-7">
          Agent 级持久化，与 Chat「对话模式」独立。空 kind 三态：保存仅允许 config
          <code>{}</code>；运行时 Builtin 仅当会话「深思考」(plan)；Chat 展示仍可按消息标签启发式识别 ReAct/A2UI。
        </div>
      </div>
    </div>

    <q-select
      v-model="form.kind"
      class="app-field-md"
      dense
      outlined
      emit-value
      map-options
      label="规划策略"
      :options="kindOptions"
    >
      <template #option="scope">
        <q-item v-bind="scope.itemProps">
          <q-item-section>
            <q-item-label>{{ scope.opt.label }}</q-item-label>
            <q-item-label caption>{{ scope.opt.caption }}</q-item-label>
          </q-item-section>
        </q-item>
      </template>
    </q-select>

    <q-banner v-if="form.kind === ''" rounded dense class="q-mt-sm settings-info-banner">
      <strong>空 kind 三态</strong>：① API 保存时 <code>planner_config_json</code> 只能为
      <code>{}</code>；② 运行时 Builtin 仅当本会话 <code>dialog_mode=plan</code>（深思考）；③
      Chat 历史消息仍可按正文标签展示 ReAct/A2UI，与持久化 kind 无关。
    </q-banner>

    <div v-if="form.kind === 'builtin'" class="q-mt-md app-form-field-grid">
      <div class="text-subtitle2">Builtin 推理参数</div>
      <q-select
        v-model="form.builtin.reasoning_effort"
        dense
        outlined
        emit-value
        map-options
        clearable
        label="reasoning_effort"
        :options="effortOptions"
        :hint="effortHint"
      />
      <q-select
        v-model="thinkingEnabledChoice"
        dense
        outlined
        emit-value
        map-options
        label="thinking_enabled"
        :options="thinkingEnabledOptions"
        hint="未设置 = 不向 API 下发，由模型默认决定"
      />
      <q-input
        v-model.number="form.builtin.thinking_tokens"
        dense
        outlined
        type="number"
        clearable
        label="thinking_tokens"
        hint="Claude / Gemini via OpenAI API；留空表示不下发"
      />
    </div>

    <div v-else-if="form.kind === 'react'" class="q-mt-md">
      <q-banner rounded dense class="settings-info-banner">
        ReAct 无额外配置。模型输出需包含 /*PLANNING*/、/*REASONING*/、/*ACTION*/ 等标签；Chat 将展示步骤卡片。
      </q-banner>
    </div>

    <div v-else-if="form.kind === 'a2ui'" class="q-mt-md q-gutter-sm">
      <div class="text-subtitle2">A2UI 协议</div>
      <q-input v-model="form.a2ui.instruction" class="app-field-long" outlined autogrow type="textarea" label="自定义指令 (instruction)" />
      <q-expansion-item
        dense
        expand-separator
        icon="code"
        label="Schema JSON（高级）"
        caption="每项须为合法 JSON 对象；留空使用框架默认"
        class="planner-schema-expansion"
      >
        <div class="q-gutter-sm q-pa-sm">
          <q-input
            v-for="field in a2uiSchemaFields"
            :key="field.key"
            v-model="form.a2ui[field.key]"
            outlined
            autogrow
            type="textarea"
            :label="field.label"
          />
        </div>
      </q-expansion-item>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { PlannerFormState } from "../../features/agents/plannerConfig";
import {
  PLANNER_KIND_OPTIONS,
  reasoningEffortOptions,
} from "../../features/agents/plannerConfig";

const form = defineModel<PlannerFormState>("form", { required: true });

const props = defineProps<{
  modelProvider?: string;
}>();

const kindOptions = PLANNER_KIND_OPTIONS;

const effortOptions = computed(() => reasoningEffortOptions(props.modelProvider ?? ""));

const effortHint = computed(() => {
  const p = (props.modelProvider ?? "").toLowerCase();
  if (p.includes("deepseek")) return "DeepSeek v4: high / max";
  return "OpenAI o 系: low / medium / high";
});

const thinkingEnabledOptions = [
  { label: "未设置", value: "unset" },
  { label: "启用", value: "true" },
  { label: "禁用", value: "false" },
];

const thinkingEnabledChoice = computed({
  get() {
    const v = form.value.builtin.thinking_enabled;
    if (v === null || v === undefined) return "unset";
    return v ? "true" : "false";
  },
  set(raw: string) {
    if (raw === "unset") form.value.builtin.thinking_enabled = null;
    else form.value.builtin.thinking_enabled = raw === "true";
  },
});

const a2uiSchemaFields: { key: keyof PlannerFormState["a2ui"]; label: string }[] = [
  {
    key: "server_to_client_with_standard_catalog_schema_json",
    label: "server_to_client_with_standard_catalog_schema_json",
  },
  { key: "client_to_server_schema_json", label: "client_to_server_schema_json" },
  { key: "client_capabilities_schema_json", label: "client_capabilities_schema_json" },
  { key: "server_to_client_only_schema_json", label: "server_to_client_only_schema_json" },
  { key: "standard_catalog_definition_json", label: "standard_catalog_definition_json" },
  { key: "catalog_description_schema_json", label: "catalog_description_schema_json" },
];
</script>

<style scoped>
.section-heading {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 14px;
}

.settings-info-banner {
  background: var(--glass-elevated);
  color: var(--color-text-secondary);
}

.planner-schema-expansion {
  border: 1px solid var(--glass-border);
  border-radius: 12px;
}
</style>
