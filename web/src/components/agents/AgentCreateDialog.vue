<template>
  <q-dialog v-model="dialogModel" persistent>
    <q-card :class="['create-agent-card', { 'create-agent-card--dark': isDark }]">
      <q-toolbar class="create-agent-card__toolbar">
        <div>
          <q-toolbar-title class="q-pa-none">创建 Agent</q-toolbar-title>
          <div class="text-caption text-grey-7">最小字段创建，模型检查通过后才能提交。</div>
        </div>
        <q-space />
        <q-btn flat round dense icon="close" v-close-popup />
      </q-toolbar>
      <q-separator />

      <q-card-section class="create-agent-card__body">
        <q-banner v-if="createFormError" dense rounded class="bg-negative text-white q-mb-md">
          {{ createFormError }}
        </q-banner>
        <div class="agent-kind-row q-mb-md">
          <div class="text-subtitle2 q-mb-sm">Agent 类型</div>
          <q-btn-toggle
            v-model="agentKindModel"
            spread
            no-caps
            rounded
            unelevated
            toggle-color="primary"
            color="grey-3"
            text-color="grey-9"
            :options="agentKindOptions"
          />
        </div>

        <div class="create-agent-layout row q-col-gutter-lg">
          <div class="col-12 col-md-auto column items-center avatar-column">
            <div class="avatar-picker-hit cursor-pointer" @click="avatarPickerOpen = true">
              <agent-avatar-q size="104px" avatar-class="avatar-picker" :icon="form.icon" :alt="form.display_name || 'Agent avatar'" />
              <q-tooltip>选择头像</q-tooltip>
            </div>
            <q-btn class="avatar-change-btn q-mt-md" outline rounded color="primary" icon="photo_library" label="选择头像" @click="avatarPickerOpen = true" />
            <div class="avatar-column__hint">从数据库内置头像选择，或上传图片。</div>
          </div>

          <div class="col">
            <div class="form-panel row q-col-gutter-md">
              <q-input
                v-model.trim="form.display_name"
                class="col-12 col-md-6 agent-dialog-control"
                dense
                outlined
                label="显示名称 *"
                :error="Boolean(displayNameError)"
                :error-message="displayNameError"
              >
                <template #prepend><q-icon name="smart_toy" /></template>
              </q-input>
              <q-input
                v-model.trim="form.agent_key"
                class="col-12 col-md-6 agent-dialog-control"
                dense
                outlined
                label="Agent 标识 *"
                hint="小写字母、数字、连字符"
                :error="Boolean(agentKeyError)"
                :error-message="agentKeyError"
              />
              <q-select v-model="categoryIndustry" class="col-12 col-md-4 agent-dialog-control" dense outlined clearable emit-value map-options label="行业" :options="industryOptions" />
              <q-select v-model="categoryDepartment" class="col-12 col-md-4 agent-dialog-control" dense outlined clearable emit-value map-options label="部门" :options="departmentOptions" :disable="!categoryIndustry" />
              <q-select v-model="form.category_position_id" class="col-12 col-md-4 agent-dialog-control" dense outlined clearable emit-value map-options label="职位" :options="positionOptions" :disable="!categoryDepartment" />
              <template v-if="isA2AProxy">
                <q-input
                  v-model.trim="a2aProxy.remote_url"
                  class="col-12 agent-dialog-control"
                  dense
                  outlined
                  label="远程 A2A URL *"
                  hint="例如 http://host:8087/"
                  :error="Boolean(remoteUrlError)"
                  :error-message="remoteUrlError"
                />
                <q-toggle v-model="a2aProxy.enable_streaming" class="col-12 col-md-4" color="primary" label="流式响应" />
                <q-input v-model.number="a2aProxy.timeout_seconds" class="col-12 col-md-4 agent-dialog-control" dense outlined type="number" min="5" label="超时（秒）" />
              </template>
              <template v-else>
              <q-select
                v-model="form.provider"
                class="col-12 col-md-5 agent-dialog-control"
                dense
                outlined
                emit-value
                map-options
                label="Provider *"
                :options="providerOptions"
                :error="Boolean(providerModelError)"
                :error-message="providerModelError"
              />
              <q-select
                v-model="form.model"
                class="col-12 col-md-5 agent-dialog-control"
                dense
                outlined
                emit-value
                map-options
                label="模型 *"
                :options="modelOptions"
                :error="Boolean(providerModelError)"
              />
              <div class="col-12 col-md-2">
                <q-btn class="model-check-btn full-width" outline rounded color="primary" label="检查" :disable="!form.provider || !form.model" :loading="checkingModel" @click="$emit('check-model')" />
              </div>
              </template>
            </div>
          </div>
        </div>

        <section v-if="!isA2AProxy" class="description-block">
          <div class="text-subtitle2">描述您的 Agent</div>
          <div class="row q-gutter-xs q-mt-sm">
            <q-chip
              v-for="template in templates"
              :key="template.key"
              clickable
              outline
              color="primary"
              :icon="template.icon"
              :class="{ 'template-chip--active': selectedTemplateKey === template.key }"
              @click="$emit('apply-template', template)"
            >
              {{ template.label }}
            </q-chip>
          </div>
          <q-input
            v-model="form.agent_description"
            class="agent-dialog-control q-mt-sm"
            outlined
            type="textarea"
            rows="7"
            label="描述您的 Agent"
            hint="AI 将根据此描述自动生成 Agent 的上下文文件。留空则使用模板。"
          />
        </section>

        <q-card flat bordered class="self-evolve-card" v-if="!isA2AProxy">
          <q-card-section class="row items-center justify-between">
            <div>
              <div class="text-subtitle2">自我进化</div>
              <div class="text-caption text-grey-7">允许 Agent 通过 SOUL.md 随时间进化其风格和语调。</div>
            </div>
            <q-toggle v-model="selfEvolveModel" color="primary" />
          </q-card-section>
        </q-card>
      </q-card-section>

      <q-card-actions align="right" class="create-agent-card__actions">
        <q-btn flat rounded label="取消" v-close-popup />
        <q-btn color="primary" rounded unelevated label="创建" :disable="!canCreate" :loading="creating" @click="$emit('create')" />
      </q-card-actions>
    </q-card>
    <agent-avatar-picker v-model="form.icon" v-model:open="avatarPickerOpen" />
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { useQuasar } from "quasar";
import AgentAvatarPicker from "../avatar/AgentAvatarPicker.vue";
import AgentAvatarQ from "../avatar/AgentAvatarQ.vue";
import { descriptionTemplates } from "./agentUi";
import type { AgentKind, A2AProxyConfig } from "../../features/agents/types";

