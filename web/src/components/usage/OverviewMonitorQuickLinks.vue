<template>
  <q-btn-dropdown
    flat
    no-caps
    icon="monitor_heart"
    label="运维监控"
    class="overview-monitor-dropdown"
  >
    <q-list dense>
      <q-item v-for="link in links" :key="link.tab" clickable v-close-popup @click="openTab(link.tab)">
        <q-item-section avatar>
          <q-icon :name="link.icon" />
        </q-item-section>
        <q-item-section>
          <q-item-label>{{ link.label }}</q-item-label>
          <q-item-label caption>{{ link.caption }}</q-item-label>
        </q-item-section>
      </q-item>
    </q-list>
  </q-btn-dropdown>
</template>

<script setup lang="ts">
import { useMonitorRunNavigation } from "../../features/monitor/useMonitorRunNavigation";

const { openMonitorTab } = useMonitorRunNavigation();

const links = [
  { tab: "traces", icon: "account_tree", label: "Runs（Traces）", caption: "单次运行排障" },
  { tab: "events", icon: "sensors", label: "实时事件", caption: "Team / 告警流" },
  { tab: "alerts", icon: "notifications_active", label: "告警规则", caption: "错误率阈值与通知" },
  { tab: "logs", icon: "terminal", label: "日志", caption: "流程 / 进程日志" }
] as const;

function openTab(tab: string) {
  openMonitorTab(tab);
}
</script>
