<template>
  <div v-if="memberCards.length === 0" class="team-member-kanban-empty">
    <q-icon name="group_add" size="32px" class="q-mb-sm" />
    <div>暂无成员</div>
    <div class="text-caption app-text-secondary q-mt-xs">请在团队编辑对话框中添加成员 Agent，组建团队协作。</div>
  </div>
  <WorkflowKanbanBoard
    v-else
    :columns="columns"
    :is-dark="isDark"
    group-name="team-members"
    empty-label="暂无成员"
    @reorder="onReorder"
  >
    <template #header>
      <div class="text-subtitle2 q-mb-md">成员看板</div>
      <div class="text-caption app-text-secondary q-mb-md">{{ headerHint }}</div>
    </template>
    <template #card="{ item }">
      <q-card flat bordered class="team-member-kanban-card q-mb-sm">
        <q-card-section class="q-py-sm">
          <div class="row items-center justify-between no-wrap q-mb-xs">
            <div class="col min-width-0">
              <div class="text-weight-medium ellipsis">{{ (item as MemberCard).label }}</div>
              <div class="text-caption text-grey-7">{{ (item as MemberCard).roleLabel }}</div>
            </div>
          </div>
          <div class="team-member-kanban-card__section">
            <div class="team-member-kanban-card__label">收到</div>
            <div>{{ (item as MemberCard).inputHint }}</div>
          </div>
          <div class="team-member-kanban-card__section">
            <div class="team-member-kanban-card__label">做什么</div>
            <div>{{ (item as MemberCard).responsibility }}</div>
          </div>
          <div class="team-member-kanban-card__section">
            <div class="team-member-kanban-card__label">交付</div>
            <div>{{ (item as MemberCard).outputHint }}</div>
          </div>
        </q-card-section>
      </q-card>
    </template>
  </WorkflowKanbanBoard>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import WorkflowKanbanBoard from '../workflow/WorkflowKanbanBoard.vue';
import type { CompileTeamGraphResult } from '../../features/orchestration/compileApi';
import { resolveTeamNodeDisplay } from '../../features/orchestration/teamNodeDisplay';
import { teamRoleLabel } from './teamUtils';
import type { NodeDef } from '../../features/graph/types';
import type { TeamDefinition } from '../../features/teams/types';

type MemberCard = {
  key: string;
  label: string;
  role: string;
  roleLabel: string;
  responsibility: string;
  inputHint: string;
  outputHint: string;
};

const emit = defineEmits<{
  reorder: [payload: { columnKey: string; items: unknown[] }];
}>();

const props = defineProps<{
  compiled: CompileTeamGraphResult | null;
  definition: TeamDefinition | null;
  isDark: boolean;
}>();

const ROLE_COLUMNS = [
  { key: 'coordinator', label: '协调' },
  { key: 'worker', label: '执行' },
  { key: 'synthesizer', label: '汇总' },
  { key: 'critic', label: '评审' },
  { key: 'other', label: '其他' },
];

const memberCards = computed<MemberCard[]>(() => {
  const nodes = props.compiled?.nodes ?? [];
  if (nodes.length > 0) {
    return nodes.map((node) => {
      const nodeDef: NodeDef = {
        id: node.id ?? '',
        funcRef: '',
        interruptBefore: false,
        interruptAfter: false,
        type: 'agent',
        description: node.description ?? '',
        instruction: node.taskPrompt ?? node.description ?? '',
        modelName: '',
        toolNames: [],
        agentName: node.agentName ?? '',
        destinations: [],
        requiredRole: node.role ?? '',
        assignmentMode: '',
        assignmentStrategy: '',
        reviewerAgent: '',
        reviewRules: '',
        timeoutSeconds: 0,
        heartbeatIntervalSeconds: 0,
        enableLeaseExtension: false,
        retryMaxAttempts: 0,
        failureAction: '',
        fallbackAgent: '',
        inputMapperJson: '',
        outputMapperJson: '',
        isolatedMessages: false,
        inputFromLastResponse: false,
        cacheEnabled: false,
        cacheTtlSeconds: 0,
      };
      const display = resolveTeamNodeDisplay(nodeDef, props.compiled, props.definition);
      return {
        key: node.id ?? node.agentName ?? '',
        label: display.displayName,
        role: display.role,
        roleLabel: display.roleLabel,
        responsibility: display.responsibility,
        inputHint: display.inputHint,
        outputHint: display.outputHint,
      };
    });
  }
  return (props.definition?.members ?? []).map((member) => ({
    key: `${member.agent_id}-${member.role}`,
    label: member.name || '成员',
    role: member.role,
    roleLabel: teamRoleLabel(member.role),
    responsibility: member.name || '执行分配任务',
    inputHint: '接收上游或协调者输入',
    outputHint: '写入 state 并传递给下游',
  }));
});

/** 提示语差异化：编译结果与成员配置回退两种数据来源给不同说明。 */
const headerHint = computed(() =>
  (props.compiled?.nodes?.length ?? 0) > 0
    ? '按角色查看成员分工（来自编排 Graph 编译结果）。'
    : '按角色查看成员分工（编排 Graph 未编译，显示成员配置）。',
);

function roleBucket(role: string) {
  const normalized = role.trim().toLowerCase();
  if (ROLE_COLUMNS.some((column) => column.key === normalized)) {
    return normalized;
  }
  return 'other';
}

/** 空列折叠：只渲染有成员的角色列。 */
const columns = computed(() =>
  ROLE_COLUMNS.map((column) => ({
    key: column.key,
    label: column.label,
    items: memberCards.value.filter((member) => roleBucket(member.role) === column.key),
  })).filter((column) => column.items.length > 0),
);

function onReorder(payload: { columnKey: string; items: unknown[] }) {
  emit('reorder', payload);
}
</script>