type CreateForm = {
  agent_key: string;
  display_name: string;
  provider: string;
  model: string;
  icon: string;
  agent_description: string;
  category_position_id: string;
};

const props = defineProps<{
  modelValue: boolean;
  form: CreateForm;
  a2aProxy: A2AProxyConfig;
  isA2AProxy: boolean;
  selfEvolve: boolean;
  agentKind: AgentKind;
  categoryIndustry: string | null;
  categoryDepartment: string | null;
  industryOptions: Array<{ label: string; value: string }>;
  departmentOptions: Array<{ label: string; value: string }>;
  positionOptions: Array<{ label: string; value: string }>;
  providerOptions: Array<{ label: string; value: string }>;
  modelOptions: Array<{ label: string; value: string }>;
  selectedTemplateKey: string;
  agentKeyError: string;
  displayNameError?: string;
  providerModelError?: string;
  remoteUrlError?: string;
  createFormError?: string;
  canCreate: boolean;
  creating: boolean;
  checkingModel: boolean;
  templates?: typeof descriptionTemplates;
}>();

const templates = computed(() => props.templates ?? descriptionTemplates);

const emit = defineEmits<{
  "update:modelValue": [value: boolean];
  "update:selfEvolve": [value: boolean];
  "update:agentKind": [value: AgentKind];
  "update:a2aProxy": [value: A2AProxyConfig];
  "update:categoryIndustry": [value: string | null];
  "update:categoryDepartment": [value: string | null];
  "apply-template": [template: (typeof descriptionTemplates)[number]];
  "check-model": [];
  create: [];
}>();

