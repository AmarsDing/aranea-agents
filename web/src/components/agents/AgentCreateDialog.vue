<template>
  <q-dialog v-model="dialogModel" persistent>
    <q-card :class="['create-agent-card app-dialog-card app-dialog-card--lg', { 'create-agent-card--dark': isDark }]">
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
            <div class="form-panel app-form-field-grid">
              <q-input
                v-model.trim="form.display_name"
                class="agent-dialog-control"
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
                class="agent-dialog-control"
                dense
                outlined
                label="Agent 标识 *"
                hint="小写字母、数字、连字符"
                :error="Boolean(agentKeyError)"
                :error-message="agentKeyError"
              />
              <agent-category-picker
                :model-value="form.category_position_id || null"
                class="app-field-long"
                :tree="categoryTree"
                label="业务分类"
                placeholder="选择行业 / 部门 / 职位"
                @update:model-value="onCategoryPick"
              />
              <template v-if="isA2AProxy">
                <q-input
                  v-model.trim="a2aProxy.remote_url"
                  class="agent-dialog-control app-field-long"
                  dense
                  outlined
                  label="远程 A2A URL *"
                  hint="例如 http://host:8087/"
                  :error="Boolean(remoteUrlError)"
                  :error-message="remoteUrlError"
                />
                <q-toggle v-model="a2aProxy.enable_streaming" color="primary" label="流式响应" />
                <q-input v-model.number="a2aProxy.timeout_seconds" class="agent-dialog-control" dense outlined type="number" min="5" label="超时（秒）" />
              </template>
              <template v-else>
              <q-select
                v-model="form.provider"
                class="agent-dialog-control"
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
                class="agent-dialog-control"
                dense
                outlined
                emit-value
                map-options
                label="模型 *"
                :options="modelOptions"
                :error="Boolean(providerModelError)"
              />
              <div class="app-actions-bar app-actions-bar--start">
                <q-btn class="model-check-btn" outline rounded no-caps color="primary" label="检查" :disable="!form.provider || !form.model" :loading="checkingModel" @click="$emit('check-model')" />
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
            class="agent-dialog-control app-field-long q-mt-sm"
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

      <q-card-actions align="right" class="create-agent-card__actions app-actions-bar">
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
import AgentCategoryPicker from "./AgentCategoryPicker.vue";
import { descriptionTemplates } from "./agentUi";
import type { AgentKind, AgentTemplatePreset, A2AProxyConfig } from "../../features/agents/types";
import type { PlatformResourceTreeNode } from "../../features/platform/types";

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
  categoryTree: PlatformResourceTreeNode[];
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
  templates?: AgentTemplatePreset[];
}>();

const templates = computed<AgentTemplatePreset[]>(() => props.templates ?? descriptionTemplates.map((t) => ({ ...t, description: t.text ?? "" })));

const emit = defineEmits<{
  "update:modelValue": [value: boolean];
  "update:selfEvolve": [value: boolean];
  "update:agentKind": [value: AgentKind];
  "update:a2aProxy": [value: A2AProxyConfig];
  "apply-template": [template: AgentTemplatePreset];
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

function onCategoryPick(value: string | null) {
  props.form.category_position_id = value ?? "";
}

const avatarPickerOpen = ref(false);

const agentKindOptions = [
  { label: "LLM 智能体", value: "llm" },
  { label: "A2A 远程代理", value: "a2a_proxy" }
];

const agentKindModel = computed({
  get: () => props.agentKind || "llm",
  set: (value: AgentKind) => emit("update:agentKind", value)
});
</script>
