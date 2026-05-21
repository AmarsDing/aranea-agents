import { onBeforeUnmount, onMounted, shallowRef, watch, type Ref } from "vue";
import type { EChartsCoreOption, EChartsType } from "echarts/core";
import { echarts, ensureUsageEcharts } from "./usageEcharts";

const RESIZE_DEBOUNCE_MS = 150;

function watchBodyClass(onChange: () => void) {
  const observer = new MutationObserver(onChange);
  observer.observe(document.body, { attributes: true, attributeFilter: ["class"] });
  return () => observer.disconnect();
}

/**
 * Host ECharts in a DOM ref; option builder stays in the caller (SRP).
 */
export function useUsageChart(
  chartEl: Ref<HTMLElement | null>,
  buildOption: () => EChartsCoreOption,
  deps: () => unknown[]
) {
  const chartRef = shallowRef<EChartsType | null>(null);
  let resizeTimer: ReturnType<typeof setTimeout> | null = null;
  let stopThemeWatch: (() => void) | null = null;

  function render() {
    if (!chartEl.value) return;
    ensureUsageEcharts();
    if (!chartRef.value) {
      chartRef.value = echarts.init(chartEl.value);
    }
    chartRef.value.setOption(buildOption(), { notMerge: true });
    chartRef.value.resize();
  }

  function scheduleRender() {
    if (resizeTimer) clearTimeout(resizeTimer);
    resizeTimer = setTimeout(render, RESIZE_DEBOUNCE_MS);
  }

  onMounted(() => {
    render();
    window.addEventListener("resize", scheduleRender);
    stopThemeWatch = watchBodyClass(scheduleRender);
  });

  onBeforeUnmount(() => {
    window.removeEventListener("resize", scheduleRender);
    stopThemeWatch?.();
    if (resizeTimer) clearTimeout(resizeTimer);
    chartRef.value?.dispose();
    chartRef.value = null;
  });

  watch(deps, () => render());

  return { render };
}
