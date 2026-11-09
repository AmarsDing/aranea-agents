<template>
  <q-page class="app-standard-page apps-page">
    <AppPageHero :kicker="t('appsPage.kicker')" :title="t('appsPage.title')" :subtitle="t('appsPage.subtitle')">
      <template #actions>
        <q-btn flat rounded no-caps icon="refresh" :label="t('appsPage.btnRefresh')" :loading="loading" @click="load" />
        <q-btn
          unelevated
          rounded
          no-caps
          color="primary"
          icon="add"
          :label="t('appsPage.btnNew')"
          @click="openCreate"
        />
      </template>
    </AppPageHero>

    <q-banner v-if="pageError" rounded class="app-page-error-banner q-mb-md" role="alert">
      {{ pageError }}
      <template #action>
        <q-btn flat dense :label="t('appsPage.retry')" @click="load" />
      </template>
    </q-banner>

    <div v-if="!loading && !apps.length" class="apps-empty text-caption">
      {{ t('appsPage.empty') }}
    </div>

    <section v-for="g in grouped" :key="g.name" class="apps-group">
      <div class="apps-group__title">{{ g.name || t('appsPage.defaultGroup') }}</div>
      <div class="apps-grid">
        <q-card v-for="app in g.items" :key="app.id" class="apps-card" flat bordered>
          <q-card-section class="apps-card__head">
            <q-icon :name="app.icon || 'open_in_new'" size="28px" class="apps-card__icon" />
            <div class="apps-card__meta">
              <div class="apps-card__name">{{ app.name }}</div>
              <div class="apps-card__url mono" :title="app.url">{{ app.url }}</div>
            </div>
          </q-card-section>
          <q-card-actions align="between">
            <q-btn
              flat
              dense
              no-caps
              icon="open_in_new"
              :label="t('appsPage.open')"
              :aria-label="t('appsPage.openAria', { name: app.name })"
              @click="openApp(app)"
            />
            <div class="row no-wrap">
              <q-btn flat dense round size="sm" icon="edit" :aria-label="t('appsPage.edit')" @click="openEdit(app)" />
              <q-btn
                flat
                dense
                round
                size="sm"
                icon="delete_outline"
                :aria-label="t('appsPage.delete')"
                @click="confirmDelete(app)"
              />
            </div>
          </q-card-actions>
        </q-card>
      </div>
    </section>

    <!-- ============ 新建/编辑应用 ============ -->
    <q-dialog v-model="editorOpen" persistent>
      <q-card class="apps-editor">
        <q-card-section class="text-h6">
          {{ editingId ? t('appsPage.editTitle') : t('appsPage.createTitle') }}
        </q-card-section>
        <q-card-section class="q-gutter-md">
          <q-banner v-if="editorError" rounded class="app-page-error-banner" role="alert">
            {{ editorError }}
          </q-banner>
          <q-input v-model="form.name" outlined dense :label="t('appsPage.fieldName')" autofocus />
          <q-input v-model="form.url" outlined dense :label="t('appsPage.fieldUrl')" hint="https://…" />
          <div class="row q-gutter-md">
            <q-input v-model="form.group_name" class="col" outlined dense :label="t('appsPage.fieldGroup')" />
            <q-input
              v-model.number="form.sort_order"
              class="col"
              outlined
              dense
              type="number"
              :label="t('appsPage.fieldSort')"
            />
          </div>
          <q-input
            v-model="form.icon"
            outlined
            dense
            :label="t('appsPage.fieldIcon')"
            hint="Material Symbols 名称，可空"
          />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat no-caps :label="t('appsPage.cancel')" @click="editorOpen = false" />
          <q-btn
            unelevated
            no-caps
            color="primary"
            :label="t('appsPage.save')"
            :loading="saving"
            :disable="!form.name.trim() || !form.url.trim()"
            @click="save"
          />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import AppPageHero from '../components/layout/AppPageHero.vue';
import { deleteApp, listApps, upsertApp } from '../features/office/api';
import type { OfficeAppRow } from '../features/office/types';

const { t } = useI18n();
const $q = useQuasar();

