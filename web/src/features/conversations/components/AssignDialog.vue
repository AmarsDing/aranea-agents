<template>
  <!--
    全局交办面板：从指令条、任务页、看板节点等处打开的都是这一个（会话中心页内用的是同一个 CcComposer）。
    设计：docs/aranea-agents/plan/2026-11-21-任务入口与执行统筹方案.md §4
  -->
  <!-- 点到面板外面不关：写了一半的要求不该因为手滑丢掉；关闭用右上角的 × 或 Esc -->
  <q-dialog v-model="open" position="right" full-height no-backdrop-dismiss @hide="req = null">
    <div class="cc-assign">
      <CcComposer
        v-if="req"
        :key="seq"
        mode="new"
        :text="req.text"
        :files="req.files"
        :to="req.to"
        :prefer="req.prefer"
        :recent="recent"
        :inputs="[]"
        :outputs="[]"
        :can-fleet="canFleet"
        :presets="fleet.state.presets"
        :workspaces="fleet.state.workspaces"
        @close="open = false"
        @sent="onSent"
      />
    </div>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { useFleet } from '../../fleet/useFleet';
import { COMMAND_DONE_EVENT, type CommandDoneDetail } from '../../today/useCommandRunner';
import { useNavStore } from '../../../stores/nav';
import { OPEN_ASSIGN_EVENT, pushRecent, readRecent, type AssignRequest } from '../assign';
import CcComposer from './CcComposer.vue';

const nav = useNavStore();
const fleet = useFleet();
const canFleet = computed(() => nav.permissions.includes('*') || nav.permissions.includes('coding.manage'));

const open = ref(false);
const req = ref<AssignRequest | null>(null);
const recent = ref<string[]>([]);
let seq = 0;

function onOpen(event: Event) {
  req.value = (event as CustomEvent<AssignRequest>).detail ?? {};
  recent.value = readRecent();
  seq++;
  open.value = true;
  // 「几个工具一起做」要运行台的工具和工作区；读不到就只是少这一项
  if (canFleet.value && !fleet.state.presets.length) void fleet.refreshAll().catch(() => undefined);
}

function onSent(r: { executor: string }) {
  if (r.executor.startsWith('agent:') || r.executor.startsWith('team:')) pushRecent(r.executor);
  open.value = false;
  // 任务页、今日页、会话中心听这个事件刷新
  window.dispatchEvent(
    new CustomEvent<CommandDoneDetail>(COMMAND_DONE_EVENT, { detail: { intent: 'delegate', task: null } }),
  );
}

onMounted(() => window.addEventListener(OPEN_ASSIGN_EVENT, onOpen));
onBeforeUnmount(() => window.removeEventListener(OPEN_ASSIGN_EVENT, onOpen));
</script>
