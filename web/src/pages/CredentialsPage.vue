<template>
  <q-page class="app-standard-page app-registry-page credentials-page">
    <AppPageHero
      :kicker="t('credentialsPage.kicker')"
      :title="t('credentialsPage.title')"
      :subtitle="t('credentialsPage.subtitle')"
    >
      <template #actions>
        <q-btn flat rounded no-caps icon="upload_file" :label="t('credentialsPage.btnImport')" @click="openImport" />
        <q-btn
          color="primary"
          rounded
          unelevated
          no-caps
          icon="add"
          :label="t('credentialsPage.btnCreate')"
          @click="openCreate"
        />
      </template>
    </AppPageHero>

    <q-tabs
      v-model="tab"
      dense
      no-caps
      active-color="primary"
      indicator-color="primary"
      align="left"
      class="credentials-page__tabs"
    >
      <q-tab name="credentials" :label="t('credentialsPage.tabCredentials')" />
      <q-tab name="executions" :label="t('credentialsPage.tabExecutions')" />
    </q-tabs>

    <q-tab-panels v-model="tab" animated class="credentials-page__panels">
      <!-- 凭据清单 -->
      <q-tab-panel name="credentials" class="q-pa-none">
        <AppPageToolbar>
          <q-input
            v-model="siteFilter"
            class="app-page-toolbar__search"
            dense
            outlined
            clearable
            debounce="200"
            :label="t('credentialsPage.filterSite')"
          >
            <template #prepend><q-icon name="search" /></template>
          </q-input>
          <template #actions>
            <q-btn
              flat
              rounded
              no-caps
              icon="refresh"
              :label="t('credentialsPage.btnRefresh')"
              :loading="loading"
              @click="loadRows"
            />
          </template>
        </AppPageToolbar>

        <AppStatePanel
          v-if="credentialState === 'loading'"
          variant="loading"
          :rows="5"
        />
        <AppStatePanel
          v-else-if="credentialState === 'error'"
          variant="error"
          :title="error"
          :action-label="t('credentialsPage.retry')"
          action-icon="refresh"
          @action="loadRows"
        />
        <AppStatePanel
          v-else-if="credentialState === 'forbidden'"
          variant="forbidden"
        />
        <AppStatePanel
          v-else-if="credentialState === 'empty'"
          variant="empty"
          :title="t('credentialsPage.empty')"
        />

        <div v-else class="app-registry-table-shell">
          <AppStatePanel
            v-if="credentialState === 'partial'"
            class="q-mb-md"
            variant="partial"
            dense
            :description="error"
            :action-label="t('credentialsPage.retry')"
            action-icon="refresh"
            @action="loadRows"
          />
          <q-table
            :rows="rows"
            :columns="credentialColumns"
            row-key="id"
            flat
            :loading="loading"
            :pagination="{ rowsPerPage: 0 }"
            hide-pagination
          >
            <template #body-cell-sensitivity="props">
              <q-td :props="props">
                <q-badge
                  v-if="props.row.sensitivity === 'high'"
                  color="negative"
                  outline
                  :label="t('credentialsPage.sensitivityHigh')"
                />
                <q-badge v-else color="primary" outline :label="t('credentialsPage.sensitivityNormal')" />
              </q-td>
            </template>
            <template #body-cell-secret_kind="props">
              <q-td :props="props">
                {{ t(`credentialsPage.kind_${props.row.secret_kind}`) }}
              </q-td>
            </template>
            <template #body-cell-actions="props">
              <q-td :props="props" class="text-right">
                <q-btn
                  flat
                  dense
                  round
                  icon="edit"
                  :aria-label="t('credentialsPage.btnEdit')"
                  @click="openEdit(props.row)"
                />
                <q-btn
                  flat
                  dense
                  round
                  icon="delete"
                  color="negative"
                  :aria-label="t('credentialsPage.btnDelete')"
                  @click="confirmDelete(props.row)"
                />
              </q-td>
            </template>
            <template #no-data>
              <div class="full-width text-center q-pa-lg text-grey-7">
                {{ t('credentialsPage.empty') }}
              </div>
            </template>
          </q-table>

          <AppRegistryPagination
            v-model:page="page"
            v-model:page-size="pageSize"
            :page-max="pageMax"
            :total="total"
            :loading="loading"
            :label="t('credentialsPage.paginationLabel')"
          />
        </div>
      </q-tab-panel>

      <!-- 执行审计 -->
      <q-tab-panel name="executions" class="q-pa-none">
        <AppPageToolbar>
          <q-input
            v-model="execSiteFilter"
            class="app-page-toolbar__search"
            dense
            outlined
            clearable
            debounce="200"
            :label="t('credentialsPage.filterSite')"
          >
            <template #prepend><q-icon name="search" /></template>
          </q-input>
          <template #actions>
            <q-btn
              flat
              rounded
              no-caps
              icon="refresh"
              :label="t('credentialsPage.btnRefresh')"
              :loading="execLoading"
              @click="loadExecutions"
            />
          </template>
        </AppPageToolbar>

        <AppStatePanel
          v-if="execState === 'loading'"
          variant="loading"
          :rows="5"
        />
        <AppStatePanel
          v-else-if="execState === 'error'"
          variant="error"
          :title="execError"
          :action-label="t('credentialsPage.retry')"
          action-icon="refresh"
          @action="loadExecutions"
        />
        <AppStatePanel
          v-else-if="execState === 'forbidden'"
          variant="forbidden"
        />
        <AppStatePanel
          v-else-if="execState === 'empty'"
          variant="empty"
          :title="t('credentialsPage.emptyExecutions')"
        />

        <div v-else class="app-registry-table-shell">
          <AppStatePanel
            v-if="execState === 'partial'"
            class="q-mb-md"
            variant="partial"
            dense
            :description="execError"
            :action-label="t('credentialsPage.retry')"
            action-icon="refresh"
            @action="loadExecutions"
          />
          <q-table
            :rows="execRows"
            :columns="executionColumns"
            row-key="id"
            flat
            :loading="execLoading"
            :pagination="{ rowsPerPage: 0 }"
            hide-pagination
          >
            <template #body-cell-verdict="props">
              <q-td :props="props">
                <q-badge
                  :color="
                    props.row.verdict === 'allowed'
                      ? 'positive'
                      : props.row.verdict === 'failed'
                        ? 'warning'
                        : 'negative'
                  "
                  outline
                  :label="t(`credentialsPage.verdict_${props.row.verdict}`)"
                />
              </q-td>
            </template>
            <template #no-data>
              <div class="full-width text-center q-pa-lg text-grey-7">
                {{ t('credentialsPage.emptyExecutions') }}
              </div>
            </template>
          </q-table>

          <AppRegistryPagination
            v-model:page="execPage"
            v-model:page-size="execPageSize"
            :page-max="execPageMax"
            :total="execTotal"
            :loading="execLoading"
            :label="t('credentialsPage.paginationLabel')"
          />
        </div>
      </q-tab-panel>
    </q-tab-panels>

    <!-- 新建 / 编辑对话框 -->
    <AppDialog
      v-model="editorOpen"
      persistent
      size="md"
      :title="editingId ? t('credentialsPage.dialogEditTitle') : t('credentialsPage.dialogCreateTitle')"
    >
      <div class="q-gutter-md">
        <q-input v-model="form.name" dense outlined :label="t('credentialsPage.fieldName')" :disable="saving" />
        <q-input
          v-model="form.site"
          dense
          outlined
          :label="t('credentialsPage.fieldSite')"
          :hint="t('credentialsPage.fieldSiteHint')"
          :disable="saving || !!editingId"
          class="mono"
        />
        <q-input
          v-model="form.url"
          dense
          outlined
          :label="t('credentialsPage.fieldUrl')"
          :disable="saving"
          class="mono"
        />
        <q-input v-model="form.username" dense outlined :label="t('credentialsPage.fieldUsername')" :disable="saving" />
        <div class="row q-col-gutter-md">
          <div class="col-6">
            <q-select
              v-model="form.secret_kind"
              dense
              outlined
              :options="secretKindOptions"
              :label="t('credentialsPage.fieldKind')"
              :disable="saving || !!editingId"
              emit-value
              map-options
            />
          </div>
          <div class="col-6">
            <q-select
              v-model="form.sensitivity"
              dense
              outlined
              :options="sensitivityOptions"
              :label="t('credentialsPage.fieldSensitivity')"
              :hint="t('credentialsPage.fieldSensitivityHint')"
              :disable="saving"
              emit-value
              map-options
              clearable
            />
          </div>
        </div>
        <q-input
          v-model="form.secret"
          dense
          outlined
          type="password"
          :label="editingId ? t('credentialsPage.fieldSecretRotate') : t('credentialsPage.fieldSecret')"
          :hint="t('credentialsPage.fieldSecretHint')"
          :disable="saving"
          autocomplete="new-password"
          class="mono"
        />
        <q-input
          v-model="form.note"
          dense
          outlined
          type="textarea"
          autogrow
          :label="t('credentialsPage.fieldNote')"
          :disable="saving"
        />
      </div>
      <template #actions>
        <q-btn flat no-caps :label="t('credentialsPage.btnCancel')" :disable="saving" @click="editorOpen = false" />
        <q-btn
          color="primary"
          unelevated
          no-caps
          :label="t('credentialsPage.btnSave')"
          :loading="saving"
          @click="saveCredential"
        />
      </template>
    </AppDialog>

    <!-- Bitwarden 导入对话框 -->
    <AppDialog v-model="importOpen" persistent size="md" :title="t('credentialsPage.importTitle')">
      <div class="q-mb-sm text-body2">{{ t('credentialsPage.importHint') }}</div>
      <q-input
        v-model="importJson"
        dense
        outlined
        type="textarea"
        autogrow
        :label="t('credentialsPage.importJsonLabel')"
        :disable="importing"
        class="mono"
      />
      <template #actions>
        <q-btn flat no-caps :label="t('credentialsPage.btnCancel')" :disable="importing" @click="importOpen = false" />
        <q-btn
          color="primary"
          unelevated
          no-caps
          :label="t('credentialsPage.btnImportRun')"
          :loading="importing"
          @click="runImport"
        />
      </template>
    </AppDialog>
  </q-page>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import AppDialog from '../components/common/AppDialog.vue';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppPageToolbar from '../components/layout/AppPageToolbar.vue';
