<template>
  <AppRegistryTable
    table-class="skill-table"
    row-key="id"
    :rows="rows"
    :columns="SKILL_TABLE_COLUMNS"
    :loading="loading"
    :pagination="tablePagination"
    :total="total"
    hide-pagination
  >
    <!-- 卡片视图：构件卡。整卡进技能详情；草稿的主操作是「发布」，其余收在「更多」里 -->
    <template #item="card">
      <CapCard
        icon="school"
        :code="card.row.slug"
        :title="card.row.name"
        :description="card.row.description"
        :status="skillStatus(card.row, t)"
        :facts="skillFacts(card.row, t)"
        :tags="skillTags(card.row)"
        @open="router.push({ name: 'skill-detail', params: { skillId: card.row.id } })"
      >
        <template #switch>
          <q-toggle
            dense
            color="primary"
            :model-value="card.row.enabled"
            :disable="
              !card.row.permissions.can_toggle_enabled ||
              card.row.status !== 'published' ||
              togglingId === card.row.id
            "
            :aria-label="card.row.enabled ? t('cap.common.batchDisable') : t('cap.common.batchEnable')"
            @update:model-value="emit('toggle-enabled', card.row, Boolean($event))"
          >
            <q-tooltip v-if="card.row.status !== 'published'">{{ t('cap.skills.publishFirst') }}</q-tooltip>
          </q-toggle>
        </template>
        <template #relation>{{ skillRelation(card.row, t, formatRelativeTime) }}</template>
        <template #actions>
          <q-btn
            v-if="card.row.status === 'draft'"
            flat
            dense
            no-caps
            size="sm"
            color="primary"
            icon="rocket_launch"
            :label="t('cap.skills.publish')"
            :disable="!card.row.permissions.can_edit"
            :loading="publishingId === card.row.id"
            @click="emit('publish', card.row)"
          />
          <q-btn
            v-else
            flat
            dense
            no-caps
            size="sm"
            color="primary"
            icon="folder_open"
            :label="t('cap.skills.editFiles')"
            :disable="!card.row.permissions.can_edit"
            @click="emit('edit-files', card.row)"
          />
          <q-btn flat dense round size="sm" icon="more_horiz" :aria-label="t('cap.common.more')">
            <q-menu anchor="bottom right" self="top right">
              <q-list dense class="cap-menu">
                <q-item
                  v-close-popup
                  clickable
                  :disable="!card.row.permissions.can_edit"
                  @click="emit('edit-meta', card.row)"
                >
                  <q-item-section avatar><q-icon name="edit" size="18px" /></q-item-section>
                  <q-item-section>{{ t('cap.skills.editMeta') }}</q-item-section>
                </q-item>
                <q-item
                  v-if="card.row.status === 'draft'"
                  v-close-popup
                  clickable
                  :disable="!card.row.permissions.can_edit"
                  @click="emit('edit-files', card.row)"
                >
                  <q-item-section avatar><q-icon name="folder_open" size="18px" /></q-item-section>
                  <q-item-section>{{ t('cap.skills.editFiles') }}</q-item-section>
                </q-item>
                <q-item
                  v-close-popup
                  clickable
                  :disable="duplicatingId === card.row.id"
                  @click="emit('duplicate', card.row)"
                >
                  <q-item-section avatar><q-icon name="content_copy" size="18px" /></q-item-section>
                  <q-item-section>{{ t('cap.skills.duplicate') }}</q-item-section>
                </q-item>
                <q-item clickable :to="{ path: '/skills/experience-reports', query: { skill_id: card.row.id } }">
                  <q-item-section avatar><q-icon name="assessment" size="18px" /></q-item-section>
                  <q-item-section>{{ t('cap.skills.reports') }}</q-item-section>
                </q-item>
                <q-item
                  v-if="card.row.status === 'published'"
                  v-close-popup
                  clickable
                  :disable="!card.row.permissions.can_edit || ecosystemPublishBtn(card.row).disabled"
                  @click="emit('publish-ecosystem', card.row)"
                >
                  <q-item-section avatar
                    ><q-icon :name="ecosystemPublishBtn(card.row).icon" size="18px"
                  /></q-item-section>
                  <q-item-section>{{ ecosystemPublishBtn(card.row).tooltip }}</q-item-section>
                </q-item>
                <q-separator />
                <q-item
                  v-close-popup
                  clickable
                  class="text-negative"
                  :disable="!card.row.permissions.can_delete"
                  @click="emit('delete', card.row)"
                >
                  <q-item-section avatar><q-icon name="delete" size="18px" /></q-item-section>
                  <q-item-section>{{ t('cap.common.remove') }}</q-item-section>
                </q-item>
              </q-list>
            </q-menu>
          </q-btn>
        </template>
      </CapCard>
    </template>

    <template #body-cell-name="props">
      <q-td :props="props">
        <AppRegistryHoverTip :text="props.row.description" empty-label="暂无描述">
          <div class="min-width-0">
            <router-link
              :to="{ name: 'skill-detail', params: { skillId: props.row.id } }"
              class="app-registry-cell-primary app-registry-cell-title-link"
              >{{ props.row.name }}</router-link
            >
            <div class="app-registry-cell-sub">{{ props.row.slug }}</div>
          </div>
        </AppRegistryHoverTip>
      </q-td>
    </template>

    <template #body-cell-tags="props">
      <q-td :props="props">
        <div class="app-registry-chip-wrap skill-tags-cell">
          <span
            v-for="group in groupSkillTags(props.row.tags)"
            :key="group.dimension || '__plain'"
            class="skill-tag-group"
          >
            <span v-if="group.dimension" class="skill-tag-group__dim">{{ group.dimension }}</span>
            <span
              v-for="tag in group.tags"
              :key="tag.name"
              class="skill-tag-value"
              :class="{ 'skill-tag-value--system': tag.source === 'system' }"
            >
              {{ tag.label }}
              <q-tooltip v-if="group.dimension">{{ tag.name }}</q-tooltip>
            </span>
          </span>
          <span v-if="!props.row.tags?.length" class="text-caption text-grey-6">{{ t('skillsPage.noTags') }}</span>
        </div>
      </q-td>
    </template>

    <template #body-cell-origin="props">
      <q-td :props="props">
        <q-chip
          v-if="props.row.sync_origin"
          dense
          size="sm"
          :outline="props.row.sync_origin !== 'filesystem'"
          color="primary"
          text-color="white"
        >
          {{ originLabel(props.row.sync_origin) }}
        </q-chip>
        <span v-else class="text-caption text-grey-6">—</span>
      </q-td>
    </template>

    <template #body-cell-disk="props">
      <q-td :props="props">
        <q-chip v-if="props.row.filesystem_missing" dense size="sm" color="negative" text-color="white">
          {{ t('skillsPage.diskMissing') }}
        </q-chip>
        <q-chip v-else dense size="sm" outline color="positive" text-color="positive">
          {{ t('skillsPage.diskOk') }}
        </q-chip>
      </q-td>
    </template>

    <template #body-cell-status="props">
      <q-td :props="props">
        <div class="skill-status-cell">
          <AppStatusChip :status="props.row.status" />
          <span class="skill-status-cell__version">{{ props.row.current_version?.version ?? '无版本' }}</span>
        </div>
      </q-td>
    </template>

    <template #body-cell-enabled="props">
      <q-td :props="props">
        <q-toggle
          dense
          color="primary"
          :model-value="props.row.enabled"
          :disable="
            !props.row.permissions.can_toggle_enabled || props.row.status !== 'published' || togglingId === props.row.id
          "
          @update:model-value="emit('toggle-enabled', props.row, Boolean($event))"
        >
          <q-tooltip v-if="props.row.status !== 'published'">仅已发布的 Skill 可启用</q-tooltip>
        </q-toggle>
      </q-td>
    </template>

    <template #body-cell-stats="props">
      <q-td :props="props">
        <skill-stats-hover-chart :skill="props.row" :load-health="loadHealth" />
      </q-td>
    </template>

    <template #body-cell-last="props">
      <q-td :props="props">
        <template v-if="props.row.last_invoked_at">
          <div class="app-registry-cell-primary">
            {{ props.row.last_agent_display_name || t('skillsPage.unknownAgent') }}
          </div>
          <div class="text-caption text-grey-7">
            {{ formatRelativeTime(props.row.last_invoked_at) }}
            <q-tooltip>{{ formatDate(props.row.last_invoked_at) }}</q-tooltip>
          </div>
        </template>
        <span v-else class="text-caption text-grey-6">{{ t('skillsPage.neverInvoked') }}</span>
      </q-td>
    </template>

    <template #body-cell-actions="props">
      <q-td :props="props">
        <div class="app-registry-cell-actions">
          <q-btn
            v-if="props.row.status !== 'published'"
            flat
            dense
            round
            color="positive"
            icon="play_circle"
            :disable="!props.row.permissions.can_edit || publishingId === props.row.id"
            :loading="publishingId === props.row.id"
            @click="emit('publish', props.row)"
          >
            <q-tooltip>{{ t('skillsPage.enableTooltip') }}</q-tooltip>
          </q-btn>
          <q-btn
            v-else
            flat
            dense
            round
            :color="ecosystemPublishBtn(props.row).color"
            :icon="ecosystemPublishBtn(props.row).icon"
            :disable="
              !props.row.permissions.can_edit ||
              publishingEcosystemId === props.row.id ||
              ecosystemPublishBtn(props.row).disabled
            "
            :loading="publishingEcosystemId === props.row.id"
            @click="emit('publish-ecosystem', props.row)"
          >
            <q-tooltip>{{ ecosystemPublishBtn(props.row).tooltip }}</q-tooltip>
          </q-btn>
          <q-btn
            flat
            dense
            round
            color="primary"
            icon="edit"
            :disable="!props.row.permissions.can_edit"
            @click="emit('edit-meta', props.row)"
          >
            <q-tooltip>{{ t('skillsPage.editMetaTooltip') }}</q-tooltip>
          </q-btn>
          <q-btn
            flat
            dense
            round
            color="primary"
            icon="folder_open"
            :disable="!props.row.permissions.can_edit"
            @click="emit('edit-files', props.row)"
          >
            <q-tooltip>{{ t('skillsPage.editFilesTooltip') }}</q-tooltip>
          </q-btn>
          <q-btn
            flat
            dense
            round
            color="primary"
            icon="content_copy"
            :disable="duplicatingId === props.row.id"
            :loading="duplicatingId === props.row.id"
            @click="emit('duplicate', props.row)"
          >
            <q-tooltip>{{ t('skillsPage.duplicateTooltip') }}</q-tooltip>
          </q-btn>
          <q-btn
            flat
            dense
            round
            color="primary"
            icon="assessment"
            :to="{ path: '/skills/experience-reports', query: { skill_id: props.row.id } }"
          >
            <q-tooltip>{{ t('skillsPage.viewReportsTooltip') }}</q-tooltip>
          </q-btn>
          <q-btn
            flat
            dense
            round
            color="negative"
            icon="delete"
            :disable="!props.row.permissions.can_delete"
            @click="emit('delete', props.row)"
          >
            <q-tooltip>{{ t('common.delete') }}</q-tooltip>
          </q-btn>
        </div>
      </q-td>
    </template>
  </AppRegistryTable>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import { useRouter } from 'vue-router';