const apps = ref<OfficeAppRow[]>([]);
const loading = ref(false);
const pageError = ref('');

const grouped = computed(() => {
  const map = new Map<string, OfficeAppRow[]>();
  for (const a of apps.value) {
    const key = app_group(a);
    const arr = map.get(key);
    if (arr) arr.push(a);
    else map.set(key, [a]);
  }
  return [...map.entries()].sort((a, b) => a[0].localeCompare(b[0])).map(([name, items]) => ({ name, items }));
});

function app_group(a: OfficeAppRow): string {
  return a.group_name.trim();
}

async function load() {
  loading.value = true;
  pageError.value = '';
  try {
    apps.value = await listApps();
  } catch (e) {
    pageError.value = e instanceof Error ? e.message : String(e);
  } finally {
    loading.value = false;
  }
}

onMounted(() => void load());

// ---------- 新建/编辑 ----------

const editorOpen = ref(false);
const editorError = ref('');
const saving = ref(false);
const editingId = ref('');

const form = ref({
  name: '',
  url: '',
  icon: '',
  group_name: '',
  sort_order: 0,
});

function openCreate() {
  editingId.value = '';
  editorError.value = '';
  form.value = { name: '', url: '', icon: '', group_name: '', sort_order: 0 };
  editorOpen.value = true;
}

function openEdit(a: OfficeAppRow) {
  editingId.value = a.id;
  editorError.value = '';
  form.value = {
    name: a.name,
    url: a.url,
    icon: a.icon,
    group_name: a.group_name,
    sort_order: a.sort_order,
  };
  editorOpen.value = true;
}

async function save() {
  saving.value = true;
  editorError.value = '';
  try {
    await upsertApp(editingId.value, {
      name: form.value.name.trim(),
      url: form.value.url.trim(),
      icon: form.value.icon.trim(),
      group_name: form.value.group_name.trim(),
      sort_order: form.value.sort_order || 0,
    });
    editorOpen.value = false;
    await load();
  } catch (e) {
    // URL 非法等 400 就地呈现（弹层保持打开）。
    editorError.value = e instanceof Error ? e.message : String(e);
  } finally {
    saving.value = false;
  }
}

function confirmDelete(a: OfficeAppRow) {
  $q.dialog({
    title: t('appsPage.deleteConfirmTitle'),
    message: t('appsPage.deleteConfirmBody', { name: a.name }),
    cancel: true,
    persistent: true,
  }).onOk(() => {
    void (async () => {
      try {
        await deleteApp(a.id);
        await load();
      } catch (e) {
        pageError.value = e instanceof Error ? e.message : String(e);
      }
    })();
  });
}

function openApp(a: OfficeAppRow) {
  window.open(a.url, '_blank', 'noopener,noreferrer');
}
</script>

<style scoped lang="sass">
// 令牌只取 --nt-*（AGENTS.md §6）；URL 一律等宽 + tabular-nums。
.mono
  font-family: var(--nt-font-mono)
  font-variant-numeric: tabular-nums
:deep(.mono)
  font-family: var(--nt-font-mono)
  font-variant-numeric: tabular-nums

.apps-empty
  color: var(--nt-text-2)
  padding: 48px 0
  text-align: center

.apps-group
  margin-bottom: 24px

.apps-group__title
  font-size: 14px
  font-weight: 600
  color: var(--nt-text-1)
  margin-bottom: 12px

.apps-grid
  display: grid
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr))
  gap: 12px

.apps-card
  background: var(--nt-bg-1)
  border-color: var(--nt-border-1)
  border-radius: 12px

.apps-card__head
  display: flex
  align-items: center
  gap: 12px

.apps-card__icon
  color: var(--nt-accent)

.apps-card__meta
  min-width: 0

.apps-card__name
  font-weight: 600
  white-space: nowrap
  overflow: hidden
  text-overflow: ellipsis

.apps-card__url
  font-size: 12px
  color: var(--nt-text-2)
  white-space: nowrap
  overflow: hidden
  text-overflow: ellipsis

.apps-editor
  min-width: min(520px, 94vw)
</style>