import AppRegistryPagination from '../components/layout/AppRegistryPagination.vue';
import { useCredentialsPage } from '../features/credentials/useCredentialsPage';
import { CREDENTIAL_SECRET_KIND_OPTIONS } from '../features/credentials/types';
import { resolvePageState } from './pageState';

const {
  t,
  tab,
  rows,
  total,
  loading,
  error,
  forbidden,
  siteFilter,
  page,
  pageSize,
  pageMax,
  loadRows,
  editorOpen,
  editingId,
  saving,
  form,
  openCreate,
  openEdit,
  saveCredential,
  confirmDelete,
  importOpen,
  importJson,
  importing,
  openImport,
  runImport,
  execRows,
  execTotal,
  execLoading,
  execError,
  execForbidden,
  execSiteFilter,
  execPage,
  execPageSize,
  execPageMax,
  loadExecutions,
} = useCredentialsPage();

const credentialState = computed(() =>
  resolvePageState({
    loading: loading.value,
    error: error.value,
    count: rows.value.length,
    forbidden: forbidden.value,
  }),
);

const execState = computed(() =>
  resolvePageState({
    loading: execLoading.value,
    error: execError.value,
    count: execRows.value.length,
    forbidden: execForbidden.value,
  }),
);

const secretKindOptions = computed(() =>
  CREDENTIAL_SECRET_KIND_OPTIONS.map((v) => ({ value: v, label: t(`credentialsPage.kind_${v}`) })),
);
const sensitivityOptions = computed(() => [
  { value: 'normal', label: t('credentialsPage.sensitivityNormal') },
  { value: 'high', label: t('credentialsPage.sensitivityHigh') },
]);