import AppRegistryTable from '../layout/AppRegistryTable.vue';
import AppRegistryHoverTip from '../layout/AppRegistryHoverTip.vue';
import CapCard from '../capability/CapCard.vue';
import { skillFacts, skillRelation, skillStatus, skillTags } from './skillCapCard';
import AppStatusChip from '../common/AppStatusChip.vue';
import SkillStatsHoverChart from './SkillStatsHoverChart.vue';
import type { Skill, SkillHealthMetric, SkillTag } from '../../features/skills/types';
import { SKILL_TABLE_COLUMNS, skillOriginLabel as originLabel } from './skillTableUi';

const props = defineProps<{
  rows: Skill[];
  loading: boolean;
  togglingId?: string;
  /** 正在调用启用（生命周期发布）的 skill id，用于按钮 loading */
  publishingId?: string;
  /** 正在发布到生态市场的 skill id，用于按钮 loading */
  publishingEcosystemId?: string;
  /** 正在复制的 skill id，用于按钮 loading */
  duplicatingId?: string;
  /** 生态市场发布状态判定（published/failed/unpublished），由 Page/composable 注入 */
  ecosystemPublishState: (skill: Skill) => 'published' | 'failed' | 'unpublished';
  /** 懒加载单行健康数据（统计悬浮图形面板用，store 方法经 Page 注入） */
  loadHealth: (skillId: string) => Promise<SkillHealthMetric>;
  /** 服务端总条数（视图条上的计数） */
  total?: number;
}>();

