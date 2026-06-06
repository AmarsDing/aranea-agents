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
          <q-input
            v-model="form.url"
            dense
            outlined
            :label="t('webhooksPage.fieldUrl')"
            :hint="t('webhooksPage.fieldUrlHint')"
          />
          <q-input
            v-model="form.secret"
            dense
            outlined
            type="password"
            :label="t('webhooksPage.fieldSecret')"
            :hint="editingId ? t('webhooksPage.fieldSecretHintEdit') : t('webhooksPage.fieldSecretHintCreate')"
          />
          <div>
            <div class="text-caption q-mb-xs">{{ t('webhooksPage.fieldEventTypes') }}</div>
            <div class="q-gutter-sm">
              <q-checkbox
                v-for="et in WEBHOOK_EVENT_TYPES"
                :key="et.value"
                v-model="selectedEventTypes"
                :val="et.value"
                :label="t(et.labelKey)"
                dense
              />
            </div>
          </div>
          <div>
            <div class="text-caption q-mb-xs">{{ t('webhooksPage.fieldHeaders') }}</div>
            <div v-for="(entry, idx) in headerEntries" :key="idx" class="row items-center q-gutter-x-sm q-mb-xs">
              <q-input v-model="entry.key" dense outlined :placeholder="t('webhooksPage.headerKey')" class="col-4" />
              <q-input v-model="entry.value" dense outlined :placeholder="t('webhooksPage.headerValue')" class="col" />
              <q-btn flat dense round icon="remove" color="negative" size="sm" @click="headerEntries.splice(idx, 1)" />
            </div>
            <q-btn
              flat
              dense
              no-caps
              icon="add"
              :label="t('webhooksPage.addHeader')"
              color="primary"
              size="sm"
              @click="headerEntries.push({ key: '', value: '' })"
            />
          </div>
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
import { WEBHOOK_EVENT_TYPES, type WebhookRow } from '../../features/webhooks/types';

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
  enabled: true,
});

const selectedEventTypes = reactive<string[]>([]);
const headerEntries = reactive<{ key: string; value: string }[]>([]);

function reset() {
  form.name = '';
  form.url = '';
  form.secret = '';
  form.enabled = true;
  selectedEventTypes.splice(0, selectedEventTypes.length);
  headerEntries.splice(0, headerEntries.length);
}

function fill(row: WebhookRow) {
  form.name = row.name;
  form.url = row.url;
  // Never prefill secret — backend returns masked value on list/get
  form.secret = '';
  form.enabled = row.enabled;
  // Parse event types from JSON
  selectedEventTypes.splice(0, selectedEventTypes.length);
  if (row.event_types_json) {
    try {
      const types = JSON.parse(row.event_types_json);
      if (Array.isArray(types)) {
        selectedEventTypes.push(...types.filter((v: unknown) => typeof v === 'string'));
      }
    } catch {
      /* ignore invalid JSON */
    }
  }
  // Parse headers from Record to key-value entries
  headerEntries.splice(0, headerEntries.length);
  if (row.headers && typeof row.headers === 'object') {
    for (const [key, value] of Object.entries(row.headers)) {
      headerEntries.push({ key, value: String(value) });
    }
  }
}

function getPayload(): {
  name: string;
  url: string;
  event_types_json: string;
  secret?: string;
  headers: Record<string, string>;
  enabled: boolean;
} {
  const headers: Record<string, string> = {};
  for (const entry of headerEntries) {
    const k = entry.key?.trim();
    if (k) headers[k] = entry.value ?? '';
  }
  const payload: {
    name: string;
    url: string;
    event_types_json: string;
    secret?: string;
    headers: Record<string, string>;
    enabled: boolean;
  } = {
    name: form.name,
    url: form.url,
    event_types_json: JSON.stringify(selectedEventTypes),
    headers,
    enabled: form.enabled,
  };
  // Only include secret when non-empty (empty = keep original on update, or unsigned on create)
  if (form.secret.trim()) {
    payload.secret = form.secret;
  }
  return payload;
}

defineExpose({ form, reset, fill, getPayload });
</script>
