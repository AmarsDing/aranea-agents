<template>
  <q-banner v-if="config" rounded class="app-info-banner">
    <div class="text-subtitle2">A2A 公开 Endpoint</div>
    <div class="text-body2">{{ config.public_base_url || "—" }}</div>
    <div class="text-caption">
      来源：{{ sourceLabel }} · 可在
      <router-link to="/settings">系统设置</router-link>
      配置 A2A 公开前缀（保存后立即生效）
    </div>
  </q-banner>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import type { A2ARuntimeConfig } from "../../features/a2a/types";
import { getA2AConfig } from "../../features/a2a/api";

const config = ref<A2ARuntimeConfig | null>(null);

const sourceLabel = computed(() => {
  const s = config.value?.public_base_url_source;
  if (s === "env") return "环境变量";
  if (s === "db") return "系统设置";
  if (s === "config") return "配置文件";
  if (s === "derived") return "推导（开发默认）";
  return s || "—";
});

onMounted(async () => {
  try {
    config.value = await getA2AConfig();
  } catch {
    config.value = null;
  }
});
</script>
