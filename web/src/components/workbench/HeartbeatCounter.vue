<template>
  <div class="wb-heartbeat" :title="tipText" role="img" :aria-label="String(value)">
    <div v-for="(ch, i) in digits" :key="i" class="wb-heartbeat__digit">
      <span
        v-for="(cell, j) in glyphOf(ch)"
        :key="j"
        class="wb-heartbeat__cell"
        :class="{ on: cell, flip: cell && flippedIndexes.has(i) }"
      />
    </div>
    <div class="wb-heartbeat__count">{{ value }}</div>
  </div>
</template>

<script setup lang="ts">
/**
 * 心跳 THE HEARTBEAT（配角②，设计 §2.5.2）：
 * 5×7 点阵「今日已完成」计数；色=--wb-flow 青（完成≠轮到人，金=语义造假）。
 * 数据=真实完成事件（task/team_run completed + 签收 + decision 已了回填），禁演示数字。
 */
import { computed, ref, watch } from 'vue';

const props = defineProps<{
  value: number;
  tipText: string;
}>();

// 5 列 × 7 行点阵字库（0-9 + 占位空格）
const GLYPHS: Record<string, readonly string[]> = {
  '0': ['#####', '#...#', '#..##', '#.#.#', '##..#', '#...#', '#####'],
  '1': ['..#..', '.##..', '..#..', '..#..', '..#..', '..#..', '.###.'],
  '2': ['#####', '....#', '....#', '#####', '#....', '#....', '#####'],
  '3': ['#####', '....#', '....#', '.####', '....#', '....#', '#####'],
  '4': ['#...#', '#...#', '#...#', '#####', '....#', '....#', '....#'],
  '5': ['#####', '#....', '#....', '#####', '....#', '....#', '#####'],
  '6': ['#####', '#....', '#....', '#####', '#...#', '#...#', '#####'],
  '7': ['#####', '....#', '...#.', '..#..', '.#...', '.#...', '.#...'],
  '8': ['#####', '#...#', '#...#', '#####', '#...#', '#...#', '#####'],
  '9': ['#####', '#...#', '#...#', '#####', '....#', '....#', '#####'],
  '+': ['.....', '..#..', '..#..', '#####', '..#..', '..#..', '.....'],
  ' ': ['.....', '.....', '.....', '.....', '.....', '.....', '.....'],
};

const digits = computed(() => {
  const n = props.value;
  if (n > 9999) return ['9', '9', '9', '9', '+'];
  const s = String(n);
  return s.split('');
});

function glyphOf(ch: string): boolean[] {
  const rows = GLYPHS[ch] ?? GLYPHS[' '];
  return rows
    .join('')
    .split('')
    .map((c) => c === '#');
}

/** 进位迟疑：变化位 flick（80ms 延迟由 transition 表达；旧值位不翻） */
const flippedIndexes = ref(new Set<number>());
watch(
  () => props.value,
  (nv, ov) => {
    const a = String(ov ?? '');
    const b = String(nv ?? '');
    const changed = new Set<number>();
    for (let i = 0; i < b.length; i += 1) {
      if (a[a.length - b.length + i] !== b[i]) changed.add(i);
    }
    flippedIndexes.value = changed.size ? changed : flippedIndexes.value;
    setTimeout(() => {
      flippedIndexes.value = new Set();
    }, 200);
  },
);
</script>

<style scoped lang="sass">
.wb-heartbeat
  display: inline-flex
  align-items: center
  gap: 8px
.wb-heartbeat__digit
  display: grid
  grid-template-columns: repeat(5, 3px)
  grid-template-rows: repeat(7, 3px)
  gap: 1px
.wb-heartbeat__cell
  width: 3px
  height: 3px
  border-radius: 0.5px
  background: color-mix(in srgb, var(--wb-flow) 10%, transparent)
  &.on
    background: var(--wb-flow)
  &.flip
    animation: wb-cell-flip 160ms ease
.wb-heartbeat__count
  font-size: var(--wb-kpi)
  line-height: 1
  font-variant-numeric: tabular-nums
  color: var(--color-text-primary)
  // 稳态唯一辉光特许领取者由 OverviewGrid KPI 带裁决；计数本体不发光
</style>
