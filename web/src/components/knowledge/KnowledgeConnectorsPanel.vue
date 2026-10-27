<template>
  <section class="app-settings-section knowledge-connectors-panel">
    <!-- 头部：标题 + 说明 + 新建 -->
    <div class="knowledge-connectors-panel__head">
      <div class="knowledge-connectors-panel__title-wrap">
        <h3 class="app-settings-section__title">{{ t('sourceSync.title') }}</h3>
        <p class="app-settings-section__hint q-mb-none q-mt-xs">{{ t('sourceSync.subtitle') }}</p>
      </div>
      <q-btn color="primary" unelevated no-caps icon="add" :label="t('sourceSync.add')" @click="openCreate" />
    </div>

    <q-banner v-if="loadError" class="bg-negative text-white q-mb-md" dense>
      {{ t('sourceSync.loadFailed') }}: {{ loadError }}
    </q-banner>

    <div v-if="!loading && connectors.length === 0" class="app-settings-section__hint q-py-lg text-center">
      {{ t('sourceSync.empty') }}
    </div>

    <!-- 连接器卡片列表 -->
    <div v-for="c in connectors" :key="c.id" class="connector-card q-mb-md">
      <div class="connector-card__head">
        <div class="connector-card__id">
          <q-badge outline color="primary" class="connector-card__kind">
            {{ c.kind === 'notion' ? t('sourceSync.kindNotion') : t('sourceSync.kindSharedDir') }}
          </q-badge>
          <span class="connector-card__name">{{ c.name || c.id }}</span>
          <q-badge :color="stateColor(c)" :label="stateLabel(c)" class="connector-card__state" />
        </div>
        <div class="connector-card__actions">
          <q-btn
            dense
            flat
            no-caps
            color="primary"
            :label="t('sourceSync.syncNow')"
            :disable="c.sync_state === 'running'"
            :loading="triggering === c.id"
            @click="trigger(c)"
          />
          <q-btn dense flat no-caps :label="t('sourceSync.edit')" @click="openEdit(c)" />
          <q-btn dense flat no-caps color="negative" :label="t('sourceSync.remove')" @click="remove(c)" />
        </div>
      </div>
      <div class="connector-card__meta app-settings-section__hint">
        <span>ID: {{ c.id }}</span>
        <span v-if="c.last_success_at">· OK {{ fmtTime(c.last_success_at) }}</span>
        <span v-if="c.next_run_at && c.enabled">· next {{ fmtTime(c.next_run_at) }}</span>
        <span v-if="c.error_message" class="text-negative">· {{ c.error_message }}</span>
      </div>

      <!-- 展开区：runs / items / 凭据 -->
      <q-expansion-item dense dense-toggle :label="t('sourceSync.runs')" @show="loadRuns(c.id)">
        <q-markup-table dense flat class="connector-card__table">
          <thead>
            <tr>
              <th>{{ t('sourceSync.colStarted') }}</th>
              <th>{{ t('sourceSync.colStatus') }}</th>
              <th>{{ t('sourceSync.colSeen') }}</th>
              <th>{{ t('sourceSync.colDeleted') }}</th>
              <th>{{ t('sourceSync.colMissUnknown') }}</th>
              <th>{{ t('sourceSync.colError') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="r in runsByConn[c.id] || []" :key="r.id">
              <td>{{ fmtTime(r.started_at) }}</td>
              <td>{{ r.status }}</td>
              <td>{{ r.created }}/{{ r.updated }}/{{ r.unchanged }}</td>
              <td>{{ r.deleted }}</td>
              <td>{{ r.miss_unknown }}</td>
              <td class="text-negative">{{ r.error_code || '' }}</td>
            </tr>
          </tbody>
        </q-markup-table>
      </q-expansion-item>
      <q-expansion-item dense dense-toggle :label="t('sourceSync.itemsMissing')" @show="loadMissing(c.id)">
        <q-markup-table dense flat class="connector-card__table">
          <thead>
            <tr>
              <th>{{ t('sourceSync.colExternal') }}</th>
              <th>{{ t('sourceSync.colDoc') }}</th>
              <th>{{ t('sourceSync.colMiss') }}</th>
              <th>{{ t('sourceSync.colLastSeen') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="it in missingByConn[c.id] || []" :key="it.external_id">
              <td class="ellipsis">{{ it.external_id }}</td>
              <td class="ellipsis">{{ it.document_id }}</td>
              <td>{{ it.miss_count }}</td>
              <td>{{ fmtTime(it.last_seen_at) }}</td>
            </tr>
          </tbody>
        </q-markup-table>
      </q-expansion-item>
      <q-expansion-item dense dense-toggle :label="t('sourceSync.credTitle')" @show="loadCredentials(c.id)">
        <div class="q-pa-sm">
          <div v-for="cred in credsByConn[c.id] || []" :key="cred.credential_key" class="connector-card__credrow">
            <code>{{ cred.credential_key }}</code>
            <span class="app-settings-section__hint">{{ cred.configured ? cred.masked_preview : t('sourceSync.credNone') }}</span>
            <q-btn dense flat no-caps color="negative" size="sm" :label="t('sourceSync.credDelete')" @click="removeCredential(c.id, cred.credential_key)" />
          </div>
          <div class="connector-card__credform">
            <q-input v-model="credForm[c.id].key" dense outlined :label="t('sourceSync.credKey')" class="connector-card__credkey" />
            <q-input
              v-model="credForm[c.id].secret"
              dense
              outlined
              type="password"
              :label="t('sourceSync.credSecret')"
              :placeholder="credHasAny(c.id) ? t('sourceSync.credSecretPhSet') : t('sourceSync.credSecretPhNew')"
              class="connector-card__credsecret"
            />
            <q-btn
              dense
              unelevated
              no-caps
              color="primary"
              :label="t('sourceSync.credSave')"
              :disable="!credForm[c.id].key || !credForm[c.id].secret"
              :loading="credSaving === c.id"
              @click="saveCredential(c.id)"
            />
          </div>
          <p class="app-settings-section__hint q-mb-none">{{ t('sourceSync.credNoKeyHint') }}</p>
        </div>
      </q-expansion-item>
    </div>

    <!-- 编辑抽屉 -->
    <q-dialog v-model="dialogOpen" persistent>
      <q-card class="knowledge-connectors-panel__dialog">
        <q-card-section>
          <div class="text-h6">{{ editing?.id ? t('sourceSync.edit') : t('sourceSync.add') }}</div>
        </q-card-section>
        <q-card-section class="q-gutter-md">
          <q-select
            v-if="!editing?.id"
            v-model="form.kind"
            :options="[
              { label: t('sourceSync.kindSharedDir'), value: 'shared_dir' },
              { label: t('sourceSync.kindNotion'), value: 'notion' },
            ]"
            emit-value
            map-options
            outlined
            dense
            :label="t('sourceSync.formKind')"
          />
          <q-input v-model="form.name" outlined dense :label="t('sourceSync.formName')" />
          <q-input v-if="form.kind === 'shared_dir'" v-model="form.root" outlined dense :label="t('sourceSync.formRoot')" />
          <template v-else>
            <q-input v-model="form.credential_key" outlined dense :label="t('sourceSync.formCredentialKey')" />
            <q-input v-model="form.query" outlined dense :label="t('sourceSync.formQuery')" />
          </template>
          <q-input v-model.number="form.interval_seconds" type="number" outlined dense :label="t('sourceSync.formInterval')" />
          <q-toggle v-model="form.enabled" :label="t('sourceSync.formEnabled')" />
          <div class="row q-gutter-sm">
            <q-input v-model.number="form.max_items_per_run" type="number" outlined dense class="col" :label="t('sourceSync.formMaxItems')" />
            <q-input v-model.number="form.max_deletes_per_run" type="number" outlined dense class="col" :label="t('sourceSync.formMaxDeletes')" />
            <q-input v-model.number="form.max_fetch_bytes_per_run" type="number" outlined dense class="col" :label="t('sourceSync.formMaxBytes')" />
          </div>
          <p class="app-settings-section__hint q-mb-none">{{ t('sourceSync.visibilityHint') }}</p>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat no-caps :label="t('sourceSync.cancel')" @click="dialogOpen = false" />
          <q-btn color="primary" unelevated no-caps :label="t('sourceSync.save')" :loading="saving" @click="save" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </section>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { SourceConnector, SourceCredentialView, SourceSyncItem, SourceSyncRun } from '../../features/knowledge/types';
import {
  createSourceConnector,
  deleteSourceConnector,
  deleteSourceCredential,
  listSourceConnectors,
  listSourceCredentials,
  listSourceSyncItems,
  listSourceSyncRuns,
  triggerSourceSync,
  updateSourceConnector,
  upsertSourceCredential,
} from '../../features/knowledge/api';
import { useSourceSyncWs } from '../../features/knowledge/useSourceSyncWs';

const { t } = useI18n();

const connectors = ref<SourceConnector[]>([]);
const loading = ref(false);
const loadError = ref('');
const triggering = ref('');
const saving = ref(false);
const dialogOpen = ref(false);
const editing = ref<SourceConnector | null>(null);
const runsByConn = reactive<Record<string, SourceSyncRun[]>>({});
const missingByConn = reactive<Record<string, SourceSyncItem[]>>({});
const credsByConn = reactive<Record<string, SourceCredentialView[]>>({});
const credForm = reactive<Record<string, { key: string; secret: string }>>({});
const credSaving = ref('');

const form = reactive({
  kind: 'shared_dir' as string,
  name: '',
  root: '',
  credential_key: 'default',
  query: '',
  interval_seconds: 900,
  enabled: true,
  max_items_per_run: 500,
  max_deletes_per_run: 200,
  max_fetch_bytes_per_run: 512 * 1024 * 1024,
});

function fmtTime(iso: string): string {
  if (!iso) return '';
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString();
}

function stateLabel(c: SourceConnector): string {
  switch (c.sync_state) {
    case 'running':
      return t('sourceSync.stateRunning');
    case 'error':
      return t('sourceSync.stateError');
    case 'auth_paused':
      return t('sourceSync.stateAuthPaused');
    default:
      return t('sourceSync.stateIdle');
  }
}

function stateColor(c: SourceConnector): string {
  switch (c.sync_state) {
    case 'running':
      return 'info';
    case 'error':
      return 'negative';
    case 'auth_paused':
      return 'warning';
    default:
      return 'positive';
  }
}

async function refresh() {
  loading.value = true;
  loadError.value = '';
  try {
    connectors.value = await listSourceConnectors();
  } catch (e) {
    console.warn('sourceSync list failed', e);
    loadError.value = String(e);
  } finally {
    loading.value = false;
  }
}

async function trigger(c: SourceConnector) {
  triggering.value = c.id;
  try {
    await triggerSourceSync(c.id);
    await refresh();
  } catch (e) {
    console.warn('sourceSync trigger failed', e);
  } finally {
    triggering.value = '';
  }
}

async function remove(c: SourceConnector) {
  if (!window.confirm(t('sourceSync.removeConfirm'))) return;
  try {
    await deleteSourceConnector(c.id);
    await refresh();
  } catch (e) {
    console.warn('sourceSync delete failed', e);
  }
}

async function loadRuns(id: string) {
  try {
    runsByConn[id] = await listSourceSyncRuns(id);
  } catch (e) {
    console.warn('sourceSync runs failed', e);
  }
}

async function loadMissing(id: string) {
  try {
    missingByConn[id] = await listSourceSyncItems(id, 'missing');
  } catch (e) {
    console.warn('sourceSync items failed', e);
  }
}

async function loadCredentials(id: string) {
  try {
    credsByConn[id] = await listSourceCredentials(id);
  } catch (e) {
    console.warn('sourceSync credentials failed', e);
  }
  if (!credForm[id]) credForm[id] = { key: 'default', secret: '' };
}

function credHasAny(id: string): boolean {
  return (credsByConn[id] || []).some((v) => v.configured);
}

async function saveCredential(id: string) {
  const f = credForm[id];
  if (!f || !f.key || !f.secret) return;
  credSaving.value = id;
  try {
    await upsertSourceCredential(id, { credential_key: f.key, secret: f.secret });
    f.secret = '';
    await loadCredentials(id);
    await refresh(); // auth_paused 唤醒面
  } catch (e) {
    console.warn('sourceSync credential save failed', e);
  } finally {
    credSaving.value = '';
  }
}

async function removeCredential(id: string, key: string) {
  try {
    await deleteSourceCredential(id, key);
    await loadCredentials(id);
  } catch (e) {
    console.warn('sourceSync credential delete failed', e);
  }
}

function openCreate() {
  editing.value = null;
  Object.assign(form, {
    kind: 'shared_dir', name: '', root: '', credential_key: 'default', query: '',
    interval_seconds: 900, enabled: true,
    max_items_per_run: 500, max_deletes_per_run: 200, max_fetch_bytes_per_run: 512 * 1024 * 1024,
  });
  dialogOpen.value = true;
}

function openEdit(c: SourceConnector) {
  editing.value = c;
  form.kind = c.kind;
  form.name = c.name;
  form.interval_seconds = c.interval_seconds;
  form.enabled = c.enabled;
  form.max_items_per_run = c.max_items_per_run;
  form.max_deletes_per_run = c.max_deletes_per_run;
  form.max_fetch_bytes_per_run = Number(c.max_fetch_bytes_per_run);
  // config_json read-modify-write 回环（sanitize 后出库——credential_key 保活）。
  try {
    const cfg = JSON.parse(c.config_json || '{}') as Record<string, string>;
    form.root = cfg.root ?? '';
    form.credential_key = cfg.credential_key ?? 'default';
    form.query = cfg.query ?? '';
  } catch (e) {
    console.warn('sourceSync config parse failed', e);
    form.root = '';
    form.credential_key = 'default';
    form.query = '';
  }
  dialogOpen.value = true;
}

async function save() {
  saving.value = true;
  try {
    const config: Record<string, string> = form.kind === 'shared_dir'
      ? { root: form.root }
      : { credential_key: form.credential_key, ...(form.query ? { query: form.query } : {}) };
    const input = {
      name: form.name,
      config_json: JSON.stringify(config),
      interval_seconds: Math.max(900, Number(form.interval_seconds) || 900),
      enabled: form.enabled,
      max_items_per_run: form.max_items_per_run,
      max_deletes_per_run: form.max_deletes_per_run,
      max_fetch_bytes_per_run: form.max_fetch_bytes_per_run,
    };
    if (editing.value?.id) {
      await updateSourceConnector(editing.value.id, input);
    } else {
      await createSourceConnector({ ...input, kind: form.kind });
    }
    dialogOpen.value = false;
    await refresh();
  } catch (e) {
    console.warn('sourceSync save failed', e);
  } finally {
    saving.value = false;
  }
}

// WS 帧 → 列表刷新（800ms 尾触发防抖；started+finished 双帧收敛一次）。
useSourceSyncWs(() => '', () => {
  void refresh();
});

onMounted(refresh);
</script>

<style lang="scss" scoped>
.knowledge-connectors-panel {
  &__head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 12px;
    margin-bottom: 16px;
  }

  &__dialog {
    min-width: 520px;
    max-width: 720px;
  }
}

.connector-card {
  border: 1px solid var(--color-border-soft);
  border-radius: 8px;
  padding: 12px;

  &__head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
  }

  &__id {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
  }

  &__name {
    font-weight: 600;
  }

  &__meta {
    margin-top: 6px;
    font-size: 12px;
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
  }

  &__table {
    font-size: 12px;
  }

  &__credrow {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 8px;
  }

  &__credform {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  &__credkey {
    width: 160px;
  }

  &__credsecret {
    flex: 1;
  }
}
</style>
