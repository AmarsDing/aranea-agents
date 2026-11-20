<template>
  <!-- Harness › 连接 › 凭据：关键数 → 凭据 / 执行审计 → 构件卡 + 检视面板（含这条凭据最近的执行） -->
  <q-page class="app-standard-page cap-page credentials-page">
    <Teleport defer to="#app-tab-actions">
      <q-btn flat no-caps icon="upload_file" :label="t('connect.credentials.import')" @click="openImport" />
      <q-btn
        unelevated
        no-caps
        color="primary"
        icon="add"
        :label="t('connect.credentials.create')"
        @click="openCreate"
      />
    </Teleport>

    <AppStatePanel v-if="forbidden" variant="forbidden" />
    <AppStatePanel v-else-if="!loaded && loading" variant="loading" :rows="4" />
    <AppStatePanel
      v-else-if="error && !rows.length"
      variant="error"
      :description="error"
      :action-label="t('credentialsPage.retry')"
      action-icon="refresh"
      @action="loadRows"
    />

    <template v-else>
      <p class="cap-note">
        <q-icon name="construction" size="16px" />
        <span class="cap-note__body">{{ t('connect.credentials.executorNote') }}</span>
      </p>

      <ConnectGuide
        v-if="!rows.length"
        icon="key"
        :title="t('connect.credentials.guideTitle')"
        :description="t('connect.credentials.guideDesc')"
        :steps="[
          t('connect.credentials.guideStep1'),
          t('connect.credentials.guideStep2'),
          t('connect.credentials.guideStep3'),
        ]"
        :action-label="t('connect.credentials.create')"
        @action="openCreate"
      >
        <template #actions>
          <q-btn flat no-caps icon="upload_file" :label="t('connect.credentials.import')" @click="openImport" />
        </template>
      </ConnectGuide>

      <template v-else>
        <CapStats :stats="stats" :label="t('connect.common.stripLabel')" :active="sensitivityFilter" @pick="pickStat" />
        <div class="cap-bar">
          <div class="cap-seg" role="group" :aria-label="t('connect.credentials.tabVault')">
            <button type="button" :aria-pressed="tab === 'credentials'" @click="tab = 'credentials'">
              {{ t('connect.credentials.tabVault') }}
            </button>
            <button type="button" :aria-pressed="tab === 'executions'" @click="tab = 'executions'">
              {{ t('connect.credentials.tabAudit') }}
            </button>
          </div>
          <q-input
            v-if="tab === 'credentials'"
            v-model="siteFilter"
            class="cap-bar__search"
            dense
            outlined
            clearable
            debounce="150"
            :placeholder="t('connect.credentials.search')"
            :aria-label="t('connect.credentials.search')"
          >
            <template #prepend><q-icon name="search" /></template>
          </q-input>
          <q-input
            v-else
            v-model="execSiteFilter"
            class="cap-bar__search"
            dense
            outlined
            clearable
            debounce="200"
            :placeholder="t('connect.credentials.search')"
            :aria-label="t('connect.credentials.search')"
          >
            <template #prepend><q-icon name="search" /></template>
          </q-input>
          <span class="cap-bar__spacer" />
          <span v-if="tab === 'credentials'" class="cap-bar__meta cx-num">
            {{ t('connect.common.shownOf', { shown: total, total: rows.length }) }}
          </span>
          <q-btn
            flat
            dense
            round
            icon="refresh"
            :loading="tab === 'credentials' ? loading : execLoading"
            :aria-label="t('connect.common.refresh')"
            @click="tab === 'credentials' ? loadRows() : loadExecutions()"
          />
        </div>

        <div v-if="tab === 'credentials'" class="cx-workspace" :class="{ 'cx-workspace--inspecting': inspectorOpen }">
          <div class="cx-workspace__main">
            <p v-if="!pagedRows.length" class="cap-note cap-note--info">
              <q-icon name="filter_alt_off" size="16px" />
              <span class="cap-note__body">{{ t('connect.common.noMatch') }}</span>
            </p>
            <div v-else class="cap-grid">
              <CapCard
                v-for="row in pagedRows"
                :key="row.id"
                icon="key"
                :code="row.site"
                :title="row.name"
                :description="row.note"
                :status="sensitivityStatus(row)"
                :facts="cardFacts(row)"
                :selected="inspected?.id === row.id"
                @open="inspect(row)"
              >
                <template #relation>{{ t('connect.common.updatedAt') }} {{ formatTime(row.updated_at) }}</template>
                <template #actions>
                  <q-btn flat dense no-caps icon="edit" :label="t('connect.common.edit')" @click="openEdit(row)" />
                  <q-btn flat dense round icon="more_horiz" :aria-label="t('connect.common.more')">
                    <q-menu anchor="bottom right" self="top right">
                      <q-list dense>
                        <q-item v-close-popup clickable @click="confirmDelete(row)">
                          <q-item-section class="text-negative">{{ t('connect.common.remove') }}</q-item-section>
                        </q-item>
                      </q-list>
                    </q-menu>
                  </q-btn>
                </template>
              </CapCard>
            </div>
            <AppRegistryPagination
              v-if="total > pageSize"
              v-model:page="page"
              v-model:page-size="pageSize"
              :page-max="pageMax"
              :total="total"
              :loading="loading"
              :label="t('credentialsPage.paginationLabel')"
              :page-size-options="[12, 24, 48]"
            />
          </div>

          <AppRegistryInspector
            v-model="inspectorOpen"
            :title="inspected?.name ?? ''"
            :eyebrow="inspected ? `CREDENTIAL / ${inspected.site}` : undefined"
          >
            <div v-if="inspected" class="cx-inspect">
              <div class="cx-inspect__lead">
                <span class="cap-card__status">
                  <i class="cap-dot" :class="`cap-dot--${sensitivityStatus(inspected).tone}`" aria-hidden="true" />
                  {{ sensitivityStatus(inspected).label }}
                </span>
              </div>
              <dl class="cx-facts">
                <div>
                  <dt>{{ t('connect.credentials.facts.site') }}</dt>
                  <dd class="cx-num">{{ inspected.site }}</dd>
                </div>
                <div v-if="inspected.url">
                  <dt>{{ t('connect.credentials.facts.url') }}</dt>
                  <dd class="cx-num">{{ inspected.url }}</dd>
                </div>
                <div>
                  <dt>{{ t('connect.credentials.facts.username') }}</dt>
                  <dd class="cx-num">{{ inspected.username || t('connect.common.none') }}</dd>
                </div>
                <div>
                  <dt>{{ t('connect.credentials.facts.kind') }}</dt>
                  <dd>{{ t(`credentialsPage.kind_${inspected.secret_kind}`) }}</dd>
                </div>
                <div v-if="inspected.note">
                  <dt>{{ t('connect.credentials.facts.note') }}</dt>
                  <dd>{{ inspected.note }}</dd>
                </div>
                <div>
                  <dt>{{ t('connect.common.createdAt') }}</dt>
                  <dd class="cx-num">{{ formatTime(inspected.created_at) }}</dd>
                </div>
                <div>
                  <dt>{{ t('connect.common.updatedAt') }}</dt>
                  <dd class="cx-num">{{ formatTime(inspected.updated_at) }}</dd>
                </div>
              </dl>
              <section class="cx-inspect__section" aria-live="polite">
                <h3 class="cx-inspect__section-title">{{ t('connect.credentials.recentTitle') }}</h3>
                <q-linear-progress v-if="inspectedExecsLoading" indeterminate />
                <p v-else-if="!inspectedExecs.length" class="cx-inspect__empty">
                  {{ t('connect.credentials.recentEmpty') }}
                </p>
                <ul v-else class="cx-rows" role="list">
                  <li v-for="exec in inspectedExecs" :key="exec.id">
                    <i class="cap-dot" :class="`cap-dot--${verdictTone(exec.verdict)}`" aria-hidden="true" />
                    <span class="cx-rows__main">
                      {{ t(`credentialsPage.verdict_${exec.verdict}`) }} · {{ exec.action }} ·
                      {{ exec.actor_key || exec.actor_type }}
                    </span>
                    <span class="cx-rows__side">{{ formatTime(exec.created_at) }}</span>
                  </li>
                </ul>
              </section>
            </div>
            <template v-if="inspected" #footer>
              <div class="cx-inspect__actions">
                <q-btn
                  unelevated
                  no-caps
                  color="primary"
                  icon="key"
                  :label="t('connect.credentials.rotate')"
                  @click="openEdit(inspected)"
                />
                <q-btn
                  flat
                  no-caps
                  color="negative"
                  icon="delete"
                  :label="t('connect.common.remove')"
                  @click="confirmDelete(inspected)"
                />
              </div>
            </template>
          </AppRegistryInspector>
        </div>

        <div v-else class="cx-workspace__main">
          <AppStatePanel
            v-if="execState === 'error' || execState === 'forbidden'"
            :variant="execState"
            :description="execError"
            :action-label="execState === 'error' ? t('credentialsPage.retry') : ''"
            action-icon="refresh"
            @action="loadExecutions"
          />
          <AppRegistryTable
            v-else
            :rows="execRows"
            :columns="executionColumns"
            row-key="id"
            :loading="execLoading"
            hide-pagination
            :pagination="{ rowsPerPage: 0 }"
          >
            <template #body-cell-verdict="props">
              <q-td :props="props">
                <span class="cap-card__status">
                  <i class="cap-dot" :class="`cap-dot--${verdictTone(props.row.verdict)}`" aria-hidden="true" />
                  {{ t(`credentialsPage.verdict_${props.row.verdict}`) }}
                </span>
              </q-td>
            </template>
            <template #no-data>
              <p class="cx-inspect__empty">{{ t('credentialsPage.emptyExecutions') }}</p>
            </template>
          </AppRegistryTable>
          <AppRegistryPagination
            v-if="execTotal > execPageSize"
            v-model:page="execPage"
            v-model:page-size="execPageSize"
            :page-max="execPageMax"
            :total="execTotal"
            :loading="execLoading"
            :label="t('credentialsPage.paginationLabel')"
          />
        </div>
      </template>
    </template>

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
          class="cx-num"
        />
        <q-input
          v-model="form.url"
          dense
          outlined
          :label="t('credentialsPage.fieldUrl')"
          :disable="saving"
          class="cx-num"
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
          class="cx-num"
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
        class="cx-num"
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
import AppRegistryInspector from '../components/layout/AppRegistryInspector.vue';
import AppRegistryPagination from '../components/layout/AppRegistryPagination.vue';
import AppRegistryTable from '../components/layout/AppRegistryTable.vue';
import CapCard from '../components/capability/CapCard.vue';
import CapStats from '../components/capability/CapStats.vue';
import type { CapFact, CapStatus, CapTone } from '../components/capability/capTypes';
import ConnectGuide from '../components/connect/ConnectGuide.vue';
import { useCredentialsPage } from '../features/credentials/useCredentialsPage';
import {
  CREDENTIAL_SECRET_KIND_OPTIONS,
  type CredentialExecutionRow,
  type CredentialRow,
} from '../features/credentials/types';
import { formatConnectTime as formatTime } from '../features/connect/format';
import { resolvePageState } from './pageState';

