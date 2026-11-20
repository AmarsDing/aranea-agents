<template>
  <!-- ics 只读订阅：令牌明文只在创建当次给出（服务端契约） -->
  <AppDialog v-model="open" size="md" :title="t('schedulePage.icalTitle')">
    <div class="q-gutter-md">
      <q-banner v-if="error" rounded class="app-page-error-banner" role="alert">{{ error }}</q-banner>
      <div class="row q-gutter-sm items-center">
        <q-input v-model="name" class="col" outlined dense :label="t('schedulePage.icalNameLabel')" />
        <q-btn
          unelevated
          no-caps
          color="primary"
          :label="t('schedulePage.icalCreate')"
          :loading="saving"
          :disable="!name.trim()"
          @click="createToken"
        />
      </div>
      <q-banner v-if="newFeedUrl" rounded class="sch-ical__once">
        <div class="text-caption">{{ t('schedulePage.icalOnceHint') }}</div>
        <div class="row items-center no-wrap q-gutter-sm q-mt-xs">
          <code class="sch-num sch-ical__url">{{ absolute(newFeedUrl) }}</code>
          <q-btn
            flat
            dense
            round
            size="sm"
            icon="content_copy"
            :aria-label="t('schedulePage.icalCopy')"
            @click="copyFeedUrl"
          />
        </div>
      </q-banner>
      <q-list v-if="tokens.length" separator dense>
        <q-item v-for="tk in tokens" :key="tk.id">
          <q-item-section>
            <q-item-label>{{ tk.name }}</q-item-label>
            <q-item-label caption class="sch-num">{{ fmtDay(tk.created_at) }}</q-item-label>
          </q-item-section>
          <q-item-section side>
            <q-badge v-if="tk.revoked_at" outline color="grey" :label="t('schedulePage.icalRevoked')" />
            <q-btn
              v-else
              flat
              dense
              no-caps
              size="sm"
              color="negative"
              :label="t('schedulePage.icalRevoke')"
              @click="revokeToken(tk)"
            />
          </q-item-section>
        </q-item>
      </q-list>
      <div v-else class="text-caption text-grey">{{ t('schedulePage.icalEmpty') }}</div>
    </div>
    <template #actions>
      <q-btn flat no-caps :label="t('schedulePage.cancel')" @click="open = false" />
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import AppDialog from '../../../components/common/AppDialog.vue';
import { createICalToken, listICalTokens, revokeICalToken } from '../../office/api';
import type { ICalTokenRow } from '../../office/types';
import { dayKey } from '../model';

const open = defineModel<boolean>({ required: true });

const { t } = useI18n();
const $q = useQuasar();

const tokens = ref<ICalTokenRow[]>([]);
const error = ref('');
const saving = ref(false);
const name = ref('');
const newFeedUrl = ref('');

watch(open, (v) => {
  if (!v) return;
  error.value = '';
  newFeedUrl.value = '';
  void refresh();
});

async function refresh() {
  try {
    tokens.value = await listICalTokens();
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e);
  }
}

async function createToken() {
  saving.value = true;
  error.value = '';
  try {
    const tk = await createICalToken(name.value.trim());
    newFeedUrl.value = tk.feed_url;
    name.value = '';
    await refresh();
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e);
  } finally {
    saving.value = false;
  }
}

async function revokeToken(tk: ICalTokenRow) {
  try {
    await revokeICalToken(tk.id);
    await refresh();
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e);
  }
}

function absolute(path: string): string {
  return path ? `${window.location.origin}${path}` : '';
}

async function copyFeedUrl() {
  try {
    await navigator.clipboard.writeText(absolute(newFeedUrl.value));
    $q.notify({ type: 'positive', message: t('schedulePage.icalCopied') });
  } catch {
    $q.notify({ type: 'negative', message: t('schedulePage.icalCopyFailed') });
  }
}

function fmtDay(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? '' : dayKey(d);
}
</script>
