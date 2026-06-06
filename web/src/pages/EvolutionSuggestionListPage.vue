<template>
  <q-page class="app-standard-page app-registry-page evolution-suggestion-page">
    <AppPageHero
      kicker="Skill intelligence"
      title="Skill 进化建议"
      subtitle="审批或拒绝由技能管家生成的 Skill 进化建议，查看沙箱验证结果与触发原因。"
    >
      <template #actions>
        <q-btn
          color="primary"
          unelevated
          rounded
          no-caps
          icon="auto_fix_high"
          label="触发 Curator"
          :loading="triggeringCurator"
          @click="handleTriggerCurator"
        />
        <q-btn unelevated rounded no-caps icon="refresh" label="刷新" :loading="loading" @click="loadRows" />
      </template>
    </AppPageHero>

    <AppPageToolbar>
      <q-input
        v-model="skillId"
        class="app-page-toolbar__search"
        dense
        outlined
        clearable
        debounce="300"
        label="Skill ID"
      >
        <template #prepend><q-icon name="search" /></template>
      </q-input>
      <q-select
        v-model="status"
        class="app-page-toolbar__field"
        dense
        outlined
        clearable
        emit-value
        map-options
        label="状态"
        :options="statusOptions"
      />
      <template #actions>
        <q-btn flat rounded no-caps icon="restart_alt" label="重置" @click="resetFilters" />
        <q-btn flat rounded no-caps icon="refresh" label="刷新" :loading="loading" @click="loadRows" />
      </template>
    </AppPageToolbar>

    <q-banner v-if="error" rounded class="app-page-error-banner q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat dense label="重试" class="text-white" @click="loadRows" />
      </template>
    </q-banner>

    <q-card v-if="!loading && rows.length === 0" flat class="app-registry-empty app-empty-state-center">
      <q-card-section class="column items-center text-center q-pa-xl">
        <q-avatar size="72px" color="primary" text-color="white" icon="auto_fix_high" />
        <div class="text-h6 q-mt-md">{{ skillId || status ? '没有匹配的进化建议' : '暂无进化建议' }}</div>
        <div class="text-body2 text-grey-7 q-mt-sm">技能管家将基于使用数据自动生成进化建议，待审批后生效。</div>
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
        <template #body-cell-skillId="props">
          <q-td :props="props">
            <span
              class="app-registry-cell-primary ellipsis cursor-pointer"
              :title="props.row.skillId ?? ''"
              @click="openDetailDialog(props.row)"
            >
              {{ props.row.skillId || '—' }}
            </span>
          </q-td>
        </template>

        <template #body-cell-type="props">
          <q-td :props="props">
            <q-chip dense square :color="evoSuggestionTypeColor(props.row.type)" text-color="white">
              {{ evoSuggestionTypeLabel(props.row.type) }}
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
            <span
              class="app-registry-cell-sub ellipsis"
              :title="props.row.triggerReason ?? ''"
              style="max-width: 240px"
            >
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
                <q-tooltip>查看详情</q-tooltip>
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
                  <q-tooltip>批准</q-tooltip>
                </q-btn>
                <q-btn flat dense round icon="close" color="negative" @click="openRejectDialog(props.row)">
                  <q-tooltip>拒绝</q-tooltip>
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
        label="条进化建议"
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
          <div class="app-glass-dialog__title">触发 Curator 流水线</div>
          <q-btn v-close-popup flat round dense icon="close" />
        </q-card-section>
        <q-separator />
        <div class="app-glass-dialog__scroll">
          <q-card-section class="app-dialog-body app-glass-dialog__body q-gutter-md">
            <div class="text-body2">
              为指定 Skill 运行完整的 Curator Agent 进化流水线：触发检测 → 草稿生成 → 沙箱验证。
            </div>
            <q-input v-model="curatorSkillId" dense outlined label="Skill ID" placeholder="输入要触发进化的 Skill ID" />
          </q-card-section>
        </div>
        <q-separator />
        <q-card-actions align="right" class="app-actions-bar app-glass-dialog__actions">
          <q-btn v-close-popup flat no-caps label="取消" />
          <q-btn
            color="primary"
            unelevated
            no-caps
            label="触发"
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
import { ref } from 'vue';
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppPageToolbar from '../components/layout/AppPageToolbar.vue';
import AppRegistryTable from '../components/layout/AppRegistryTable.vue';
import AppRegistryPagination from '../components/layout/AppRegistryPagination.vue';
import EvolutionSuggestionDetailDialog from '../components/skills/EvolutionSuggestionDetailDialog.vue';
import EvolutionSuggestionRejectDialog from '../components/skills/EvolutionSuggestionRejectDialog.vue';
import {
  EVOLUTION_SUGGESTION_TABLE_COLUMNS,
  evoSuggestionTypeColor,
  evoSuggestionTypeLabel,
  evoSuggestionStatusColor,
  evoSuggestionStatusLabel,
  evoLifecycleStatusColor,
  evoLifecycleStatusLabel,
} from '../components/skills/evolutionSuggestionTableUi';
import {
  useEvolutionSuggestionListPage,
  statusOptions,
} from '../features/skillEvolutionSuggestions/useEvolutionSuggestionListPage';
import type { EvolutionSuggestionView } from '../features/skills/types';

const {
  skillId,
  status,
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
  loadRows,
  approveSuggestion,
  openRejectDialog,
  confirmReject,
  resetFilters,
  triggerCuratorFlow,
} = useEvolutionSuggestionListPage();

const columns = EVOLUTION_SUGGESTION_TABLE_COLUMNS;

const detailDialogOpen = ref(false);
const detailTarget = ref<EvolutionSuggestionView | null>(null);
const curatorDialogOpen = ref(false);
const curatorSkillId = ref('');
const triggeringCurator = ref(false);

function openDetailDialog(row: EvolutionSuggestionView) {
  detailTarget.value = row;
  detailDialogOpen.value = true;
}

function handleTriggerCurator() {
  curatorSkillId.value = skillId.value || '';
  curatorDialogOpen.value = true;
}

async function confirmTriggerCurator() {
  const id = curatorSkillId.value.trim();
  if (!id) return;
  triggeringCurator.value = true;
  try {
    await triggerCuratorFlow(id);
    curatorDialogOpen.value = false;
    curatorSkillId.value = '';
    void loadRows();
  } catch {
    // error handled in composable
  } finally {
    triggeringCurator.value = false;
  }
}

function formatDate(ts: unknown): string {
  if (!ts || typeof ts !== 'string') return '—';
  try {
    return new Date(ts).toLocaleString('zh-CN', { hour12: false });
  } catch {
    return ts;
  }
}
</script>
