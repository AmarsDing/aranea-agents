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

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="loadRows" />
      </template>
    </q-banner>

    <q-card v-if="!loading && rows.length === 0" flat class="app-registry-empty app-empty-state-center">
      <q-card-section class="column items-center text-center q-pa-xl">
        <q-avatar size="72px" color="primary" text-color="white" icon="assessment" />
        <div class="text-h6 q-mt-md">{{ hasActiveFilters ? '没有匹配的经验报告' : '暂无经验报告' }}</div>
        <div class="text-body2 text-grey-7 q-mt-sm">{{ hasActiveFilters ? '请调整筛选条件后重试。' : 'Skill 执行后将自动生成经验报告，可在此查看。' }}</div>
      </q-card-section>
    </q-card>

    <template v-else>
      <div class="row q-col-gutter-md q-mb-md">
        <div class="col-12 col-md-6">
          <FailureTagsChart :failure-tags="failureTagsDistribution" />
        </div>
        <div class="col-12 col-md-6">
          <RootCauseAnalysisCards :cards="rootCauseReports" />
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
import SkillPagination from '../components/skills/SkillPagination.vue';
import ExperienceReportTable from '../components/skills/ExperienceReportTable.vue';
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
  pageMax,
  failureTagsDistribution,
  rootCauseReports,
  loadRows,
  resetFilters,
} = useExperienceReportListPage(initialSkillId);

/** 任一筛选条件生效（Skill ID / 日期范围），用于空态文案区分「无数据」与「无匹配」 */
const hasActiveFilters = computed(() => Boolean(skillId.value || from.value || to.value));
</script>