const router = useRouter();

const emit = defineEmits<{
  'toggle-enabled': [skill: Skill, enabled: boolean];
  publish: [skill: Skill];
  'publish-ecosystem': [skill: Skill];
  'edit-meta': [skill: Skill];
  'edit-files': [skill: Skill];
  duplicate: [skill: Skill];
  delete: [skill: Skill];
}>();

const { t } = useI18n();

const tablePagination = { rowsPerPage: 0 };

/** 生态市场发布按钮三态：未发布（可点击）/ 已发布（勾选禁用）/ 发布失败（警示可重试）。 */
function ecosystemPublishBtn(skill: Skill): { icon: string; color: string; tooltip: string; disabled: boolean } {
  const state = props.ecosystemPublishState(skill);
  if (state === 'published') {
    return {
      icon: 'cloud_done',
      color: 'positive',
      tooltip: t('skillsPage.ecosystemPublishedTooltip'),
      disabled: true,
    };
  }
  if (state === 'failed') {
    return {
      icon: 'error_outline',
      color: 'negative',
      tooltip: t('skillsPage.ecosystemFailedTooltip'),
      disabled: false,
    };
  }
  return { icon: 'storefront', color: 'secondary', tooltip: t('skillsPage.publishEcosystemTooltip'), disabled: false };
}

