<template>
  <q-page :class="['team-run-observatory', { 'is-dark': isDark }]">
    <div class="team-run-observatory__toolbar">
      <q-btn flat dense round icon="arrow_back" @click="goBack" />
      <div>
        <div class="team-run-observatory__title">Team 运行观测</div>
        <div v-if="observatory" class="text-caption text-grey-7">
          {{ observatory.mode }} · {{ observatory.status }} · {{ runId }}
          <q-btn
            v-if="observatory.trace_id"
            flat
            dense
            no-caps
            size="sm"
            color="primary"
            icon="timeline"
            :label="`trace ${observatory.trace_id.slice(0, 8)}…`"
            class="q-ml-xs"
            @click="openTrace"
          />
        </div>
      </div>
      <q-space />
      <q-badge v-if="observatory" rounded :color="runStatusColor">{{ observatory.status }}</q-badge>
    </div>

    <div v-if="loading" class="flex flex-center q-pa-xl">
      <q-spinner color="primary" size="40px" />
    </div>
    <div v-else-if="error" class="q-pa-md text-negative">{{ error }}</div>
    <div v-else class="team-run-observatory__body">
      <OrchestrationFailureBanner
        :nodes="nodeList"
        :run-status="observatory?.status"
        @review="onFailureReview"
        @retry="onFailureRetry"
        @fallback="onFailureFallback"
        @halt="onFailureHalt"
      />
      <section class="team-run-observatory__graph">
        <GraphEditorCanvas
          :graph-def="graphDef"
          :is-dark="isDark"
          :exec-node-states="execNodeStates"
          :selected-node-id="selectedNodeId"
          :focus-selected-node="true"
          :read-only="true"
          @select-node="onSelectNode"
          @update-graph="() => {}"
        />
      </section>
      <section class="team-run-observatory__kanban">
        <q-tabs v-model="observatoryTab" dense align="left" class="q-mb-md">
          <q-tab name="agents" label="Agent 工作看板" />
          <q-tab name="timeline" label="Timeline (RPC)" />
          <q-tab name="summary" label="Summary" />
          <q-tab name="hitl" label="HITL" :disable="!waitingReviewNodes.length">
            <q-badge v-if="waitingReviewNodes.length" floating color="warning">{{ waitingReviewNodes.length }}</q-badge>
          </q-tab>
          <q-tab name="tasks" label="任务看板" :disable="!graphExecutionId" />
        </q-tabs>
        <q-tab-panels v-model="observatoryTab" animated>
          <q-tab-panel name="agents" class="q-pa-none">
            <OrchestrationKanban
              :nodes="nodeList"
              :is-dark="isDark"
              :live-connected="streamConnected"
              :selected-node-id="selectedNodeId"
              @select-node="onKanbanSelectNode"
            />
          </q-tab-panel>
          <q-tab-panel name="timeline" class="q-pa-none">
            <OrchestrationActivityTimeline
              :rows="filteredTimelineRows"
              :loading="timelineLoading"
              :node-filter="timelineNodeFilter"
              :node-filter-options="timelineNodeFilterOptions"
              @update:node-filter="timelineNodeFilter = $event"
              @refresh="loadTimeline"
              @select-node="onKanbanSelectNode"
            />
          </q-tab-panel>
          <q-tab-panel name="summary" class="q-pa-none">
            <div v-if="summaryLoading" class="flex flex-center q-pa-md">
              <q-spinner color="primary" size="28px" />
            </div>
            <q-list v-else-if="runSummary" dense bordered separator class="rounded-borders">
              <q-item>
                <q-item-section>
                  <q-item-label caption>状态</q-item-label>
                  <q-item-label>{{ runSummary.status }}</q-item-label>
                </q-item-section>
                <q-item-section side>
                  <q-item-label caption>{{ runSummary.duration_ms }}ms</q-item-label>
                </q-item-section>
              </q-item>
              <q-item v-if="runSummary.output_preview">
                <q-item-section>
                  <q-item-label caption>输出预览</q-item-label>
                  <q-item-label class="text-body2">{{ runSummary.output_preview }}</q-item-label>
                </q-item-section>
              </q-item>
              <q-item v-if="runSummary.error_message">
                <q-item-section>
                  <q-item-label caption>错误</q-item-label>
                  <q-item-label class="text-negative">{{ runSummary.error_message }}</q-item-label>
                </q-item-section>
              </q-item>
              <q-item>
                <q-item-section>
                  <q-item-label caption>成员 / 工具</q-item-label>
                  <q-item-label>{{ runSummary.member_count }} 成员 · {{ runSummary.tool_call_count }} 工具调用</q-item-label>
                </q-item-section>
              </q-item>
            </q-list>
            <div v-else class="text-caption text-grey-7 q-pa-sm">暂无 Run Summary</div>
          </q-tab-panel>
          <q-tab-panel name="hitl" class="q-pa-none">
            <div v-if="!waitingReviewNodes.length" class="text-caption text-grey-7 q-pa-sm">
              当前无等待审核节点。
            </div>
            <q-list v-else dense bordered separator class="rounded-borders">
              <q-item v-for="node in waitingReviewNodes" :key="node.node_id" clickable @click="openHitlForNode(node.node_id)">
                <q-item-section avatar>
                  <q-icon name="hourglass_empty" color="warning" />
                </q-item-section>
                <q-item-section>
                  <q-item-label>{{ node.agent_name || node.node_id }}</q-item-label>
                  <q-item-label caption>{{ node.error_message || node.output_preview || "等待审核" }}</q-item-label>
                </q-item-section>
                <q-item-section side>
                  <q-btn flat dense color="primary" label="审核" @click.stop="openHitlForNode(node.node_id)" />
                </q-item-section>
              </q-item>
            </q-list>
          </q-tab-panel>
          <q-tab-panel name="tasks" class="q-pa-none">
            <GraphTaskKanban
              v-if="graphExecutionId"
              :tasks="taskList"
              :loading="tasksLoading"
              :live-connected="taskStreamConnected"
              :selected-task-id="selectedTaskId"
              :is-dark="isDark"
              admin-drag
              @refresh="loadObservatoryTasks"
              @select-task="onSelectTask"
              @admin-action="onKanbanAdminAction"
            />
            <div v-else class="text-caption text-grey-7 q-pa-sm">
              当前 Team Run 未绑定 Graph 执行，任务看板不可用。
            </div>
          </q-tab-panel>
        </q-tab-panels>
      </section>
    </div>

    <OrchestrationHitlReviewDialog
      v-model:open="hitlDialogOpen"
      v-model:advanced-json="hitlAdvancedJson"
      :node="hitlReviewNode"
      :loading="resumeLoading"
      @approve="onHitlApprove"
      @reject="onHitlReject($event)"
      @fallback="onHitlFallback"
    />
  </q-page>
