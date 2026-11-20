<template>
  <q-page class="app-standard-page cap-page app-registry-page evolution-suggestion-page">
    <Teleport defer to="#app-tab-actions">
      <q-btn
        unelevated
        dense
        no-caps
        color="primary"
        icon="auto_fix_high"
        :label="$t('evolutionSuggestionsPage.triggerCurator')"
        :loading="triggeringCurator"
        @click="handleTriggerCurator(targetId)"
      />
    </Teleport>
    <AppPageToolbar>
      <q-btn-toggle
        v-model="targetType"
        rounded
        unelevated
        toggle-color="primary"
        :options="targetTypeOptions"
        class="q-mr-sm"
      />
      <q-input
        v-model="targetId"
        class="app-page-toolbar__search"
        dense
        outlined
        clearable
        debounce="300"
        :label="targetType === 'agent' ? 'Agent ID' : 'Skill ID'"
      >
        <template #prepend><q-icon name="search" /></template>
      </q-input>
      <q-select
        v-model="status"
        class="app-page-toolbar__field"
        dense
        outlined
        emit-value
        map-options
        :label="$t('evolutionSuggestionsPage.filterStatus')"
        :options="statusOptions"
      />
      <template #actions>
        <q-btn
          flat
          rounded
          no-caps
          icon="restart_alt"
          :label="$t('evolutionSuggestionsPage.reset')"
          @click="resetFilters"
        />
        <q-btn
          flat
          rounded
          no-caps
          icon="refresh"
          :label="$t('evolutionSuggestionsPage.refresh')"
          :loading="loading"
          @click="loadRows"
        />
      </template>
    </AppPageToolbar>

    <!-- 五态：已有数据但刷新失败 → 局部失败横幅，列表照常渲染 -->
    <AppStatePanel
      v-if="(error || forbidden) && rows.length > 0"
      variant="partial"
      dense
      class="q-mb-md"
      :description="error"
      :action-label="$t('evolutionSuggestionsPage.retry')"
      action-icon="refresh"
      @action="loadRows"
    />

    <!-- 五态：无权限 → 只渲染 forbidden 面板，不提供重试 -->
    <AppStatePanel v-else-if="forbidden" variant="forbidden" class="q-mb-md" />

    <!-- 五态：加载失败且无数据 → 只渲染可重试错误面板，不与空态并存。 -->
    <AppStatePanel
      v-else-if="error"
      variant="error"
      class="q-mb-md"
      :description="error"
      :action-label="$t('evolutionSuggestionsPage.retry')"
      action-icon="refresh"
      @action="loadRows"
    />

    <!-- 五态：首屏加载无数据 → 骨架面板 -->
    <AppStatePanel v-if="loading && rows.length === 0 && !error && !forbidden" variant="loading" :rows="5" />

    <!-- 五态：成功但无数据 → 空态 -->
    <AppStatePanel
      v-else-if="!loading && !error && !forbidden && rows.length === 0"
      variant="empty"
      :title="
        targetId || status ? $t('evolutionSuggestionsPage.emptyFiltered') : $t('evolutionSuggestionsPage.emptyDefault')
      "
      :description="
        targetId || status
          ? $t('evolutionSuggestionsPage.emptyFilteredHint')
          : $t('evolutionSuggestionsPage.emptyDefaultHint')
      "
      icon="auto_fix_high"
    />

    <template v-else-if="rows.length > 0">
      <AppRegistryTable
        :rows="rows"
        :columns="columns"
        row-key="id"
        :loading="loading"
        column-persist-key="evolution-suggestion"
      >
        <template #body-cell-targetType="props">
          <q-td :props="props">
            <q-chip dense square :color="evoTargetTypeColor(props.row.targetType)" text-color="white">
              {{ evoTargetTypeLabel(props.row.targetType) }}
            </q-chip>
          </q-td>
        </template>

        <template #body-cell-targetId="props">
          <q-td :props="props">
            <span
              class="app-registry-cell-primary ellipsis cursor-pointer"
              :title="props.row.targetId ?? ''"
              @click="openDetailDialog(props.row)"
            >
              {{ props.row.targetName || props.row.targetId || '—' }}
            </span>
          </q-td>
        </template>

        <template #body-cell-actionType="props">
          <q-td :props="props">
            <q-chip dense square :color="evoActionTypeColor(props.row.actionType)" text-color="white">
              {{ evoActionTypeLabel(props.row.actionType) }}
            </q-chip>
          </q-td>
        </template>

        <template #body-cell-status="props">
          <q-td :props="props">
            <q-chip dense square :color="evoSuggestionStatusColor(props.row.status)" text-color="white">
              {{ evoSuggestionStatusLabel(props.row.status) }}
            </q-chip>
          </q-td>
        </template>

        <template #body-cell-lifecycleStatus="props">
          <q-td :props="props">
            <q-chip dense square :color="evoLifecycleStatusColor(props.row.lifecycleStatus)" text-color="white">
              {{ evoLifecycleStatusLabel(props.row.lifecycleStatus) }}
            </q-chip>
          </q-td>
        </template>

        <template #body-cell-triggerReason="props">
          <q-td :props="props">
            <span class="app-registry-cell-sub ellipsis app-field-max-240" :title="props.row.triggerReason ?? ''">
              {{ props.row.triggerReason || '—' }}
            </span>
          </q-td>
        </template>

        <template #body-cell-sandboxPassed="props">
          <q-td :props="props">
            <template v-if="props.row.sandboxPassed === true">
              <q-icon name="check_circle" color="positive" size="sm" />
            </template>
            <template v-else-if="props.row.sandboxPassed === false">
              <q-icon name="cancel" color="negative" size="sm" />
            </template>
            <template v-else>
              <span class="text-grey-7">—</span>
            </template>
          </q-td>
        </template>

        <template #body-cell-createdAt="props">
          <q-td :props="props">
            <span class="app-registry-cell-sub">{{ formatDate(props.row.createdAt) }}</span>
          </q-td>
        </template>

        <template #body-cell-actions="props">
          <q-td :props="props">
            <div class="app-registry-cell-actions">
              <q-btn flat dense round icon="visibility" color="primary" @click="openDetailDialog(props.row)">
                <q-tooltip>{{ $t('evolutionSuggestionsPage.actionView') }}</q-tooltip>
              </q-btn>
              <template v-if="props.row.status === 'pending'">
                <q-btn
                  flat
                  dense
                  round
                  icon="check"
                  color="positive"
                  :loading="approvingId === props.row.id"
                  @click="approveSuggestion(props.row)"
                >
                  <q-tooltip>{{ $t('evolutionSuggestionsPage.actionApprove') }}</q-tooltip>
                </q-btn>
                <q-btn flat dense round icon="close" color="negative" @click="openRejectDialog(props.row)">
                  <q-tooltip>{{ $t('evolutionSuggestionsPage.actionReject') }}</q-tooltip>
                </q-btn>
              </template>
            </div>
          </q-td>
        </template>
      </AppRegistryTable>

      <AppRegistryPagination
        v-model:page="page"
        v-model:page-size="pageSize"
        :page-max="pageMax"
        :total="total"
        :loading="loading"
        :label="$t('evolutionSuggestionsPage.paginationUnit')"
      />
    </template>

    <EvolutionSuggestionRejectDialog
      :open="rejectDialogOpen"
      :target="rejectTarget"
      :reason="rejectionReason"
      :loading="rejecting"
      @update:open="rejectDialogOpen = $event"
      @update:reason="rejectionReason = $event"
      @confirm="confirmReject"
    />

    <AppDialog
      v-model="curatorDialogOpen"
      persistent
      size="md"
      :title="$t('evolutionSuggestionsPage.curatorDialogTitle')"
    >
      <div class="q-gutter-md">
        <div class="text-body2">
          {{ $t('evolutionSuggestionsPage.curatorDialogDesc') }}
        </div>
        <q-input
          v-model="curatorSkillId"
          dense
          outlined
          label="Skill ID"
          :placeholder="$t('evolutionSuggestionsPage.curatorSkillIdPlaceholder')"
        />
      </div>
      <template #actions>
        <q-btn flat no-caps :label="$t('evolutionSuggestionsPage.cancel')" @click="curatorDialogOpen = false" />
        <q-btn
          color="primary"
          unelevated
          no-caps
          :label="$t('evolutionSuggestionsPage.trigger')"
          :loading="triggeringCurator"
          :disable="!curatorSkillId.trim()"
          @click="confirmTriggerCurator"
        />
      </template>
    </AppDialog>

    <EvolutionSuggestionDetailDialog
      :open="detailDialogOpen"
      :suggestion="detailTarget"
      @update:open="detailDialogOpen = $event"
    />
  </q-page>
