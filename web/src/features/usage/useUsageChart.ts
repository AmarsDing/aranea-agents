import { onBeforeUnmount, onMounted, shallowRef, watch, type Ref } from 'vue';
import type { EChartsCoreOption, EChartsType } from 'echarts/core';
import { echarts, ensureUsageEcharts } from './usageEcharts';

const RESIZE_DEBOUNCE_MS = 150;

function watchBodyClass(onChange: () => void) {
  const observer = new MutationObserver(onChange);
  observer.observe(document.body, { attributes: true, attributeFilter: ['class'] });
  return () => observer.disconnect();
}

/**
 * Host ECharts in a DOM ref; option builder stays in the caller (SRP).
 */
export function useUsageChart(
  chartEl: Ref<HTMLElement | null>,
  buildOption: () => EChartsCoreOption,
  deps: () => unknown[],
) {
  const chartRef = shallowRef<EChartsType | null>(null);
  let resizeTimer: ReturnType<typeof setTimeout> | null = null;
  let stopThemeWatch: (() => void) | null = null;
  let resizeObserver: ResizeObserver | null = null;

  function render() {
    if (!chartEl.value) return;
    // Skip render when container is hidden (display:none) — zero-dimension init produces blank charts
    if (chartEl.value.offsetWidth === 0 || chartEl.value.offsetHeight === 0) return;
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
    window.addEventListener('resize', scheduleRender);
    stopThemeWatch = watchBodyClass(scheduleRender);

    // Re-render when container becomes visible (tab switch, dialog open, v-show toggle)
    if (typeof ResizeObserver !== 'undefined') {
      resizeObserver = new ResizeObserver(() => {
        scheduleRender();
      });
      if (chartEl.value) {
        resizeObserver.observe(chartEl.value);
      }
    }
  });

  // Late-bind ResizeObserver if chartEl is set after mount (e.g. v-show toggles)
  const stopElWatch = watch(chartEl, (el) => {
    if (el && resizeObserver) {
      resizeObserver.observe(el);
    }
  });

  onBeforeUnmount(() => {
    window.removeEventListener('resize', scheduleRender);
    stopThemeWatch?.();
    stopElWatch();
    resizeObserver?.disconnect();
    if (resizeTimer) clearTimeout(resizeTimer);
    chartRef.value?.dispose();
    chartRef.value = null;
  });

  watch(deps, () => render());

  return { render };
}
