<template>
  <aside class="task-activity column no-wrap" :aria-label="t('taskWorkspace.activity')" tabindex="-1">
    <header class="row items-center q-pa-sm">
      <strong>{{ t('taskWorkspace.activity') }}</strong
      ><q-space />
      <q-btn flat round dense icon="close" :aria-label="t('taskWorkspace.closePanel')" @click="emit('close')" />
    </header>
    <p class="text-caption q-px-sm q-mb-sm">{{ t('taskWorkspace.activityHint') }}</p>
    <div class="task-activity__body">
      <h2>{{ t('taskWorkspace.needsAttention', { count: pendingSteps.length }) }}</h2>
      <p v-if="!pendingSteps.length" class="text-caption">
        {{ t(incomplete ? 'taskWorkspace.attentionUnknown' : 'taskWorkspace.noAttention') }}
      </p>
      <q-list bordered separator>
        <q-item v-for="step in pendingSteps" :key="step.ID">
          <q-item-section>
            <q-btn
              flat
              no-caps
              align="left"
              class="task-activity__locate"
              @click="emit('locate', step.TaskID, step.ID)"
            >
              <span class="column items-start"
                ><span>{{ step.ToolName || t('taskWorkspace.clarification') }}</span
                ><span class="text-caption">{{ t('taskWorkspace.openOriginalCard') }}</span></span
              >
              <q-icon name="arrow_forward" class="q-ml-sm" />
            </q-btn> </q-item-section
          ><q-item-section side
            ><q-btn
              flat
              round
              dense
              icon="link"
              :aria-label="t('taskWorkspace.copyLocationLink')"
              @click.stop="emit('copy-link', step.TaskID, step.ID)"
          /></q-item-section>
        </q-item>
      </q-list>
      <h2>{{ t('taskWorkspace.taskTimeline', { count: tasks.length }) }}</h2>
      <p v-if="!tasks.length" class="text-caption">{{ t('taskWorkspace.noTasks') }}</p>
      <section
        v-if="process && (tasks.length || process.manual)"
        class="task-activity__process"
        :aria-label="t('taskWorkspace.processTitle')"
      >
        <q-select
          dense
          outlined
          emit-value
          map-options
          :model-value="process.task?.ID"
          :options="taskOptions"
          :label="t('taskWorkspace.chooseTask')"
          @update:model-value="selectTask"
        />
        <div class="row items-center justify-between q-mt-xs">
          <span class="text-caption">{{
            t(process.manual ? 'taskWorkspace.viewingHistory' : 'taskWorkspace.followingCurrent')
          }}</span>
          <q-btn
            flat
            dense
            no-caps
            :disable="!process.manual"
            :label="t('taskWorkspace.followCurrentTask')"
            @click="emit('follow-current')"
          />
        </div>
        <p v-if="process.missingSelection" role="status" class="text-caption">
          {{ t('taskWorkspace.processUnavailable') }}
        </p>
        <p v-else-if="process.state === 'loading'" role="status" class="text-caption">
          {{ t('taskWorkspace.loadingProcess') }}
        </p>
        <div v-else-if="process.state === 'error'" role="alert">
          <p class="text-caption">{{ t('taskWorkspace.processLoadFailed') }}</p>
          <q-btn outline no-caps :label="t('taskWorkspace.retryProcess')" @click="emit('retry-process')" />
        </div>
        <template v-else-if="process.state === 'ready' && process.task">
          <q-select
            v-if="process.turns.length"
            dense
            outlined
            emit-value
            map-options
            class="q-mt-sm"
            :model-value="process.turn?.ID"
            :options="turnOptions"
            :label="t('taskWorkspace.chooseTurn')"
            @update:model-value="selectTurn"
          />
          <p v-else class="text-caption">{{ t('taskWorkspace.noTurns') }}</p>
          <div v-if="process.turn" class="row items-center q-gutter-xs q-my-xs">
            <span class="text-caption">{{ t(`taskWorkspace.turnStates.${process.turn.Status}`) }}</span>
            <q-btn
              v-if="process.canLocateTurn"
              flat
              dense
              no-caps
              :label="t('taskWorkspace.openOriginalTurn')"
              @click="emit('locate-turn', process.task.ID, process.turn.ID)"
            />
            <q-btn
              v-if="process.member"
              flat
              dense
              no-caps
              :label="t('taskWorkspace.openMemberDetails')"
              @click="emit('open-member', process.task.ID, process.member.ID)"
            />
          </div>
          <p v-if="process.turn && !process.canLocateTurn && !process.member" class="text-caption">
            {{ t('taskWorkspace.processReadOnly') }}
          </p>
          <p v-if="!process.steps.length" class="text-caption">{{ t('taskWorkspace.noProcessSteps') }}</p>
          <q-list v-else separator :aria-label="t('taskWorkspace.processSteps')">
            <q-item v-for="step in process.steps" :key="step.ID">
              <q-item-section>
                <q-item-label class="task-activity__step-label">{{
                  step.ToolName || t(`taskWorkspace.stepKinds.${step.Kind}`)
                }}</q-item-label>
                <q-item-label caption>{{ t(`taskWorkspace.stepStates.${step.Status}`) }}</q-item-label>
              </q-item-section>
              <q-item-section side>
                <q-btn
                  v-if="
                    (process.canLocateTurn && step.Kind !== 'clarify') ||
                    (!process.turn && step.SessionID === process.task.SessionID)
                  "
                  flat
                  round
                  dense
                  icon="arrow_forward"
                  :aria-label="t('taskWorkspace.openOriginalStep')"
                  @click="emit('locate', process.task.ID, step.ID)"
                />
              </q-item-section>
            </q-item>
          </q-list>
        </template>
      </section>
      <q-list v-else separator>
        <q-item v-for="task in tasks" :key="task.ID">
          <q-item-section>
            <q-btn flat no-caps align="left" class="task-activity__locate" @click="emit('locate', task.ID)">
              <span class="column items-start"
                ><span class="task-activity__title">{{ task.UserMessage }}</span
                ><span class="text-caption">{{ t(`taskWorkspace.taskStates.${task.Status}`) }}</span></span
              >
              <q-icon name="arrow_forward" class="q-ml-sm" />
            </q-btn> </q-item-section
          ><q-item-section side
            ><q-btn
              flat
              round
              dense
              icon="link"
              :aria-label="t('taskWorkspace.copyLocationLink')"
              @click.stop="emit('copy-link', task.ID)"
          /></q-item-section>
        </q-item>
      </q-list>
    </div>
  </aside>