</template>

<script setup lang="ts">
import { useRouter } from "vue-router";
import GraphEditorCanvas from "../components/graph/GraphEditorCanvas.vue";
import GraphTaskKanban from "../components/graph/GraphTaskKanban.vue";
import OrchestrationActivityTimeline from "../components/orchestration/OrchestrationActivityTimeline.vue";
import OrchestrationFailureBanner from "../components/orchestration/OrchestrationFailureBanner.vue";
import OrchestrationHitlReviewDialog from "../components/orchestration/OrchestrationHitlReviewDialog.vue";
import OrchestrationKanban from "../components/orchestration/OrchestrationKanban.vue";
import { useTeamRunObservatoryPage } from "../features/teams/useTeamRunObservatoryPage";

const {
  isDark,
  runId,
  loading,
  error,
  observatory,
  observatoryTab,
  selectedNodeId,
  selectedTaskId,
  graphDef,
  streamConnected,
  taskStreamConnected,
  nodeList,
  taskList,
  tasksLoading,
  graphExecutionId,
  execNodeStates,
  runStatusColor,
  timelineLoading,
  timelineNodeFilter,
  timelineNodeFilterOptions,
  filteredTimelineRows,
  runSummary,
  summaryLoading,
  waitingReviewNodes,
  hitlDialogOpen,
  hitlReviewNode,
  hitlAdvancedJson,
  resumeLoading,
  loadTimeline,
  onSelectNode,
  onSelectTask,
  onKanbanAdminAction,
  loadObservatoryTasks,
  goBack,
  onFailureReview,
  onFailureRetry,
  onFailureFallback,
  onFailureHalt,
  onHitlApprove,
  onHitlReject,
  onHitlFallback,
} = useTeamRunObservatoryPage();

const router = useRouter();

function openTrace() {
  if (!observatory.value?.trace_id) return;
  router.push({ name: "monitor-logs", query: { trace_id: observatory.value.trace_id } });
}

function onKanbanSelectNode(nodeId: string) {
  onSelectNode(nodeId);
}

function openHitlForNode(nodeId: string) {
  onFailureReview(nodeId);
}
</script>
