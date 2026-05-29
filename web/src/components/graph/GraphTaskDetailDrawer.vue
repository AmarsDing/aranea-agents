<template>
  <q-dialog :model-value="open" persistent @update:model-value="onDialogUpdate">
    <q-card class="graph-task-drawer app-dialog-card app-glass-dialog" :style="{ minWidth: '480px', maxWidth: '600px' }">
      <q-card-section class="app-glass-dialog__head row items-start justify-between no-wrap">
        <div class="col min-width-0">
          <div class="app-glass-dialog__title">任务详情</div>
          <div v-if="task" class="app-glass-dialog__subtitle">
            {{ task.nodeId }} · {{ statusLabel }}
          </div>
        </div>
        <q-btn flat dense round icon="close" v-close-popup />
      </q-card-section>
      <q-separator />
      <div class="app-glass-dialog__scroll">
        <q-card-section v-if="task" class="app-dialog-body app-glass-dialog__body">
          <q-tabs v-model="tab" dense align="left" class="q-mb-md">
            <q-tab name="detail" label="详情" />
            <q-tab name="comments" label="评论" />
            <q-tab name="events" label="事件" />
            <q-tab name="logs" label="日志" />
            <q-tab name="runs" label="运行" />
          </q-tabs>
          <q-tab-panels v-model="tab" animated>
            <q-tab-panel name="detail" class="q-pa-none q-gutter-md">
              <q-input v-model="agentKey" dense outlined label="Agent Key" hint="认领任务时使用" />
              <q-input :model-value="task.input" dense outlined autogrow type="textarea" label="输入" readonly />
              <q-input v-model="submitOutput" dense outlined autogrow type="textarea" label="输出（提交）" />
              <q-input v-model="submitSummary" dense outlined label="摘要（提交）" />
              <div class="row q-gutter-sm">
                <q-btn
                  color="primary"
                  outline
                  label="认领"
                  :loading="actionLoading"
                  :disable="!canClaim"
                  @click="$emit('claim', { taskId: task.taskId, agentKey })"
                />
                <q-btn
                  color="primary"
                  unelevated
                  label="提交结果"
                  :loading="actionLoading"
                  :disable="!canSubmit"
                  @click="$emit('submit', { taskId: task.taskId, output: submitOutput, summary: submitSummary })"
                />
                <q-btn
                  color="warning"
                  outline
                  label="上报阻塞"
                  :loading="actionLoading"
                  :disable="!canReportBlocked"
                  @click="onReportBlocked"
                />
                <q-btn
                  v-if="canUnblock"
                  color="positive"
                  outline
                  label="解除阻塞"
                  :loading="actionLoading"
                  @click="onUnblock"
                />
              </div>
              <q-input
                v-if="canReportBlocked"
                v-model="blockedReason"
                class="q-mt-sm"
                dense
                outlined
                autogrow
                type="textarea"
                label="阻塞原因（可选）"
              />
              <q-input
                v-if="canUnblock"
                v-model="unblockComment"
                class="q-mt-sm"
                dense
                outlined
                autogrow
                type="textarea"
                label="解除阻塞说明（可选）"
              />
              <q-separator v-if="canReview" class="q-my-md" />
              <div v-if="canReview" class="q-gutter-sm">
                <div class="text-subtitle2">审核</div>
                <q-input v-model="reviewerAgent" dense outlined label="审核 Agent" />
                <q-input v-model="reviewComment" dense outlined autogrow type="textarea" label="审核意见" />
                <div class="row q-gutter-sm">
                  <q-btn
                    color="positive"
                    outline
                    label="通过"
                    :loading="actionLoading"
                    @click="onReview(true)"
                  />
                  <q-btn
                    color="negative"
                    outline
                    label="拒绝"
                    :loading="actionLoading"
                    @click="onReview(false)"
                  />
                </div>
              </div>
            </q-tab-panel>
            <q-tab-panel name="comments" class="q-pa-none">
              <q-spinner v-if="detailLoading" color="primary" size="28px" />
              <q-list v-else dense bordered separator class="rounded-borders">
                <q-item v-for="comment in comments" :key="comment.commentId">
                  <q-item-section>
                    <q-item-label>{{ comment.author }} · {{ comment.type }}</q-item-label>
                    <q-item-label caption>{{ comment.content }}</q-item-label>
                  </q-item-section>
                </q-item>
              </q-list>
              <q-input v-model="commentAuthor" class="q-mt-md" dense outlined label="作者" />
              <q-input v-model="commentContent" class="q-mt-sm" dense outlined autogrow type="textarea" label="评论" />
              <q-btn
                class="q-mt-sm"
                flat
                color="primary"
                label="添加评论"
                :disable="!commentContent.trim()"
                @click="$emit('addComment', { taskId: task.taskId, author: commentAuthor, content: commentContent })"
              />
            </q-tab-panel>
            <q-tab-panel name="events" class="q-pa-none">
              <q-spinner v-if="detailLoading" color="primary" size="28px" />
              <q-list v-else-if="events.length" dense bordered separator class="rounded-borders">
                <q-item v-for="event in events" :key="event.eventId">
                  <q-item-section>
                    <q-item-label>{{ event.eventType }}</q-item-label>
                    <q-item-label caption>{{ event.description }}</q-item-label>
                    <q-item-label caption class="text-grey-7">{{ event.timestamp }}</q-item-label>
                  </q-item-section>
                </q-item>
              </q-list>
              <div v-else class="text-caption text-grey-7 q-pa-sm">暂无事件记录</div>
            </q-tab-panel>
            <q-tab-panel name="logs" class="q-pa-none">
              <q-spinner v-if="detailLoading" color="primary" size="28px" />
              <q-list v-else dense bordered separator class="rounded-borders">
                <q-item v-for="log in logs" :key="log.logId">
                  <q-item-section>
                    <q-item-label>{{ log.level }} · {{ log.stream }}</q-item-label>
                    <q-item-label caption>{{ log.content }}</q-item-label>
                  </q-item-section>
                </q-item>
              </q-list>
            </q-tab-panel>
            <q-tab-panel name="runs" class="q-pa-none">
              <q-spinner v-if="detailLoading" color="primary" size="28px" />
              <q-list v-else dense bordered separator class="rounded-borders">
                <q-item v-for="run in runs" :key="run.runId">
                  <q-item-section>
                    <q-item-label>{{ run.runId }}</q-item-label>
                    <q-item-label caption>exit {{ run.exitCode }} · {{ run.startedAt }}</q-item-label>
                  </q-item-section>
                </q-item>
              </q-list>
            </q-tab-panel>
          </q-tab-panels>
        </q-card-section>
      </div>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import type { Task, TaskComment, TaskEvent, TaskLog, TaskRun } from "../../features/graph/types";