const $q = useQuasar();
const isDark = computed(() => $q.dark.isActive);
const dialogModel = computed({
  get: () => props.modelValue,
  set: (value: boolean) => emit("update:modelValue", value)
});

const selfEvolveModel = computed({
  get: () => props.selfEvolve,
  set: (value: boolean) => emit("update:selfEvolve", value)
});

const categoryIndustry = computed({
  get: () => props.categoryIndustry,
  set: (value: string | null) => emit("update:categoryIndustry", value)
});

const categoryDepartment = computed({
  get: () => props.categoryDepartment,
  set: (value: string | null) => emit("update:categoryDepartment", value)
});

const avatarPickerOpen = ref(false);

const agentKindOptions = [
  { label: "LLM Agent", value: "llm" },
  { label: "A2A 远程代理", value: "a2a_proxy" }
];

const agentKindModel = computed({
  get: () => props.agentKind || "llm",
  set: (value: AgentKind) => emit("update:agentKind", value)
});
</script>

<style scoped>
.create-agent-card {
  width: 880px;
  max-width: 94vw;
  border: 1px solid rgb(15 23 42 / 8%);
  border-radius: 28px;
  background:
    radial-gradient(circle at 8% 0%, rgb(25 118 210 / 10%), transparent 32%),
    radial-gradient(circle at 88% 16%, rgb(245 158 11 / 8%), transparent 28%),
    var(--color-on-accent);
  box-shadow: 0 30px 90px rgb(16 24 40 / 20%);
}

.create-agent-card__toolbar {
  padding: 22px 26px;
  background: linear-gradient(180deg, var(--color-on-accent), var(--color-page-tint));
}

.create-agent-card__body {
  padding: 22px 24px 18px;
}

.create-agent-layout {
  padding-top: 4px;
}

.avatar-column {
  width: 190px;
  padding: 18px 16px;
  border: 1px solid rgb(15 23 42 / 8%);
  border-radius: 24px;
  background:
    linear-gradient(180deg, rgb(255 255 255 / 72%), rgb(248 250 252 / 92%)),
    radial-gradient(circle at top, rgb(25 118 210 / 10%), transparent 54%);
  box-shadow: inset 0 1px 0 rgb(255 255 255 / 90%);
}

.avatar-picker-hit {
  display: inline-block;
  line-height: 0;
}

.avatar-picker {
  border: 4px solid var(--color-on-accent);
  box-shadow: 0 18px 40px rgb(25 118 210 / 24%);
  transition:
    transform 180ms ease,
    box-shadow 180ms ease;
}

.avatar-picker:hover {
  transform: translateY(-2px) scale(1.01);
  box-shadow: 0 22px 50px rgb(25 118 210 / 30%);
}

.avatar-change-btn {
  width: 100%;
  min-height: 38px;
  font-weight: 700;
}

.avatar-column__hint {
  margin-top: 10px;
  color: var(--color-text-tertiary);
  font-size: 12px;
  line-height: 1.55;
  text-align: center;
}

.form-panel {
  padding: 2px 0 0;
}

.agent-dialog-control :deep(.q-field__control) {
  min-height: 44px;
  border-radius: 16px;
  background: var(--color-on-accent);
}

.agent-dialog-control :deep(.q-field__control::before) {
  border-color: rgb(15 23 42 / 14%);
}

.agent-dialog-control :deep(.q-field__control::after) {
  border-width: 1px;
}

.agent-dialog-control :deep(textarea) {
  min-height: 132px;
  color: var(--color-text-heading);
  line-height: 1.65;
}

.model-check-btn {
  min-height: 44px;
  font-weight: 700;
}

