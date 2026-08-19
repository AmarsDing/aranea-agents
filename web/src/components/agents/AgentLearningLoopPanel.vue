<template>
  <div class="evolution-panel settings-grid settings-grid--wide">
    <section class="settings-section">
      <div class="section-heading">
        <div class="section-heading__main">
          <div class="section-title">
            <span class="section-title__text">学习闭环</span>
          </div>
          <p class="settings-section__hint">观察 → 模式识别 → 知识提议 → 审批注册，形成持续学习闭环。</p>
        </div>
      </div>
      <q-inner-loading :showing="loading" label="加载学习数据..." />
      <learning-loop-overview
        v-if="!loading"
        :observation-count="overview.observationCount"
        :pattern-count="overview.patternCount"
        :pending-count="overview.pendingCount"
        :registered-count="overview.registeredCount"
        :running-loop="runningLoop"
        @run-loop="onRunLoop"
      />
    </section>

    <learning-pattern-list
      :patterns="filteredPatterns"
      :loading="loading"
      :status-filter="patternStatus"
      @update:status-filter="patternStatus = $event"
      @confirm="onConfirmPattern"
      @dismiss="onDismissPattern"
    />

    <learning-proposal-list
      :proposals="filteredProposals"
      :loading="loading"
      :status-filter="proposalStatus"
      @update:status-filter="proposalStatus = $event"
      @approve="onApprove"
      @reject="onReject"
      @apply="onApply"
    />

    <learning-observation-list :observations="observations" :loading="loading" />
  </div>
</template>

<script setup lang="ts">
import { toValue } from 'vue';
import LearningLoopOverview from './LearningLoopOverview.vue';
import LearningPatternList from './LearningPatternList.vue';
import LearningProposalList from './LearningProposalList.vue';
import LearningObservationList from './LearningObservationList.vue';
import { useLearningLoopPanel } from '../../features/agents/useLearningLoopPanel';

const props = defineProps<{
  agentId: string | (() => string);
}>();

const agentIdFn = () => toValue(props.agentId);

const {
  loading,
  runningLoop,
  patternStatus,
  proposalStatus,
  filteredPatterns,
  filteredProposals,
  observations,
  overview,
  onRunLoop,
  onConfirmPattern,
  onDismissPattern,
  onApprove,
  onReject,
  onApply,
} = useLearningLoopPanel(agentIdFn);
</script>