type SkillTagView = { name: string; label: string; source: string };
type SkillTagGroup = { dimension: string; tags: SkillTagView[] };

/** 按 `维度:值` 前缀分组标签；无维度的标签归入 dimension='' 组（显示全名）。 */
function groupSkillTags(tags: SkillTag[]): SkillTagGroup[] {
  const grouped = new Map<string, SkillTagView[]>();
  const plain: SkillTagView[] = [];
  for (const t of tags ?? []) {
    const idx = t.name.indexOf(':');
    if (idx > 0) {
      const dim = t.name.slice(0, idx);
      const label = t.name.slice(idx + 1) || t.name;
      if (!grouped.has(dim)) grouped.set(dim, []);
      grouped.get(dim)!.push({ name: t.name, label, source: t.source });
    } else {
      plain.push({ name: t.name, label: t.name, source: t.source });
    }
  }
  const out: SkillTagGroup[] = [...grouped.entries()].map(([dimension, list]) => ({ dimension, tags: list }));
  if (plain.length) out.push({ dimension: '', tags: plain });
  return out;
}

/** 相对时间：<1min 刚刚，<60min N 分钟前，<24h N 小时前，<7d N 天前，否则本地化日期。 */
function formatRelativeTime(value: string) {
  const ts = new Date(value).getTime();
  if (Number.isNaN(ts)) return '-';
  const diff = Date.now() - ts;
  if (diff < 60_000) return t('skillsPage.timeJustNow');
  if (diff < 3_600_000) return t('skillsPage.timeMinutesAgo', { n: Math.floor(diff / 60_000) });
  if (diff < 86_400_000) return t('skillsPage.timeHoursAgo', { n: Math.floor(diff / 3_600_000) });
  if (diff < 7 * 86_400_000) return t('skillsPage.timeDaysAgo', { n: Math.floor(diff / 86_400_000) });
  return new Date(ts).toLocaleDateString();
}

function formatDate(value?: string) {
  if (!value) return '-';
  return new Date(value).toLocaleString();
}
</script>
