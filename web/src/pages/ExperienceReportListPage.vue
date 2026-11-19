<template>
  <q-page class="app-standard-page app-registry-page experience-report-list-page">
    <AppPageHero
      kicker="Skill intelligence"
      title="经验报告"
      subtitle="查看 Skill 执行后的经验报告，包括成功/失败、评分、失败标签与流程摘要。"
    >
      <template #actions>
        <q-btn outline rounded no-caps color="primary" icon="arrow_back" label="返回 Skill 管理" to="/skills" />
        <q-btn
          color="primary"
          unelevated
          rounded
          no-caps
          icon="refresh"
          label="刷新"
          :loading="loading"
          @click="loadRows"
        />
      </template>
    </AppPageHero>

    <AppPageToolbar>
      <q-input
        v-model="skillId"
        class="app-page-toolbar__field"
        dense
        outlined
        clearable
        debounce="350"
        label="Skill ID"
      />
      <q-input v-model="from" class="app-page-toolbar__field" dense outlined clearable type="date" label="开始日期" />
      <q-input v-model="to" class="app-page-toolbar__field" dense outlined clearable type="date" label="结束日期" />
      <template #actions>
        <q-btn flat rounded no-caps icon="restart_alt" label="重置" @click="resetFilters" />
        <q-btn flat rounded no-caps icon="refresh" label="刷新" :loading="loading" @click="loadRows" />
      </template>
    </AppPageToolbar>

    <!-- 五态：已有数据但刷新失败 → 局部失败横幅，列表照常渲染 -->
    <AppStatePanel
      v-if="(error || forbidden) && rows.length > 0"
      variant="partial"
      dense
      class="q-mb-md"
      :description="error"
      action-label="重试"
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
      action-label="重试"
      action-icon="refresh"
      @action="loadRows"
    />

    <!-- KPI 概览条：始终展示（筛选条件下为 0 亦是有效信息），数据来自后端全量聚合而非当前页 -->
    <ExperienceReportStatCards
      :total="total"
      :success-count="stats.successCount"
      :failure-count="stats.failureCount"
      :avg-score="stats.avgScore"
      :rca-count="rootCauseReports.length"
    />

    <!-- 五态：首屏加载无数据 → 骨架面板 -->
    <AppStatePanel v-if="loading && rows.length === 0 && !error && !forbidden" variant="loading" :rows="5" />

    <!-- 五态：成功但无数据 → 空态 -->
    <AppStatePanel
      v-else-if="!loading && !error && !forbidden && rows.length === 0"
      variant="empty"
      :title="hasActiveFilters ? '没有匹配的经验报告' : '暂无经验报告'"
      :description="
        hasActiveFilters ? '请调整筛选条件后重试。' : 'Skill 执行后将自动生成经验报告，可在此查看。'
      "
      icon="assessment"
    />

    <template v-else-if="rows.length > 0">
      <div v-if="hasAnalytics" class="row q-col-gutter-md q-mb-md">
        <div class="col-12 col-md-6">
          <FailureTagsChart class="full-height" :failure-tags="failureTagsDistribution" />
        </div>
        <div class="col-12 col-md-6">
          <RootCauseAnalysisCards class="full-height" :cards="rootCauseReports" />
        </div>
      </div>

      <experience-report-table :rows="rows" :loading="loading" />

      <skill-pagination
        v-model:page="page"
        v-model:page-size="pageSize"
        :page-max="pageMax"
        :total="total"
        :loading="loading"
        label="条报告"
      />
    </template>
  </q-page>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useRoute } from 'vue-router';
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppPageToolbar from '../components/layout/AppPageToolbar.vue';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import SkillPagination from '../components/skills/SkillPagination.vue';
import ExperienceReportTable from '../components/skills/ExperienceReportTable.vue';
import ExperienceReportStatCards from '../components/skills/ExperienceReportStatCards.vue';
import FailureTagsChart from '../components/skills/FailureTagsChart.vue';
import RootCauseAnalysisCards from '../components/skills/RootCauseAnalysisCards.vue';
import { useExperienceReportListPage } from '../features/skills/useExperienceReportListPage';

const route = useRoute();
const initialSkillId = (route.query.skill_id as string) || '';

const {
  skillId,
  from,
  to,
  page,
  pageSize,
  rows,
  total,
  loading,
  error,
  forbidden,
  pageMax,
  failureTagsDistribution,
  rootCauseReports,
  stats,
  loadRows,
  resetFilters,
} = useExperienceReportListPage(initialSkillId);

/** 任一筛选条件生效（Skill ID / 日期范围），用于空态文案区分「无数据」与「无匹配」 */
const hasActiveFilters = computed(() => Boolean(skillId.value || from.value || to.value));

/** 有失败标签或根因分析数据时才展示统计卡片，避免全成功场景下两个「暂无数据」卡片占空间 */
const hasAnalytics = computed(
  () => Object.keys(failureTagsDistribution.value).length > 0 || rootCauseReports.value.length > 0,
);
</script>