const {
  t,
  tab,
  rows,
  total,
  loading,
  loaded,
  error,
  forbidden,
  siteFilter,
  sensitivityFilter,
  highCount,
  page,
  pageSize,
  pageMax,
  pagedRows,
  loadRows,
  inspected,
  inspectorOpen,
  inspectedExecs,
  inspectedExecsLoading,
  inspect,
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

const execState = computed(() =>
  resolvePageState({
    loading: execLoading.value,
    error: execError.value,
    count: execRows.value.length,
    forbidden: execForbidden.value,
  }),
);

const stats = computed(() => [
  { key: '', label: t('connect.common.all'), value: rows.value.length, clickable: true },
  {
    key: 'normal',
    label: t('connect.credentials.normal'),
    value: rows.value.length - highCount.value,
    tone: 'ok' as const,
    clickable: true,
  },
  {
    key: 'high',
    label: t('connect.credentials.high'),
    value: highCount.value,
    tone: 'watch' as const,
    clickable: true,
  },
]);

function pickStat(key: string) {
  tab.value = 'credentials';
  sensitivityFilter.value = sensitivityFilter.value === key ? '' : key;
}

function sensitivityStatus(row: CredentialRow): CapStatus {
  return row.sensitivity === 'high'
    ? { tone: 'watch', label: t('connect.credentials.high') }
    : { tone: 'ok', label: t('connect.credentials.normal') };
}

function cardFacts(row: CredentialRow): CapFact[] {
  return [
    { label: t('connect.credentials.facts.username'), value: row.username || t('connect.common.none') },
    { label: t('connect.credentials.facts.kind'), value: t(`credentialsPage.kind_${row.secret_kind}`) },
  ];
}

function verdictTone(verdict: string): CapTone {
  if (verdict === 'allowed') return 'ok';
  if (verdict === 'failed') return 'watch';
  return 'alert';
}

const secretKindOptions = computed(() =>
  CREDENTIAL_SECRET_KIND_OPTIONS.map((v) => ({ value: v, label: t(`credentialsPage.kind_${v}`) })),
);
const sensitivityOptions = computed(() => [
  { value: 'normal', label: t('credentialsPage.sensitivityNormal') },
  { value: 'high', label: t('credentialsPage.sensitivityHigh') },
]);

const executionColumns = computed(() => [
  {
    name: 'created_at',
    label: t('credentialsPage.colTime'),
    field: 'created_at',
    align: 'left' as const,
    classes: 'cx-num',
    format: (v: string) => formatTime(v),
  },
  { name: 'site', label: t('credentialsPage.colSite'), field: 'site', align: 'left' as const, classes: 'cx-num' },
  { name: 'action', label: t('credentialsPage.colAction'), field: 'action', align: 'left' as const },
  {
    name: 'actor_key',
    label: t('credentialsPage.colActorKey'),
    field: (r: CredentialExecutionRow) => r.actor_key || r.actor_type,
    align: 'left' as const,
    classes: 'cx-num',
  },
  {
    name: 'agent_id',
    label: t('connect.credentials.agent'),
    field: 'agent_id',
    align: 'left' as const,
    classes: 'cx-num',
  },
  {
    name: 'session_id',
    label: t('connect.credentials.session'),
    field: 'session_id',
    align: 'left' as const,
    classes: 'cx-num',
  },
  { name: 'verdict', label: t('credentialsPage.colVerdict'), field: 'verdict', align: 'left' as const },
  { name: 'reason', label: t('credentialsPage.colReason'), field: 'reason', align: 'left' as const },
]);
</script>
