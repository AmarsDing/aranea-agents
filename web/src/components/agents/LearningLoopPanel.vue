<template>
  <div class="learning-loop-panel settings-grid settings-grid--wide">
    <section class="settings-section">
      <div class="section-heading">
        <div class="section-heading__main">
          <div class="section-title">
            <span class="section-title__text">学习循环</span>
          </div>
          <p class="settings-section__hint">观察 → 模式识别 → 知识提议 → 审批注册，形成 Agent 自主学习闭环。</p>
        </div>
        <q-btn
          color="primary"
          rounded
          unelevated
          no-caps
          icon="play_arrow"
          label="手动触发"
          :loading="running"
          @click="emit('run')"
        />
      </div>
      <q-inner-loading :showing="loading" label="加载学习数据..." />
      <div v-if="!loading" class="app-metrics-grid q-mt-sm">
        <q-card flat bordered class="overview-metric-card app-metrics-grid__item">
          <q-card-section class="overview-metric-card__body">
            <div class="row items-center q-gutter-sm">
              <q-icon name="psychology" color="primary" size="26px" />
              <div class="overview-metric-card__label">检测模式</div>
            </div>
            <div class="overview-metric-card__value">{{ patternCount }}</div>
            <div class="overview-metric-card__caption">已识别的行为模式</div>
          </q-card-section>
        </q-card>
        <q-card flat bordered class="overview-metric-card app-metrics-grid__item">
          <q-card-section class="overview-metric-card__body">
            <div class="row items-center q-gutter-sm">
              <q-icon name="pending_actions" color="orange" size="26px" />
              <div class="overview-metric-card__label">待审批</div>
            </div>
            <div class="overview-metric-card__value">{{ pendingCount }}</div>
            <div class="overview-metric-card__caption">等待审核的知识提议</div>
          </q-card-section>
        </q-card>
        <q-card flat bordered class="overview-metric-card app-metrics-grid__item">
          <q-card-section class="overview-metric-card__body">
            <div class="row items-center q-gutter-sm">
              <q-icon name="verified" color="positive" size="26px" />
              <div class="overview-metric-card__label">已注册知识</div>
            </div>
            <div class="overview-metric-card__value">{{ knowledgeCount }}</div>
            <div class="overview-metric-card__caption">已批准并注册的知识</div>
          </q-card-section>
        </q-card>
      </div>
    </section>

    <section v-if="patterns.length > 0" class="settings-section">
      <div class="section-heading">
        <div class="section-heading__main">
          <div class="section-title">
            <span class="section-title__text">检测到的模式</span>
          </div>
          <p class="settings-section__hint">从观察中自动识别的重复行为模式。</p>
        </div>
      </div>
      <q-list separator class="app-glass-list">
        <q-item v-for="p in patterns" :key="p.id" class="app-glass-list__item--md">
          <q-item-section>
            <q-item-label class="text-weight-medium">
              <q-badge :color="patternKindColor(p.kind)" class="q-mr-sm" :label="p.kind" />
              {{ p.description }}
            </q-item-label>
            <q-item-label caption class="q-mt-xs">
              频率 {{ p.frequency }} · 置信度 {{ formatConfidence(p.confidence) }} · {{ formatDate(p.detected_at) }}
            </q-item-label>
          </q-item-section>
          <q-item-section side>
            <q-badge :color="patternStatusColor(p.status)" :label="patternStatusLabel(p.status)" />
          </q-item-section>
        </q-item>
      </q-list>
    </section>

    <section v-if="proposals.length > 0" class="settings-section">
      <div class="section-heading">
        <div class="section-heading__main">
          <div class="section-title">
            <span class="section-title__text">知识提议</span>
          </div>
          <p class="settings-section__hint">基于模式生成的知识注册提议，可批准或拒绝。</p>
        </div>
      </div>
      <q-list separator class="app-glass-list">
        <q-item v-for="p in proposals" :key="p.id" class="app-glass-list__item--lg">
          <q-item-section>
            <q-item-label class="text-weight-medium">
              <q-badge :color="proposalKindColor(p.kind)" class="q-mr-sm" :label="p.kind" />
              {{ p.title }}
            </q-item-label>
            <q-item-label caption class="q-mt-xs">{{ p.content }}</q-item-label>
            <q-item-label caption class="q-mt-xs text-grey-5">
              模式 {{ p.pattern_id }} · {{ formatDate(p.created_at) }}
            </q-item-label>
          </q-item-section>
          <q-item-section side>
            <div v-if="p.status === 'pending'" class="row q-gutter-xs">
              <q-btn
                flat
                round
                dense
                icon="check"
                color="positive"
                size="sm"
                :loading="actingId === p.id"
                @click="confirmApprove(p.id)"
              >
                <q-tooltip>批准</q-tooltip>
              </q-btn>
              <q-btn
                flat
                round
                dense
                icon="close"
                color="negative"
                size="sm"
                :loading="actingId === p.id"
                @click="emit('reject', p.id)"
              >
                <q-tooltip>拒绝</q-tooltip>
              </q-btn>
            </div>
            <q-badge
              v-else
              :color="p.status === 'approved' ? 'positive' : p.status === 'rejected' ? 'grey' : 'warning'"
              :label="proposalStatusLabel(p.status)"
            />
          </q-item-section>
        </q-item>
      </q-list>
    </section>

    <section v-if="observations.length > 0" class="settings-section">
      <div class="section-heading">
        <div class="section-heading__main">
          <div class="section-title">
            <span class="section-title__text">最近观察</span>
          </div>
          <p class="settings-section__hint">从会话中采集的原始观察记录。</p>
        </div>
      </div>
      <q-list separator class="app-glass-list">
        <q-item v-for="o in observations.slice(0, 10)" :key="o.id" class="app-glass-list__item--md">
          <q-item-section>
            <q-item-label class="text-weight-medium">
              <q-badge outline color="grey-7" class="q-mr-sm" :label="o.kind" />
              {{ truncateContent(o.content) }}
            </q-item-label>
            <q-item-label caption class="q-mt-xs">
              会话 {{ o.session_id }} · {{ formatDate(o.observed_at) }}
            </q-item-label>
          </q-item-section>
        </q-item>
      </q-list>
    </section>

    <q-banner
      v-if="!loading && patterns.length === 0 && proposals.length === 0 && observations.length === 0"
      rounded
      class="settings-info-banner settings-info-banner--bordered q-mt-md"
    >
      暂无学习数据。点击「手动触发」启动学习循环，或等待 Agent 在对话中自动积累观察。
    </q-banner>
  </div>
