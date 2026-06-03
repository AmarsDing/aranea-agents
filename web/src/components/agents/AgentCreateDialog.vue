<template>
  <q-dialog v-model="dialogModel" persistent>
    <q-card
      :class="[
        'create-agent-card app-dialog-card app-dialog-card--2xl create-agent-card--tall',
        { 'create-agent-card--dark': isDark },
      ]"
    >
      <q-toolbar class="create-agent-card__toolbar">
        <div class="avatar-picker-hit cursor-pointer" @click="avatarPickerOpen = true">
          <agent-avatar-q
            size="42px"
            avatar-class="avatar-picker--toolbar"
            :icon="form.icon"
            :alt="form.display_name || 'Agent avatar'"
          />
          <q-tooltip>点击更换头像</q-tooltip>
        </div>
        <div class="q-ml-sm">
          <q-toolbar-title class="q-pa-none">创建 Agent</q-toolbar-title>
          <div class="text-caption create-agent-card__subtitle">最小字段创建，模型检查通过后才能提交。</div>
        </div>
        <q-space />
        <q-btn v-close-popup flat round dense icon="close" />
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
            no-caps
            rounded
            unelevated
            class="agent-kind-toggle"
            :options="agentKindOptions"
          />
        </div>

        <div class="form-panel create-agent-grid">
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
          <taxonomy-picker
            :model-value="form.taxonomy_position_id || null"
            class="app-field-long"
            :tree="categoryTree"
            label="业务分类"
            placeholder="选择行业 / 部门 / 职位"
            @update:model-value="onCategoryPick"
          />
          <template v-if="isA2AProxy">
            <q-input
              v-model.trim="a2aProxy.remote_url"
              class="agent-dialog-control app-field-long create-agent-grid__span-full"
              dense
              outlined
              label="远程 A2A URL *"
              hint="例如 http://host:8087/"
              :error="Boolean(remoteUrlError)"
              :error-message="remoteUrlError"
            />
            <q-toggle v-model="a2aProxy.enable_streaming" color="primary" label="流式响应" />
            <q-input
              v-model.number="a2aProxy.timeout_seconds"
              class="agent-dialog-control"
              dense
              outlined
              type="number"
              min="5"
              label="超时（秒）"
            />
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
            <div class="create-agent-inline-actions">
              <q-btn
                class="model-check-btn"
                outline
                rounded
                no-caps
                color="primary"
                label="检查"
                :disable="!form.provider || !form.model"
                :loading="checkingModel"
                @click="$emit('check-model')"
              />
              <q-toggle v-model="selfEvolveModel" color="primary" label="自我进化" dense />
            </div>
          </template>
        </div>

        <section v-if="!isA2AProxy" class="description-block">
          <div class="row items-center q-gutter-xs">
            <div class="text-subtitle2">描述您的 Agent</div>
            <q-chip
              v-for="template in templates"
              :key="template.key"
              clickable
              outline
              color="primary"
              :icon="template.icon"
              size="md"
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
            rows="6"
            label="Agent 描述"
            hint="AI 将根据此描述自动生成上下文文件。留空则使用模板。"
          />
        </section>
      </q-card-section>

      <q-card-actions align="right" class="create-agent-card__actions app-actions-bar">
        <q-btn v-close-popup flat rounded label="取消" />
        <q-btn
          color="primary"
          rounded
          unelevated
          label="创建"
          :disable="!canCreate"
          :loading="creating"
          @click="$emit('create')"
        />
      </q-card-actions>
    </q-card>
    <agent-avatar-picker v-model="form.icon" v-model:open="avatarPickerOpen" />
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useQuasar } from 'quasar';
import AgentAvatarPicker from '../avatar/AgentAvatarPicker.vue';
import AgentAvatarQ from '../avatar/AgentAvatarQ.vue';
import TaxonomyPicker from './TaxonomyPicker.vue';
import type { AgentKind, AgentTemplatePreset, A2AProxyConfig } from '../../features/agents/types';
import type { PlatformResourceTreeNode } from '../../features/platform/types';

type CreateForm = {
  agent_key: string;
  display_name: string;
  provider: string;
  model: string;
  icon: string;
  agent_description: string;
  position_key: string;
  agent_variant: string;
  variant_description: string;
  taxonomy_position_id: string;
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

const templates = computed<AgentTemplatePreset[]>(() => props.templates ?? []);

const emit = defineEmits<{
  'update:modelValue': [value: boolean];
  'update:selfEvolve': [value: boolean];
  'update:agentKind': [value: AgentKind];
  'update:a2aProxy': [value: A2AProxyConfig];
  'apply-template': [template: AgentTemplatePreset];
  'check-model': [];
  create: [];
}>();

const $q = useQuasar();
const isDark = computed(() => $q.dark.isActive);
const dialogModel = computed({
  get: () => props.modelValue,
  set: (value: boolean) => emit('update:modelValue', value),
});

const selfEvolveModel = computed({
  get: () => props.selfEvolve,
  set: (value: boolean) => emit('update:selfEvolve', value),
});

function onCategoryPick(value: string | null) {
  props.form.taxonomy_position_id = value ?? '';
}

const avatarPickerOpen = ref(false);

const agentKindOptions = [
  { label: 'LLM 智能体', value: 'llm' },
  { label: 'A2A 远程代理', value: 'a2a_proxy' },
];

const agentKindModel = computed({
  get: () => props.agentKind || 'llm',
  set: (value: AgentKind) => emit('update:agentKind', value),
});
</script>
