<template>
  <q-page class="app-standard-page skill-detail-page">
    <div v-if="loadingSkill" class="column items-center justify-center" style="min-height: 300px">
      <q-spinner color="primary" size="40px" />
      <div class="q-mt-md text-grey-7">加载中...</div>
    </div>

    <template v-else-if="skillError">
      <q-banner class="bg-negative text-white rounded-borders q-mb-md">
        <template #avatar><q-icon name="error" /></template>
        {{ skillError }}
        <template #action>
          <q-btn flat label="返回列表" @click="goBack" />
        </template>
      </q-banner>
    </template>

    <template v-else-if="skill">
      <div class="row items-center q-mb-md">
        <q-btn flat round icon="arrow_back" class="q-mr-sm" @click="goBack" />
        <div class="col">
          <div class="row items-center q-gutter-sm">
            <div class="text-h5" style="color: var(--color-text-primary)">{{ skill.name }}</div>
            <q-badge rounded :color="statusColor(skill.status)">{{ statusLabel(skill.status) }}</q-badge>
            <q-badge v-if="skill.enabled" rounded color="positive">已启用</q-badge>
            <q-badge v-else rounded color="grey">未启用</q-badge>
          </div>
          <div class="text-caption text-grey-7 q-mt-xs">
            {{ skill.slug }} · 创建 {{ formatDate(skill.created_at) }} · 更新 {{ formatDate(skill.updated_at) }}
          </div>
        </div>
      </div>

      <div class="row q-col-gutter-md">
        <div class="col-12 col-md-6">
          <SkillHealthCard :health="health" :loading="loadingHealth" :error="healthError" @refresh="loadHealth" />
        </div>

        <div class="col-12 col-md-6">
          <q-card flat bordered>
            <q-card-section>
              <div class="text-subtitle2 q-mb-sm">基本信息</div>
              <div v-if="skill.description" class="text-body2 q-mb-md">{{ skill.description }}</div>
              <div class="row q-col-gutter-sm">
                <div class="col-6">
                  <div class="text-caption text-grey-7">调用次数</div>
                  <div class="text-body1">{{ skill.invoke_count }}</div>
                </div>
                <div class="col-6">
                  <div class="text-caption text-grey-7">7d 使用</div>
                  <div class="text-body1">{{ skill.usage_count_7d }}</div>
                </div>
                <div class="col-6">
                  <div class="text-caption text-grey-7">成功</div>
                  <div class="text-body1 text-positive">{{ skill.success_count }}</div>
                </div>
                <div class="col-6">
                  <div class="text-caption text-grey-7">失败</div>
                  <div class="text-body1 text-negative">{{ skill.failure_count }}</div>
                </div>
                <div class="col-6">
                  <div class="text-caption text-grey-7">平均耗时</div>
                  <div class="text-body1">{{ formatDuration(skill.avg_duration_ms) }}</div>
                </div>
                <div class="col-6">
                  <div class="text-caption text-grey-7">最近调用</div>
                  <div class="text-body1">{{ formatDate(skill.last_invoked_at) }}</div>
                </div>
              </div>
              <div v-if="skill.tags?.length" class="q-mt-md">
                <div class="text-caption text-grey-7 q-mb-xs">标签</div>
                <q-chip
                  v-for="tag in skill.tags"
                  :key="tag.name"
                  dense
                  :outline="tag.source === 'system'"
                  color="primary"
                  text-color="white"
                >
                  {{ tag.name }}
                </q-chip>
              </div>
            </q-card-section>
          </q-card>
        </div>
      </div>
    </template>
  </q-page>
</template>

<script setup lang="ts">
import { useRoute } from 'vue-router';
import SkillHealthCard from '../components/skills/SkillHealthCard.vue';
import { useSkillDetailPage } from '../features/skills/useSkillDetailPage';
import { skillStatusLabel as statusLabel, skillStatusColor as statusColor } from '../components/skills/skillTableUi';

const route = useRoute();
const skillId = route.params.skillId as string;

const { skill, health, loadingSkill, loadingHealth, skillError, healthError, loadHealth, goBack } =
  useSkillDetailPage(skillId);

function formatDate(value?: string) {
  if (!value) return '-';
  return new Date(value).toLocaleString();
}

function formatDuration(value?: number | null) {
  if (value === undefined || value === null) return '-';
  if (value < 1000) return `${Math.round(value)}ms`;
  return `${(value / 1000).toFixed(1)}s`;
}
</script>
