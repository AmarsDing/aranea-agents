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
        :observation-count="observations.length"
        :pattern-count="patterns.length"
        :pending-count="pendingProposalsCount"
        :registered-count="registeredKnowledgeCount"
        :running-loop="runningLoop"
        @run-loop="onRunLoop"
      />
    </section>

    <learning-pattern-list
      :patterns="patterns"
      :loading="loading"
      :status-filter="patternStatusFilter"
      @update:status-filter="patternStatusFilter = $event"
    />

    <learning-proposal-list
      :proposals="proposals"
      :loading="loading"
      :approving-id="approvingId"
      :rejecting-id="rejectingId"
      @approve="onApprove"
      @reject="onReject"
    />
  </div>
</template>

<script setup lang="ts">
import { toValue } from 'vue';
import LearningLoopOverview from './LearningLoopOverview.vue';
import LearningPatternList from './LearningPatternList.vue';
import LearningProposalList from './LearningProposalList.vue';
import { useLearningLoopPanel } from '../../features/agents/useLearningLoopPanel';

const props = defineProps<{
  agentId: string | (() => string);
}>();

const agentIdFn = () => toValue(props.agentId);

const {
  loading,
  runningLoop,
  approvingId,
  rejectingId,
  patternStatusFilter,
  observations,
  patterns,
  proposals,
  pendingProposalsCount,
  registeredKnowledgeCount,
  onApprove,
  onReject,
  onRunLoop,
} = useLearningLoopPanel(agentIdFn);
</script>
