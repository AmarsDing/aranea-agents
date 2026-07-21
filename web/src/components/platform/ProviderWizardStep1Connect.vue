<template>
  <div class="provider-wizard-panel">
    <h3 class="provider-step-heading">连接与身份</h3>
    <p class="provider-step-hint">从 models.dev 目录选择或手动添加本地/自定义 Provider。</p>
    <div v-if="!editingId" class="app-grid-span-full q-mb-md">
      <q-btn-toggle
        :model-value="providerAddMode"
        spread
        no-caps
        toggle-color="primary"
        :options="[
          { label: '目录选择', value: 'catalog' },
          { label: '自定义', value: 'custom' },
        ]"
        @update:model-value="$emit('update:providerAddMode', $event === 'custom' ? 'custom' : 'catalog')"
      />
    </div>
    <div class="app-form-field-grid app-form-field-grid--2col">
      <q-input
        v-if="providerAddMode === 'catalog'"
        :model-value="catalogProviderSearch"
        dense
        outlined
        clearable
        debounce="300"
        label="搜索供应商"
        class="app-grid-span-full"
        @update:model-value="$emit('update:catalogProviderSearch', String($event ?? ''))"
      />
      <q-select
        v-if="providerAddMode === 'catalog'"
        :model-value="catalogProviderId"
        dense
        outlined
        emit-value
        map-options
        label="供应商（models.dev）"
        :loading="catalogLoading"
        :options="catalogProviderOptions"
        @update:model-value="$emit('update:catalogProviderId', String($event ?? ''))"
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
      <div
        v-if="providerAddMode === 'catalog' && (catalogProviderHint || catalogProviderDocUrl)"
        class="app-grid-span-full row items-center q-gutter-sm q-mb-sm"
      >
        <span v-if="catalogProviderHint" class="text-caption text-grey-7">{{ catalogProviderHint }}</span>
        <a
          v-if="catalogProviderDocUrl"
          :href="catalogProviderDocUrl"
          target="_blank"
          rel="noopener noreferrer"
          class="text-caption text-primary"
        >
          查看 Provider 文档 ↗
        </a>
      </div>
      <q-select
        v-if="providerAddMode === 'custom'"
        :model-value="providerPresetKey"
        dense
        outlined
        emit-value
        map-options
        label="供应商预设（可选）"
        :options="providerPresetOptions"
        clearable
        @update:model-value="$emit('update:providerPresetKey', String($event ?? ''))"
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
      <q-input
        v-if="providerRuntimeLocked"
        dense
        outlined
        readonly
        label="运行时类型"
        :model-value="providerRuntimeSummary"
        hint="由 models.dev 目录 / runtime overlay 自动决定，无需手动选择"
      />
      <q-select
        v-else
        v-model="providerForm.provider_type"
        dense
        outlined
        emit-value
        map-options
        label="Provider类型 *"
        :options="providerTypeOptions"
      />
      <q-input
        v-model="providerForm.api_key"
        dense
        outlined
        :type="showApiKey ? 'text' : 'password'"
        label="API 密钥"
        :hint="apiKeyFieldHint"
        :placeholder="apiKeyMaskedPlaceholder"
        :loading="revealingCredentials"
      >
        <template #append>
          <q-btn
            flat
            dense
            round
            :icon="showApiKey ? 'visibility_off' : 'visibility'"
            :aria-label="showApiKey ? '隐藏密钥' : '显示密钥'"
            :disable="revealingCredentials"
            @click="$emit('toggle-api-key-visibility')"
          />
        </template>
      </q-input>
      <div v-if="catalogModelsHint" class="app-grid-span-full text-caption text-grey-7 q-mb-xs">
        {{ catalogModelsHint }}
      </div>
      <q-select
        v-model="providerForm.model_api_id"
        dense
        outlined
        :use-input="providerAddMode === 'custom' || useCatalogModelPicker"
        :new-value-mode="providerAddMode === 'custom' ? 'add-unique' : undefined"
        :fill-input="false"
        :hide-selected="false"
        input-debounce="0"
        emit-value
        map-options
        label="模型"
        :hint="providerAddMode === 'custom' ? '输入模型 ID 后按回车可添加目录外模型' : undefined"
        :loading="catalogModelsLoading"
        :options="providerModelOptions"
        @filter="onModelFilter"
        @new-value="onNewModelValue"
        @update:model-value="$emit('update:model-api-id', $event)"
      >
        <template #append>
          <q-btn
            flat
            dense
            no-caps
            class="provider-inspect-btn"
            label="检查"
            :loading="checkingModel"
            :disable="!canInspectProviderModel"
            @click.stop="$emit('inspect-current-provider-model')"
          />
        </template>
        <template #option="scope">
          <q-item v-bind="scope.itemProps">
            <q-item-section>
              <q-item-label>{{ scope.opt.label }}</q-item-label>
              <q-item-label caption>{{ scope.opt.caption }}</q-item-label>
            </q-item-section>
          </q-item>
        </template>
      </q-select>
      <q-input
        v-model="providerForm.provider_code"
        dense
        outlined
        label="Provider ID *"
        hint="厂商 ID（如 deepseek），勿填模型名"
        :readonly="providerAddMode === 'catalog'"
        :rules="[providerCodeRule]"
      />
      <q-banner
        v-if="providerRuntimeBindingPreview"
        dense
        rounded
        class="app-grid-span-full bg-info text-white text-caption"
      >
        {{ providerRuntimeBindingPreview }}
      </q-banner>
      <q-banner
        v-if="editingId && providerIdentityChanged"
        dense
        rounded
        class="app-grid-span-full app-banner-warning text-caption"
      >
        已修改 Provider ID 或模型 ID，保存前请点击模型旁的「检查」验证连通性。
      </q-banner>
      <q-input
        v-model="providerForm.provider_display_name"
        dense
        outlined
        label="供应商名称"
        :readonly="providerAddMode === 'catalog'"
        :hint="providerAddMode === 'catalog' ? '来自 catalog 的 name 字段' : undefined"
      />
      <q-input v-model="providerForm.model_display_name" dense outlined label="模型展示名" />
      <q-input v-model="providerForm.api_base_url" dense outlined label="API 基础 URL" placeholder="https://..." />
      <q-toggle v-model="providerForm.enabled" label="已启用" />
      <q-select
        v-if="!providerRuntimeLocked && providerForm.provider_type === 'openai'"
        v-model="providerForm.variant"
        dense
        outlined
        emit-value
        map-options
        label="Variant"
        :options="variantOptions"
      />
      <template v-if="currentAuthType === 'secret_id_key'">
        <q-input v-model="providerForm.secret_id" dense outlined label="Secret ID" />
        <q-input
          v-model="providerForm.secret_key"
          dense
          outlined
          :type="showSecretKey ? 'text' : 'password'"
          label="Secret Key"
          :hint="editingId ? '留空表示不修改' : undefined"
          :placeholder="secretKeyMaskedPlaceholder"
          :loading="revealingCredentials"
        >
          <template #append>
            <q-btn
              flat
              dense
              round
              :icon="showSecretKey ? 'visibility_off' : 'visibility'"
              :aria-label="showSecretKey ? '隐藏 Secret Key' : '显示 Secret Key'"
              :disable="revealingCredentials"
              @click="$emit('toggle-secret-key-visibility')"
            />
          </template>
        </q-input>
      </template>
      <q-input
        v-if="currentAuthType === 'aws_config'"
        v-model="providerForm.aws_region"
        dense
        outlined
        label="AWS Region"
        placeholder="us-east-1"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