const credentialColumns = computed(() => [
  { name: 'name', label: t('credentialsPage.colName'), field: 'name', align: 'left' as const, sortable: false },
  { name: 'site', label: t('credentialsPage.colSite'), field: 'site', align: 'left' as const, classes: 'mono' },
  { name: 'username', label: t('credentialsPage.colUsername'), field: 'username', align: 'left' as const },
  { name: 'secret_kind', label: t('credentialsPage.colKind'), field: 'secret_kind', align: 'left' as const },
  { name: 'sensitivity', label: t('credentialsPage.colSensitivity'), field: 'sensitivity', align: 'left' as const },
  {
    name: 'updated_at',
    label: t('credentialsPage.colUpdatedAt'),
    field: 'updated_at',
    align: 'left' as const,
    classes: 'mono',
  },
  { name: 'actions', label: '', field: 'id', align: 'right' as const },
]);

const executionColumns = computed(() => [
  {
    name: 'created_at',
    label: t('credentialsPage.colTime'),
    field: 'created_at',
    align: 'left' as const,
    classes: 'mono',
  },
  { name: 'site', label: t('credentialsPage.colSite'), field: 'site', align: 'left' as const, classes: 'mono' },
  { name: 'action', label: t('credentialsPage.colAction'), field: 'action', align: 'left' as const },
  { name: 'actor_type', label: t('credentialsPage.colActorType'), field: 'actor_type', align: 'left' as const },
  {
    name: 'actor_key',
    label: t('credentialsPage.colActorKey'),
    field: 'actor_key',
    align: 'left' as const,
    classes: 'mono',
  },
  { name: 'verdict', label: t('credentialsPage.colVerdict'), field: 'verdict', align: 'left' as const },
  { name: 'reason', label: t('credentialsPage.colReason'), field: 'reason', align: 'left' as const },
]);
</script>

<style lang="sass" scoped>
.credentials-page__tabs
  margin-bottom: 8px

.credentials-page__panels
  background: transparent

.credentials-page__dialog
  width: 560px
  max-width: 92vw
  border-radius: 12px

// 数字/站点/URL/账号一律等宽 + tabular-nums（AGENTS.md §6.9）。
.mono,
:deep(.mono)
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace
  font-variant-numeric: tabular-nums
</style>
