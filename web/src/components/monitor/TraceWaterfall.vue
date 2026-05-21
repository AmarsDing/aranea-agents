<template>
  <div>
    <div v-if="!waterfallRows.length" class="text-caption text-grey-7 q-pa-sm">暂无 Span 数据</div>
    <div v-else class="trace-waterfall">
      <div v-for="row in waterfallRows" :key="row.id" class="trace-waterfall-row row items-center q-col-gutter-sm q-mb-xs">
        <div class="col-12 col-md-4">
          <div class="text-body2 text-weight-medium">{{ row.name }}</div>
          <div class="text-caption text-grey-7">{{ row.caption }}</div>
        </div>
        <div class="col-12 col-md-8">
          <div class="trace-waterfall-track">
            <div class="trace-waterfall-bar" :style="{ marginLeft: `${row.offsetPct}%`, width: `${row.widthPct}%` }" />
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";

type SpanRow = {
  id: string;
  name: string;
  caption: string;
  startMs: number;
  durationMs: number;
  offsetPct: number;
  widthPct: number;
};

const props = defineProps<{
  spans: unknown[];
}>();

function flattenSpans(input: unknown[], out: Omit<SpanRow, "offsetPct" | "widthPct">[] = []) {
  for (const raw of input) {
    if (!raw || typeof raw !== "object") continue;
    const row = raw as Record<string, unknown>;
    const id = String(row.id ?? row.name ?? out.length);
    const durationMs = Number(row.duration_ms ?? row.durationMs ?? 0);
    const startMs = Number(row.start_ms ?? row.startMs ?? 0);
    const name = String(row.name ?? row.type ?? "span");
    const caption = [row.status, durationMs ? `${durationMs}ms` : "", row.tool_name ?? row.model]
      .filter(Boolean)
      .join(" · ");
    out.push({ id, name, caption, startMs, durationMs });
    const children = row.children;
    if (Array.isArray(children)) flattenSpans(children, out);
  }
  return out;
}

const waterfallRows = computed((): SpanRow[] => {
  const flat = flattenSpans(props.spans);
  let max = 1;
  for (const row of flat) {
    const end = row.startMs + row.durationMs;
    if (end > max) max = end;
  }
  return flat.map((row) => ({
    ...row,
    offsetPct: Math.min(100, (row.startMs / max) * 100),
    widthPct: Math.max(2, (row.durationMs / max) * 100)
  }));
});
</script>

<style scoped>
.trace-waterfall-track {
  position: relative;
  height: 12px;
  background: rgba(0, 0, 0, 0.06);
  border-radius: 6px;
  overflow: hidden;
}
.trace-waterfall-bar {
  height: 100%;
  min-width: 4px;
  background: linear-gradient(90deg, #1976d2, #26a69a);
  border-radius: 6px;
}
</style>