.description-block {
  margin-top: 20px;
  padding: 18px 18px 14px;
  border: 1px solid rgb(15 23 42 / 8%);
  border-radius: 24px;
  background:
    linear-gradient(180deg, var(--color-on-accent), var(--color-page-tint)),
    radial-gradient(circle at top left, rgb(25 118 210 / 5%), transparent 35%);
  box-shadow: 0 12px 30px rgb(16 24 40 / 3.5%);
}

.description-block :deep(.q-chip) {
  font-weight: 700;
  background: var(--color-on-accent);
  transition:
    background 160ms ease,
    border-color 160ms ease,
    transform 160ms ease;
}

.description-block :deep(.q-chip:hover) {
  transform: translateY(-1px);
  background: var(--color-info-soft);
}

.template-chip--active {
  border-color: rgb(245 158 11 / 36%);
  background: var(--color-status-warning-bg-alt);
  color: var(--color-status-warning-text);
}

.self-evolve-card {
  margin-top: 18px;
  border-color: rgb(245 158 11 / 18%);
  border-radius: 22px;
  background: linear-gradient(135deg, var(--color-status-warning-bg-warm), var(--color-page-tint-blue));
  box-shadow: 0 12px 30px rgb(16 24 40 / 3.5%);
}

.create-agent-card__actions {
  padding: 14px 22px 20px;
  background: rgb(248 250 252 / 58%);
}

.create-agent-card__actions :deep(.q-btn) {
  min-height: 40px;
  padding: 0 18px;
  font-weight: 700;
}

.create-agent-card.create-agent-card--dark {
  border-color: rgb(148 163 184 / 16%);
  background:
    radial-gradient(circle at 8% 0%, rgb(59 130 246 / 14%), transparent 32%),
    radial-gradient(circle at 88% 16%, rgb(245 158 11 / 10%), transparent 28%),
    var(--color-surface-elevated);
  color: var(--color-border-soft);
  box-shadow: 0 30px 90px rgb(0 0 0 / 55%);
}

.create-agent-card.create-agent-card--dark .create-agent-card__toolbar {
  background: linear-gradient(180deg, rgb(17 24 39 / 98%), rgb(15 23 42 / 94%));
}

.create-agent-card.create-agent-card--dark .avatar-column,
.create-agent-card.create-agent-card--dark .description-block,
.create-agent-card.create-agent-card--dark .self-evolve-card {
  border-color: rgb(148 163 184 / 16%);
  background:
    linear-gradient(180deg, rgb(30 41 59 / 68%), rgb(15 23 42 / 82%)),
    radial-gradient(circle at top, rgb(59 130 246 / 12%), transparent 54%);
  box-shadow: 0 12px 30px rgb(0 0 0 / 25%);
}

.create-agent-card.create-agent-card--dark .avatar-picker {
  border-color: rgb(15 23 42 / 90%);
}

.create-agent-card.create-agent-card--dark .avatar-column__hint {
  color: var(--color-text-tertiary);
}

.create-agent-card.create-agent-card--dark .agent-dialog-control :deep(.q-field__control) {
  background: rgb(30 41 59 / 76%);
}

.create-agent-card.create-agent-card--dark .agent-dialog-control :deep(.q-field__control::before) {
  border-color: rgb(148 163 184 / 18%);
}

.create-agent-card.create-agent-card--dark .agent-dialog-control :deep(textarea) {
  color: var(--color-border-soft);
}

.create-agent-card.create-agent-card--dark .description-block :deep(.q-chip) {
  background: rgb(30 41 59 / 72%);
}

.create-agent-card.create-agent-card--dark .description-block :deep(.q-chip:hover) {
  background: rgb(51 65 85 / 78%);
}

.create-agent-card.create-agent-card--dark .template-chip--active {
  border-color: rgb(245 158 11 / 36%);
  background: rgb(120 53 15 / 32%);
  color: var(--color-accent-amber);
}

.create-agent-card.create-agent-card--dark .create-agent-card__actions {
  background: rgb(15 23 42 / 72%);
}

@media (width <= 767px) {
  .avatar-column {
    width: 100%;
  }
}
</style>
