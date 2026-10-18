<template>
  <q-page class="app-standard-page app-registry-page evolution-suggestion-page">
    <AppPageHero
      kicker="Skill intelligence"
      :title="$t('evolutionSuggestionsPage.pageTitle')"
      :subtitle="$t('evolutionSuggestionsPage.pageSubtitle')"
    >
      <template #actions>
        <q-btn
          color="primary"
          unelevated
          rounded
          no-caps
          icon="auto_fix_high"
          :label="$t('evolutionSuggestionsPage.triggerCurator')"
          :loading="triggeringCurator"
          @click="handleTriggerCurator(targetId)"
        />
      </template>
    </AppPageHero>

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

    <q-banner v-if="error" rounded class="app-page-error-banner q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat dense :label="$t('evolutionSuggestionsPage.retry')" class="text-white" @click="loadRows" />
      </template>
    </q-banner>

    <q-card v-if="!loading && rows.length === 0" flat class="app-registry-empty app-empty-state-center">
      <q-card-section class="column items-center text-center q-pa-xl">
        <q-avatar size="72px" color="primary" text-color="white" icon="auto_fix_high" />
        <div class="text-h6 q-mt-md">
          {{
            targetId || status
              ? $t('evolutionSuggestionsPage.emptyFiltered')
              : $t('evolutionSuggestionsPage.emptyDefault')
          }}
        </div>
        <div class="text-body2 text-grey-7 q-mt-sm">
          {{
            targetId || status
              ? $t('evolutionSuggestionsPage.emptyFilteredHint')
              : $t('evolutionSuggestionsPage.emptyDefaultHint')
          }}
        </div>
      </q-card-section>
    </q-card>

    <template v-else>
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

    <q-dialog v-model="curatorDialogOpen" persistent>
      <q-card class="app-dialog-card app-glass-dialog">
        <q-card-section class="app-glass-dialog__head row items-center justify-between">
          <div class="app-glass-dialog__title">{{ $t('evolutionSuggestionsPage.curatorDialogTitle') }}</div>
          <q-btn v-close-popup flat round dense icon="close" />
        </q-card-section>
        <q-separator />
        <div class="app-glass-dialog__scroll">
          <q-card-section class="app-dialog-body app-glass-dialog__body q-gutter-md">
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
          </q-card-section>
        </div>
        <q-separator />
        <q-card-actions align="right" class="app-actions-bar app-glass-dialog__actions">
          <q-btn v-close-popup flat no-caps :label="$t('evolutionSuggestionsPage.cancel')" />
          <q-btn
            color="primary"
            unelevated
            no-caps
            :label="$t('evolutionSuggestionsPage.trigger')"
            :loading="triggeringCurator"
            :disable="!curatorSkillId.trim()"
            @click="confirmTriggerCurator"
          />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <EvolutionSuggestionDetailDialog
      :open="detailDialogOpen"
      :suggestion="detailTarget"
      @update:open="detailDialogOpen = $event"
    />
  </q-page>
</template>

<script setup lang="ts">
import AppPageHero from '../components/layout/AppPageHero.vue';
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
