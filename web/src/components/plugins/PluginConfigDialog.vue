<template>
  <q-dialog :model-value="open" persistent @update:model-value="$emit('update:open', $event)">
    <q-card class="app-dialog-card app-dialog-card--md app-glass-dialog">
      <q-card-section class="app-glass-dialog__head row items-center justify-between">
        <div class="min-width-0">
          <div class="app-glass-dialog__title">配置 {{ target?.name }}</div>
          <div class="app-glass-dialog__subtitle">Schema 驱动表单或 JSON 编辑；保存后 Runner 热重载生效。</div>
        </div>
        <q-btn v-close-popup flat round dense icon="close" />
      </q-card-section>
      <q-separator />
      <q-card-section class="app-dialog-body q-gutter-md">
        <q-tabs
          :model-value="mode"
          dense
          align="left"
          class="text-primary"
          @update:model-value="$emit('update:mode', $event as 'form' | 'json')"
        >
          <q-tab name="form" label="表单" />
          <q-tab name="json" label="JSON" />
        </q-tabs>
        <q-tab-panels
          :model-value="mode"
          animated
          @update:model-value="$emit('update:mode', $event as 'form' | 'json')"
        >
          <q-tab-panel name="form" class="q-pa-none">
            <PluginSchemaForm
              :model-value="configText"
              :schema-json="target?.config_schema_json || '{}'"
              @update:model-value="$emit('update:configText', $event)"
              @validation-error="$emit('validationError', $event)"
            />
          </q-tab-panel>
          <q-tab-panel name="json" class="q-pa-none">
            <q-input
              :model-value="configText"
              type="textarea"
              autogrow
              outlined
              label="config_json"
              :error="Boolean(configError)"
              :error-message="configError"
              @update:model-value="$emit('update:configText', String($event ?? ''))"
            />
          </q-tab-panel>
        </q-tab-panels>
        <q-expansion-item icon="schema" label="默认配置 / Schema 参考">
          <pre class="app-code-block app-code-block--compact">{{
            prettyJSON(target?.default_config_json || '{}', '暂无默认配置')
          }}</pre>
          <pre class="app-code-block app-code-block--compact">{{
            prettyJSON(target?.config_schema_json || '{}', '暂无 Schema')
          }}</pre>
        </q-expansion-item>
      </q-card-section>
      <q-card-actions align="right" class="app-actions-bar app-glass-dialog__actions">
        <q-btn v-close-popup flat rounded no-caps label="取消" />
        <q-btn
          color="primary"
          rounded
          unelevated
          no-caps
          label="保存"
          :loading="saving"
          :disable="Boolean(configError)"
          @click="$emit('save')"
        />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import PluginSchemaForm from './PluginSchemaForm.vue';
import type { Plugin } from '../../features/plugins/types';
import { prettyJSON } from './pluginUi';

defineProps<{
  open: boolean;
  target: Plugin | null;
  configText: string;
  mode: 'form' | 'json';
  configError: string;
  saving: boolean;
}>();

defineEmits<{
  'update:open': [value: boolean];
  'update:configText': [value: string];
  'update:mode': [value: 'form' | 'json'];
  save: [];
  validationError: [message: string];
}>();
</script>
