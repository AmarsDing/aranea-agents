<template>
  <WorkflowKanbanBoard
    :columns="columns"
    :is-dark="isDark"
    group-name="team-members"
    empty-label="暂无成员"
    @reorder="onReorder"
  >
    <template #header>
      <div class="text-subtitle2 q-mb-md">成员看板</div>
      <div class="text-caption app-text-secondary q-mb-md">按角色查看 Team 成员与编译节点。</div>
    </template>
    <template #card="{ item }">
      <q-card
        flat
        bordered
        class="team-member-kanban-card q-mb-sm"
      >
        <q-card-section class="q-py-sm">
          <div class="row items-center justify-between no-wrap q-mb-xs">
            <div class="col min-width-0">
              <div class="text-weight-medium ellipsis">{{ (item as MemberCard).label }}</div>
              <div class="text-caption text-grey-7">{{ (item as MemberCard).roleLabel }} · {{ (item as MemberCard).agentKey || "—" }}</div>
            </div>
            <q-badge dense rounded>{{ (item as MemberCard).nodeType }}</q-badge>
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
import { computed } from "vue";
import WorkflowKanbanBoard from "../workflow/WorkflowKanbanBoard.vue";
import type { CompileTeamGraphResult } from "../../features/orchestration/compileApi";
import { resolveTeamNodeDisplay } from "../../features/orchestration/teamNodeDisplay";
import type { NodeDef } from "../../features/graph/types";
import type { TeamDefinition } from "../../features/teams/types";

type MemberCard = {
  key: string;
  label: string;
  role: string;
  roleLabel: string;
  agentKey: string;
  nodeType: string;
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
  { key: "coordinator", label: "协调" },
  { key: "worker", label: "执行" },
  { key: "synthesizer", label: "汇总" },
  { key: "critic", label: "评审" },
  { key: "other", label: "其他" },
];

const memberCards = computed<MemberCard[]>(() => {
  const nodes = props.compiled?.nodes ?? [];
  if (nodes.length > 0) {
    return nodes.map((node) => {
      const nodeDef: NodeDef = {
        id: node.id ?? "",
        funcRef: "",
        interruptBefore: false,
        interruptAfter: false,
        type: "agent",
        description: node.description ?? "",
        instruction: node.taskPrompt ?? node.description ?? "",
        modelName: "",
        toolNames: [],
        agentName: node.agentName ?? "",
        destinations: [],
        requiredRole: node.role ?? "",
        assignmentMode: "",
        assignmentStrategy: "",
        reviewerAgent: "",
        reviewRules: "",
        timeoutSeconds: 0,
        heartbeatIntervalSeconds: 0,
        enableLeaseExtension: false,
        retryMaxAttempts: 0,
        failureAction: "",
        fallbackAgent: "",
        inputMapperJson: "",
        outputMapperJson: "",
        isolatedMessages: false,
        inputFromLastResponse: false,
        cacheEnabled: false,
        cacheTtlSeconds: 0,
      };
      const display = resolveTeamNodeDisplay(nodeDef, props.compiled, props.definition);
      return {
        key: node.id ?? node.agentName ?? "",
        label: display.displayName,
        role: display.role,
        roleLabel: display.roleLabel,
        agentKey: display.agentKey,
        nodeType: node.type || "agent",
        responsibility: display.responsibility,
        inputHint: display.inputHint,
        outputHint: display.outputHint,
      };
    });
  }
  return (props.definition?.members ?? []).map((member) => ({
    key: `${member.agent_id}-${member.role}`,
    label: member.name || member.agent_id,
    role: member.role,
    roleLabel: member.role,
    agentKey: member.agent_id,
    nodeType: member.enabled ? "enabled" : "disabled",
    responsibility: member.name || "执行分配任务",
    inputHint: "接收上游或协调者输入",
    outputHint: "写入 state 并传递给下游",
  }));
});

function roleBucket(role: string) {
  const normalized = role.trim().toLowerCase();
  if (ROLE_COLUMNS.some((column) => column.key === normalized)) {
    return normalized;
  }
  return "other";
}

const columns = computed(() =>
  ROLE_COLUMNS.map((column) => ({
    key: column.key,
    label: column.label,
    items: memberCards.value.filter((member) => roleBucket(member.role) === column.key),
  })),
);

function onReorder(payload: { columnKey: string; items: unknown[] }) {
  emit("reorder", payload);
}
</script>
