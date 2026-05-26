import { computed, onBeforeUnmount, ref } from "vue";
import type { QTableProps } from "quasar";
import { normalizeRegistryColumns } from "./registryTableColumns";

type RegistryColumn = NonNullable<QTableProps["columns"]>[number];

const STORAGE_PREFIX = "aranea.registry-table.cols.v1.";
const MIN_COL_WIDTH = 48;

function buildWidthCss(px: number): string {
  const width = Math.max(MIN_COL_WIDTH, Math.round(px));
  return `width: ${width}px; min-width: ${width}px; max-width: ${width}px`;
}

function derivePersistKey(columns: QTableProps["columns"], explicit?: string): string {
  if (explicit) return explicit;
  if (!Array.isArray(columns)) return "default";
  return columns
    .map((col) => (col && typeof col === "object" ? String(col.name ?? "") : ""))
    .filter(Boolean)
    .join("|");
}

function loadStoredWidths(key: string): Record<string, number> {
  try {
    const raw = localStorage.getItem(STORAGE_PREFIX + key);
    if (!raw) return {};
    const parsed = JSON.parse(raw) as unknown;
    if (!parsed || typeof parsed !== "object") return {};
    const out: Record<string, number> = {};
    for (const [name, value] of Object.entries(parsed)) {
      if (typeof value === "number" && Number.isFinite(value)) {
        out[name] = value;
      }
    }
    return out;
  } catch {
    return {};
  }
}

function saveStoredWidths(key: string, widths: Record<string, number>) {
  try {
    localStorage.setItem(STORAGE_PREFIX + key, JSON.stringify(widths));
  } catch {
    // quota / private mode — ignore
  }
}

export function useResizableRegistryColumns(
  columnsSource: () => QTableProps["columns"] | undefined,
  options: {
    enabled: () => boolean;
    persistKey?: () => string | undefined;
  },
) {
  const widthOverrides = ref<Record<string, number>>({});
  const activeResize = ref<{ colName: string; startX: number; startWidth: number } | null>(null);

  let loadedPersistKey = "";

  function ensureLoaded(columns: QTableProps["columns"]) {
    const key = derivePersistKey(columns, options.persistKey?.());
    if (key === loadedPersistKey) return;
    loadedPersistKey = key;
    widthOverrides.value = loadStoredWidths(key);
  }

  const displayColumns = computed(() => {
    const raw = columnsSource();
    if (!Array.isArray(raw)) return raw;
    if (!options.enabled()) return normalizeRegistryColumns(raw);

    ensureLoaded(raw);

    return normalizeRegistryColumns(
      raw.map((col) => {
        if (!col || typeof col !== "object") return col;
        const entry = col as RegistryColumn;
        const name = String(entry.name ?? "");
        const px = widthOverrides.value[name];
        if (px == null) return entry;
        const css = buildWidthCss(px);
        return { ...entry, style: css, headerStyle: css };
      }),
    );
  });

  function onResizeMove(event: MouseEvent) {
    if (!activeResize.value) return;
    const delta = event.clientX - activeResize.value.startX;
    const next = Math.max(MIN_COL_WIDTH, activeResize.value.startWidth + delta);
    widthOverrides.value = {
      ...widthOverrides.value,
      [activeResize.value.colName]: next,
    };
  }

  function onResizeEnd() {
    if (activeResize.value && loadedPersistKey) {
      saveStoredWidths(loadedPersistKey, widthOverrides.value);
    }
    activeResize.value = null;
    document.body.classList.remove("app-registry-col-resizing");
    document.removeEventListener("mousemove", onResizeMove);
    document.removeEventListener("mouseup", onResizeEnd);
  }

  function onResizeStart(colName: string, event: MouseEvent) {
    if (!options.enabled()) return;

    const th = (event.currentTarget as HTMLElement | null)?.closest("th");
    const startWidth = th?.getBoundingClientRect().width ?? MIN_COL_WIDTH;

    activeResize.value = { colName, startX: event.clientX, startWidth };
    document.body.classList.add("app-registry-col-resizing");
    document.addEventListener("mousemove", onResizeMove);
    document.addEventListener("mouseup", onResizeEnd);
  }

  onBeforeUnmount(onResizeEnd);

  return {
    displayColumns,
    onResizeStart,
    isResizing: computed(() => activeResize.value != null),
  };
}
