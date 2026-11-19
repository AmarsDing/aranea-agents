<template>
  <app-dialog
    :model-value="dialogModel"
    :title="$t('domains.agentCreate.title')"
    :subtitle="$t('domains.agentCreate.subtitle')"
    icon="smart_toy"
    size="xl"
    persistent
    @update:model-value="dialogModel = $event"
  >
    <template #header-prefix>
      <div class="avatar-picker-hit cursor-pointer" @click="avatarPickerOpen = true">
        <agent-avatar-q
          size="42px"
          avatar-class="avatar-picker--toolbar"
          :icon="form.icon"
          :alt="form.display_name || 'Agent avatar'"
        />
        <q-tooltip>{{ $t('domains.agentCreate.changeAvatar') }}</q-tooltip>
      </div>
    </template>

    <div class="create-agent-card">
        <q-banner v-if="createFormError" dense rounded class="bg-negative text-white q-mb-md">
          {{ createFormError }}
        </q-banner>
        <div class="agent-kind-row q-mb-md">
          <div class="text-subtitle2 q-mb-sm">{{ $t('domains.agentCreate.agentKind') }}</div>
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
            :label="$t('domains.agentCreate.displayName')"
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
            :label="$t('domains.agentCreate.agentKey')"
            :hint="$t('domains.agentCreate.agentKeyHint')"
            :error="Boolean(agentKeyError)"
            :error-message="agentKeyError"
          />
          <taxonomy-picker
            :model-value="form.taxonomy_position_id || null"
            class="app-field-long"
            :tree="taxonomyTree"
            :label="$t('agentsPage.taxonomy.labelOptional')"
            :placeholder="$t('domains.agentCreate.taxonomyPlaceholder')"
            @update:model-value="onCategoryPick"
          />
          <template v-if="isA2AProxy">
            <q-input
              v-model.trim="a2aProxy.remote_url"
              class="agent-dialog-control app-field-long create-agent-grid__span-full"
              dense
              outlined
              :label="$t('domains.agentCreate.remoteUrl')"
              :hint="$t('domains.agentCreate.remoteUrlHint')"
              :error="Boolean(remoteUrlError)"
              :error-message="remoteUrlError"
            />
            <q-toggle
              v-model="a2aProxy.enable_streaming"
              color="primary"
              :label="$t('domains.agentCreate.streaming')"
            />
            <q-input
              v-model.number="a2aProxy.timeout_seconds"
              class="agent-dialog-control"
              dense
              outlined
              type="number"
              min="5"
              :label="$t('domains.agentCreate.timeoutSeconds')"
            />
            <q-toggle v-model="selfEvolveModel" color="primary" :label="$t('domains.agentCreate.selfEvolve')" dense />
          </template>
          <template v-else>
            <q-select
              v-model="form.provider"
              class="agent-dialog-control"
              dense
              outlined
              emit-value
              map-options
              :label="$t('domains.agentCreate.provider')"
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
              :label="$t('domains.agentCreate.model')"
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
                :label="$t('domains.agentCreate.checkModel')"
                :disable="!form.provider || !form.model"
                :loading="checkingModel"
                @click.stop="$emit('check-model')"
              />
              <q-toggle v-model="selfEvolveModel" color="primary" :label="$t('domains.agentCreate.selfEvolve')" dense />
            </div>
          </template>
        </div>

        <section v-if="!isA2AProxy" class="description-block">
          <div class="row items-center q-gutter-xs">
            <div class="text-subtitle2">{{ $t('domains.agentCreate.descriptionTitle') }}</div>
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
            :label="$t('domains.agentCreate.description')"
            :hint="$t('domains.agentCreate.descriptionHint')"
          />
        </section>
    </div>

    <template #actions>
      <div v-if="createDisabledHint" class="text-caption text-negative">{{ createDisabledHint }}</div>
      <div class="row items-center q-gutter-sm">
        <q-btn flat rounded :label="$t('domains.agentCreate.cancel')" @click="dialogModel = false" />
        <!-- wrapper 捕获禁用态点击：Quasar disable 按钮不派发事件，点击/悬停均给出原因反馈 -->
        <div class="create-agent-card__submit" @click="onCreateClick">
          <q-btn
            color="primary"
            rounded
            unelevated
            :label="$t('domains.agentCreate.create')"
            :disable="!canCreate"
            :loading="creating"
            @click="$emit('create')"
          >
            <q-tooltip v-if="!canCreate && createDisabledHint">{{ createDisabledHint }}</q-tooltip>
          </q-btn>
        </div>
      </div>
    </template>
  </app-dialog>
  <agent-avatar-picker v-model="form.icon" v-model:open="avatarPickerOpen" />
</template>

<script setup lang="ts">
const form = defineModel<CreateForm>('form', { required: true });
const a2aProxy = defineModel<A2AProxyConfig>('a2aProxy', { required: true });
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import AppDialog from '../common/AppDialog.vue';
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
  isA2AProxy: boolean;
  selfEvolve: boolean;
  agentKind: AgentKind;
  taxonomyTree: PlatformResourceTreeNode[];
  providerOptions: Array<{ label: string; value: string }>;
  modelOptions: Array<{ label: string; value: string }>;
  selectedTemplateKey: string;
  agentKeyError: string;
  displayNameError?: string;
  providerModelError?: string;
  remoteUrlError?: string;
  createFormError?: string;
  canCreate: boolean;
  createDisabledHint?: string;
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
  'create-attempt': [];
  create: [];
}>();

const { t } = useI18n();
const dialogModel = computed({
  get: () => props.modelValue,
  set: (value: boolean) => emit('update:modelValue', value),
});

const selfEvolveModel = computed({
  get: () => props.selfEvolve,
  set: (value: boolean) => emit('update:selfEvolve', value),
});

function onCategoryPick(value: string | null) {
  form.value.taxonomy_position_id = value ?? '';
}

const avatarPickerOpen = ref(false);

const agentKindOptions = computed(() => [
  { label: t('domains.agentCreate.agentKindLlm'), value: 'llm' },
  { label: t('domains.agentCreate.agentKindA2a'), value: 'a2a_proxy' },
]);

const agentKindModel = computed({
  get: () => props.agentKind || 'llm',
  set: (value: AgentKind) => emit('update:agentKind', value),
});

/** 禁用态点击反馈：q-btn disable 时不派发 click，由 wrapper 捕获并上抛，字段级标红与 toast 由页面层统一处理。 */
function onCreateClick() {
  if (!props.canCreate) {
    emit('create-attempt');
  }
}
</script>
