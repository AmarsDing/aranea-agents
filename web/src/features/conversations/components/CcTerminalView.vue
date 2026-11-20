<template>
  <!-- 运行台终端的一次执行：直接接上终端（原样输出，能看能敲键）；要看改动、分屏、提交去运行台 -->
  <div class="cc-xview">
    <FleetTerminalView
      v-if="terminal"
      :key="terminal.id"
      class="cc-xview__term"
      :terminal-id="terminal.id"
      :active="true"
      @restart="restart"
      @jump="openInFleet"
    />
    <p v-else class="cc-xview__empty">{{ t('cc.terminal.gone') }}</p>
    <footer class="cc-xview__dock">
      <span class="cc-xview__note">{{ t('cc.terminal.via') }}</span>
      <q-btn flat dense no-caps icon="open_in_new" :label="t('cc.terminal.openInFleet')" @click="openInFleet" />
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { useRouter } from 'vue-router';
import { Notify } from 'quasar';
import { errText, restartTerminal } from '../../fleet/api';
import { useFleet } from '../../fleet/useFleet';
import FleetTerminalView from '../../fleet/components/FleetTerminalView.vue';
import type { Execution } from '../model';

const props = defineProps<{ exec: Execution }>();
const { t } = useI18n();
const router = useRouter();
const fleet = useFleet();
const terminal = computed(() => fleet.state.terminals.find((x) => x.id === props.exec.refId) ?? null);

function openInFleet() {
  fleet.focusTerminal(props.exec.refId);
  void router.push('/fleet');
}
async function restart() {
  try {
    await restartTerminal(props.exec.refId);
  } catch (e) {
    Notify.create({ type: 'negative', message: errText(e) });
  }
}
</script>
