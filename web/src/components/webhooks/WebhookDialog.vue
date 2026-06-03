<template>
  <q-dialog v-model="open" persistent>
    <q-card class="app-dialog-card app-dialog-card--lg app-glass-dialog">
      <q-card-section class="app-glass-dialog__head row items-center justify-between">
        <div class="app-glass-dialog__title">
          {{ editingId ? t('webhooksPage.dialogTitleEdit') : t('webhooksPage.dialogTitleCreate') }}
        </div>
        <q-btn v-close-popup flat round dense icon="close" />
      </q-card-section>
      <q-separator />
      <div class="app-glass-dialog__scroll">
        <q-card-section class="app-dialog-body app-glass-dialog__body q-gutter-md app-form-wide">
          <div class="app-form-field-grid app-form-field-grid--2col">
            <q-input v-model="form.name" dense outlined :label="t('webhooksPage.fieldName')" />
            <q-toggle v-model="form.enabled" :label="t('webhooksPage.fieldEnabled')" />
          </div>
          <q-input v-model="form.url" dense outlined :label="t('webhooksPage.fieldUrl')" />
          <q-input v-model="form.secret" dense outlined type="password" :label="t('webhooksPage.fieldSecret')" />
          <q-input
            v-model="form.event_types_json"
            dense
            outlined
            type="textarea"
            autogrow
            :label="t('webhooksPage.fieldEventTypes')"
            :hint="t('webhooksPage.fieldEventTypesHint')"
          />
          <q-input
            v-model="form.headers_json"
            dense
            outlined
            type="textarea"
            autogrow
            :label="t('webhooksPage.fieldHeaders')"
            :hint="t('webhooksPage.fieldHeadersHint')"
          />
        </q-card-section>
      </div>
      <q-separator />
      <q-card-actions align="right" class="app-actions-bar app-glass-dialog__actions">
        <q-btn v-close-popup flat no-caps :label="t('webhooksPage.btnCancel')" />
        <q-btn
          color="primary"
          unelevated
          no-caps
          :label="t('webhooksPage.btnSave')"
          :loading="saving"
          :disable="!form.name?.trim() || !form.url?.trim()"
          @click="$emit('save')"
        />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { reactive } from 'vue';
import { useI18n } from 'vue-i18n';

const { t } = useI18n();

const open = defineModel<boolean>('open', { required: true });

const props = defineProps<{
  editingId: string;
  saving: boolean;
}>();

defineEmits<{ save: [] }>();

const form = reactive({
  name: '',
  url: '',
  secret: '',
  event_types_json: '[]',
  headers_json: '{}',
  enabled: true,
});

function reset(create: boolean) {
  form.name = '';
  form.url = '';
  form.secret = '';
  form.event_types_json = '[]';
  form.headers_json = '{}';
  form.enabled = true;
}

function fill(row: {
  name: string;
  url: string;
  secret: string;
  event_types_json: string;
  headers: Record<string, string>;
  enabled: boolean;
}) {
  form.name = row.name;
  form.url = row.url;
  form.secret = row.secret;
  form.event_types_json = row.event_types_json || '[]';
  form.headers_json = JSON.stringify(row.headers ?? {}, null, 2);
  form.enabled = row.enabled;
}

function getPayload() {
  let headers: Record<string, string> = {};
  try {
    headers = JSON.parse(form.headers_json || '{}');
  } catch {
    // intentional empty — invalid headers fall back to empty object
  }
  return {
    name: form.name,
    url: form.url,
    secret: form.secret,
    event_types_json: form.event_types_json,
    headers,
    enabled: form.enabled,
  };
}

defineExpose({ form, reset, fill, getPayload });
</script>