</template>

<script setup lang="ts">
import AppDialog from '../components/common/AppDialog.vue';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import AppPageToolbar from '../components/layout/AppPageToolbar.vue';
import AppRegistryTable from '../components/layout/AppRegistryTable.vue';
import AppRegistryPagination from '../components/layout/AppRegistryPagination.vue';
import EvolutionSuggestionDetailDialog from '../components/skills/EvolutionSuggestionDetailDialog.vue';
import EvolutionSuggestionRejectDialog from '../components/skills/EvolutionSuggestionRejectDialog.vue';
import {
  EVOLUTION_SUGGESTION_TABLE_COLUMNS,
  evoSuggestionStatusColor,
  evoSuggestionStatusLabel,
  evoLifecycleStatusColor,
  evoLifecycleStatusLabel,
  evoTargetTypeColor,
  evoTargetTypeLabel,
  evoActionTypeColor,
  evoActionTypeLabel,
} from '../components/skills/evolutionSuggestionTableUi';
import {
  useEvolutionSuggestionListPage,
  statusOptions,
  targetTypeOptions,
} from '../features/skillEvolutionSuggestions/useEvolutionSuggestionListPage';
import { formatDate } from '../features/skillEvolutionSuggestions/formatUtils';

const {
  targetType,
  status,
  targetId,
  page,
  pageSize,
  rows,
  total,
  loading,
  error,
  forbidden,
  pageMax,
  approvingId,
  rejectDialogOpen,
  rejectTarget,
  rejectionReason,
  rejecting,
  curatorSkillId,
  curatorDialogOpen,
  triggeringCurator,
  detailDialogOpen,
  detailTarget,
  openDetailDialog,
  loadRows,
  approveSuggestion,
  openRejectDialog,
  confirmReject,
  handleTriggerCurator,
  confirmTriggerCurator,
  resetFilters,
} = useEvolutionSuggestionListPage();

const columns = EVOLUTION_SUGGESTION_TABLE_COLUMNS;
</script>