const providerForm = defineModel<ProviderForm>('providerForm', { required: true });
import type { ProviderForm } from '../../features/platform/types';

type SelectOption = { label: string; value: string; caption?: string };

const props = defineProps<{
  editingId: string | null;
  providerAddMode: 'catalog' | 'custom';
  catalogProviderSearch: string;
  catalogProviderId: string;
  catalogProviderHint: string;
  catalogProviderDocUrl: string;
  catalogProviderOptions: SelectOption[];
  catalogLoading: boolean;
  catalogModelsHint: string;
  catalogModelsLoading: boolean;
  providerPresetKey: string;
  providerPresetOptions: SelectOption[];
  providerRuntimeLocked: boolean;
  providerRuntimeSummary: string;
  providerTypeOptions: SelectOption[];
  showApiKey: boolean;
  apiKeyFieldHint: string;
  apiKeyMaskedPlaceholder: string;
  revealingCredentials: boolean;
  useCatalogModelPicker: boolean;
  providerModelOptions: SelectOption[];
  providerCodeRule: (val: string) => boolean | string;
  providerRuntimeBindingPreview: string;
  providerIdentityChanged: boolean;
  variantOptions: SelectOption[];
  currentAuthType: string;
  showSecretKey: boolean;
  secretKeyMaskedPlaceholder: string;
  canInspectProviderModel: boolean;
  checkingModel: boolean;
  filterCatalogModelsLocal: (val: string, update: (fn: () => void) => void) => void;
}>();

const emit = defineEmits<{
  'update:providerAddMode': [value: string];
  'update:catalogProviderSearch': [value: string];
  'update:catalogProviderId': [value: string];
  'update:providerPresetKey': [value: string];
  'toggle-api-key-visibility': [];
  'set-custom-model-value': [value: string, done?: (value: string, mode?: 'add' | 'add-unique' | 'toggle') => void];
  'update:model-api-id': [value: string | number | null];
  'inspect-current-provider-model': [];
  'toggle-secret-key-visibility': [];
}>();

function onNewModelValue(value: string, done?: (value: string, mode?: 'add' | 'add-unique' | 'toggle') => void) {
  emit('set-custom-model-value', value, done);
}

function onModelFilter(val: string, update: (fn: () => void) => void) {
  if (props.useCatalogModelPicker) {
    props.filterCatalogModelsLocal(val, update);
  } else {
    update(() => {});
  }
}
</script>
