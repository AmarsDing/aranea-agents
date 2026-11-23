<template>
  <!--
    执行者头像：编码工具用产品图标（public/coding-tools/），没有图标的工具用两个字母；
    本平台同事 / 团队、流程用名字的第一个字。只是识别，旁边总有文字。
  -->
  <span
    class="cc-glyph"
    :class="[`cc-glyph--${tone}`, size ? `cc-glyph--${size}` : '', { 'cc-glyph--img': icon && !broken }]"
    :title="name"
    aria-hidden="true"
  >
    <img v-if="icon && !broken" :src="icon" alt="" @error="broken = true" />
    <template v-else>{{ mono }}</template>
  </span>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { KNOWN_TOOLS, executorGroup } from '../../features/conversations/model';
import { toolIcon } from '../../features/fleet/api';

const props = defineProps<{ executor: string; name: string; size?: 'xs' | 'sm' | 'lg' }>();

const MONO: Record<string, string> = { cc: 'CC', cx: 'CX', wb: 'WB', ds: 'DS', gk: 'GK' };
// 会话中心里的工具键 → 编码工具页的工具 id（图标按工具 id 取）
const PRESET: Record<string, string> = {
  cc: 'claude',
  cx: 'codex',
  wb: 'codebuddy',
  ds: 'deepseek',
  gk: 'grok',
  tr: 'trae',
};

const group = computed(() => executorGroup(props.executor));
const tone = computed(() =>
  group.value === 'ag' ? 'ag' : (KNOWN_TOOLS as readonly string[]).includes(group.value) ? group.value : 'other',
);
// 编码工具桥登记的键常带前缀（cli_grok、fleet:cursor）；同事 / 团队 / 流程没有产品图标
const bare = computed(() => props.executor.replace(/^(fleet:|cli_)/, '').toLowerCase());
const icon = computed(() =>
  group.value === 'ag' || group.value === 'flow' ? '' : toolIcon(PRESET[bare.value] ?? bare.value),
);
const broken = ref(false);
watch(icon, () => (broken.value = false));

const mono = computed(
  () =>
    MONO[group.value] ??
    (group.value === 'ag' || group.value === 'flow'
      ? (Array.from(props.name.trim())[0] ?? '?')
      : bare.value
          .replace(/[^a-z0-9]/gi, '')
          .slice(0, 2)
          .toUpperCase() || '?'),
);
</script>