import { TASK_STATUS_LABELS } from "../../features/graph/types";

const props = defineProps<{
  open: boolean;
  task: Task | null;
  comments: TaskComment[];
  events: TaskEvent[];
  logs: TaskLog[];
  runs: TaskRun[];
  detailLoading: boolean;
  actionLoading: boolean;
}>();

const emit = defineEmits<{
  "update:open": [value: boolean];
  claim: [payload: { taskId: string; agentKey: string }];
  submit: [payload: { taskId: string; output: string; summary: string }];
  reportBlocked: [payload: { taskId: string; reason: string }];
  unblock: [payload: { taskId: string; comment: string }];
  review: [payload: { taskId: string; reviewerAgent: string; approved: boolean; comment: string }];
  addComment: [payload: { taskId: string; author: string; content: string }];
}>();

const tab = ref("detail");
const agentKey = ref("");
const submitOutput = ref("");
const submitSummary = ref("");
const blockedReason = ref("");
const unblockComment = ref("");
const reviewerAgent = ref("");
const reviewComment = ref("");
const commentAuthor = ref("");
const commentContent = ref("");

const statusLabel = computed(() =>
  props.task ? (TASK_STATUS_LABELS[props.task.status] ?? props.task.status) : "",
);

const canClaim = computed(() =>
  props.task?.status === "TASK_PENDING" || props.task?.status === "TASK_PENDING_ASSIGNMENT",
);

const canSubmit = computed(() =>
  props.task?.status === "TASK_CLAIMED" || props.task?.status === "TASK_REVIEW_REQUIRED",
);

const canReportBlocked = computed(() =>
  props.task?.status === "TASK_CLAIMED" || props.task?.status === "TASK_PENDING",
);

const canUnblock = computed(() => props.task?.status === "TASK_BLOCKED");

const canReview = computed(() => props.task?.status === "TASK_REVIEW_REQUIRED");

function onReportBlocked() {
  if (!props.task) return;
  const reason = blockedReason.value.trim() || submitSummary.value.trim() || "blocked";
  emit("reportBlocked", { taskId: props.task.taskId, reason });
}

function onUnblock() {
  if (!props.task) return;
  emit("unblock", { taskId: props.task.taskId, comment: unblockComment.value.trim() });
}

function onReview(approved: boolean) {
  if (!props.task) return;
  emit("review", {
    taskId: props.task.taskId,
    reviewerAgent: reviewerAgent.value,
    approved,
    comment: reviewComment.value,
  });
}

watch(
  () => props.task?.taskId,
  () => {
    tab.value = "detail";
    submitOutput.value = props.task?.output ?? "";
    submitSummary.value = props.task?.summary ?? "";
    commentContent.value = "";
    unblockComment.value = "";
    agentKey.value = "";
    reviewerAgent.value = "";
    commentAuthor.value = "";
    blockedReason.value = "";
    reviewComment.value = "";
  },
);

function onDialogUpdate(value: boolean) {
  emit("update:open", value);
}
</script>