</template>
<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { Step, Task } from '../../features/chat/v2Types';
import type { TaskWorkspaceProcessView } from '../../features/chat/types';
const props = defineProps<{
  tasks: Task[];
  pendingSteps: Step[];
  incomplete: boolean;
  process?: TaskWorkspaceProcessView;
  executorNames?: ReadonlyMap<string, { displayName: string }>;
}>();
const emit = defineEmits<{
  locate: [taskId: string, stepId?: string];
  'copy-link': [taskId: string, stepId?: string];
  'select-task': [taskId: string];
  'select-turn': [turnId: string];
  'locate-turn': [taskId: string, turnId: string];
  'open-member': [taskId: string, memberId: string];
  'follow-current': [];
  'retry-process': [];
  close: [];
}>();
const { t } = useI18n();
const taskOptions = computed(() =>
  props.tasks.map((task) => ({
    value: task.ID,
    label: t('taskWorkspace.taskOption', { seq: task.Seq, title: task.UserMessage }),
  })),
);
const turnOptions = computed(
  () =>
    props.process?.turns.map((turn) => ({
      value: turn.ID,
      label: t('taskWorkspace.turnOption', {
        seq: turn.Seq,
        agent:
          props.executorNames?.get(turn.AgentKey)?.displayName || turn.AgentKey || t('taskWorkspace.unknownExecutor'),
      }),
    })) ?? [],
);
function selectTask(value: unknown) {
  if (typeof value === 'string') emit('select-task', value);
}
function selectTurn(value: unknown) {
  if (typeof value === 'string') emit('select-turn', value);
}
</script>
<style scoped lang="sass">
.task-activity
  background: var(--glass-surface)
  border: 1px solid var(--glass-border)
  border-radius: var(--radius-lg)
  overflow: hidden
.task-activity__body
  padding: 8px
  overflow: auto
  min-height: 0
  h2
    font-size: 14px
    font-weight: 600
    line-height: 1.5
    margin: 12px 8px
.task-activity__title
  display: -webkit-box
  -webkit-line-clamp: 2
  -webkit-box-orient: vertical
  overflow: hidden
  overflow-wrap: anywhere
.task-activity__locate
  min-width: 0
  width: 100%
  padding: 8px
.task-activity__process
  padding: 0 var(--space-2) var(--space-2)
  :deep(.q-btn)
    min-height: var(--control-height-touch, 44px)
  :deep(.q-field__native)
    min-width: 0
    > span
      overflow: hidden
      white-space: nowrap
      text-overflow: ellipsis
      max-width: 100%
.task-activity__step-label
  overflow-wrap: anywhere
</style>
