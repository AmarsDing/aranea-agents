<template>
  <q-dialog :model-value="open" persistent @update:model-value="$emit('update:open', $event)">
    <q-card class="app-dialog-card app-dialog-card--md app-glass-dialog">
      <q-card-section class="app-glass-dialog__head row items-center justify-between">
        <div class="min-width-0">
          <div class="app-glass-dialog__title">{{ t('pluginsPage.config.title', { name: target?.name }) }}</div>
          <div class="app-glass-dialog__subtitle">{{ t('pluginsPage.config.subtitle') }}</div>
        </div>
        <q-btn v-close-popup flat round dense icon="close" />
      </q-card-section>
      <q-separator />
      <q-card-section class="app-dialog-body q-gutter-md">
        <q-tabs :model-value="mode" dense align="left" class="text-primary" @update:model-value="onModeUpdate">
          <q-tab name="form" :label="t('pluginsPage.config.tabForm')" />
          <q-tab name="json" :label="t('pluginsPage.config.tabJson')" />
        </q-tabs>
        <q-tab-panels :model-value="mode" animated @update:model-value="onModeUpdate">
          <q-tab-panel name="form" class="q-pa-none">
            <PluginSchemaForm
              ref="schemaFormRef"
              :model-value="configText"
              :schema-json="target?.config_schema_json || '{}'"
              @update:model-value="$emit('update:configText', $event)"
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
        <q-expansion-item icon="schema" :label="t('pluginsPage.config.defaultRef')">
          <pre class="app-code-block app-code-block--compact">{{
            prettyJSON(target?.default_config_json || '{}', t('pluginsPage.detail.noDefaultConfig'))
          }}</pre>
          <pre class="app-code-block app-code-block--compact">{{
            prettyJSON(target?.config_schema_json || '{}', t('pluginsPage.detail.noSchema'))
          }}</pre>
        </q-expansion-item>
      </q-card-section>
      <q-card-actions align="right" class="app-actions-bar app-glass-dialog__actions">
        <q-btn v-close-popup flat rounded no-caps :label="t('pluginsPage.config.cancel')" />
        <q-btn
          color="primary"
          rounded
          unelevated
          no-caps
          :label="t('pluginsPage.config.save')"
          :loading="saving"
          :disable="Boolean(configError)"
          @click="onSaveClick"
        />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import { useI18n } from 'vue-i18n';
import PluginSchemaForm from './PluginSchemaForm.vue';
import type { Plugin } from '../../features/plugins/types';
import { prettyJSON } from './pluginUi';

const props = defineProps<{
  open: boolean;
  target: Plugin | null;
  configText: string;
  mode: 'form' | 'json';
  configError: string;
  saving: boolean;
}>();

const emit = defineEmits<{
  'update:open': [value: boolean];
  'update:configText': [value: string];
  'update:mode': [value: 'form' | 'json'];
  save: [];
  validationError: [message: string];
}>();

const { t } = useI18n();

const schemaFormRef = ref<{ validationSummary: () => string } | null>(null);

// JSON 非法时禁止切回表单 Tab：表单侧会静默回退 {} 并覆盖用户原文
function onModeUpdate(value: string | number) {
  const next = value as 'form' | 'json';
  if (next === 'form' && props.mode === 'json' && props.configError) {
    emit('validationError', t('pluginsPage.config.fixJsonBeforeForm'));
    return;
  }
  emit('update:mode', next);
}

// 字段级校验错误仅在保存提交时汇总提示一次
function onSaveClick() {
  if (props.mode === 'form') {
    const summary = schemaFormRef.value?.validationSummary() ?? '';
    if (summary) {
      emit('validationError', summary);
      return;
    }
  }
  emit('save');
}
</script>