</template>

<script setup lang="ts">
import { useQuasar } from 'quasar';
import type {
  LearningObservation,
  LearningPattern,
  LearningProposal as KnowledgeProposal,
} from '../../features/agents/learning.types';

const $q = useQuasar();

const props = defineProps<{
  observations: LearningObservation[];
  patterns: LearningPattern[];
  proposals: KnowledgeProposal[];
  loading: boolean;
  running: boolean;
  actingId: string | null;
  patternCount: number;
  pendingCount: number;
  knowledgeCount: number;
}>();

const emit = defineEmits<{
  approve: [id: string];
  reject: [id: string];
  run: [];
}>();

function confirmApprove(proposalId: string) {
  $q.dialog({
    title: '批准知识提议',
    message: '确定批准此知识提议？批准后将注册为 Agent 的持久知识。',
    cancel: true,
    persistent: true,
  }).onOk(() => {
    emit('approve', proposalId);
  });
}

function formatDate(iso: string): string {
  if (!iso) return '';
  try {
    return new Date(iso).toLocaleDateString();
  } catch {
    return iso;
  }
}

function formatConfidence(v: number): string {
  if (v === undefined || v === null) return '—';
  return (v * 100).toFixed(1) + '%';
}

function truncateContent(text: string, max = 80): string {
  if (!text) return '';
  return text.length > max ? text.slice(0, max) + '…' : text;
}

function patternKindColor(kind: string): string {
  switch (kind) {
    case 'behavior':
      return 'blue';
    case 'preference':
      return 'teal';
    case 'error':
      return 'red';
    case 'efficiency':
      return 'orange';
    default:
      return 'grey';
  }
}

function patternStatusColor(status: string): string {
  switch (status) {
    case 'active':
      return 'positive';
    case 'superseded':
      return 'grey';
    case 'invalidated':
      return 'negative';
    default:
      return 'grey';
  }
}

function patternStatusLabel(status: string): string {
  switch (status) {
    case 'active':
      return '活跃';
    case 'superseded':
      return '已替代';
    case 'invalidated':
      return '已失效';
    default:
      return status;
  }
}

function proposalKindColor(kind: string): string {
  switch (kind) {
    case 'fact':
      return 'primary';
    case 'rule':
      return 'deep-purple';
    case 'preference':
      return 'teal';
    case 'skill':
      return 'orange';
    default:
      return 'grey';
  }
}

function proposalStatusLabel(status: string): string {
  switch (status) {
    case 'approved':
      return '已批准';
    case 'rejected':
      return '已拒绝';
    case 'pending':
      return '待审批';
    default:
      return status;
  }
}
</script>
