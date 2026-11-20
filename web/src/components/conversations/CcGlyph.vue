<template>
  <!-- 执行者小方块：编码工具用两个字母（CC / CX / WB / DS / GK），本平台智能体 / 团队用名字的第一个字。只是识别，旁边总有文字 -->
  <span
    class="cc-glyph"
    :class="[`cc-glyph--${tone}`, size ? `cc-glyph--${size}` : '']"
    :title="name"
    aria-hidden="true"
    >{{ mono }}</span
  >
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { KNOWN_TOOLS, executorGroup } from '../../features/conversations/model';

const props = defineProps<{ executor: string; name: string; size?: 'xs' | 'lg' }>();

const MONO: Record<string, string> = { cc: 'CC', cx: 'CX', wb: 'WB', ds: 'DS', gk: 'GK' };
const group = computed(() => executorGroup(props.executor));
const tone = computed(() =>
  group.value === 'ag' ? 'ag' : (KNOWN_TOOLS as readonly string[]).includes(group.value) ? group.value : 'other',
);
const mono = computed(
  () =>
    MONO[group.value] ??
    (group.value === 'ag'
      ? (Array.from(props.name.trim())[0] ?? '?')
      : props.executor
          .replace(/[^a-z0-9]/gi, '')
          .slice(0, 2)
          .toUpperCase() || '?'),
);
</script>
