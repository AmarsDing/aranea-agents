import type { QTableColumn, QTableProps } from "quasar";

type RegistryColumn = NonNullable<QTableProps["columns"]>[number];

/** AppRegistryTable / AppRegistryMarkupTable 统一列定义类型 */
export type RegistryTableColumn<T extends Record<string, any> = Record<string, any>> = QTableColumn<T>;

/** 常用列宽 token（Registry 列表统一引用，避免散落 width 字符串） */
export const REGISTRY_COL_W = {
  name: "14%",
  nameWide: "18%",
  desc: "16%",
  status: "9%",
  time: "50px",
  timeWide: "18%",
  agent: "10%",
  session: "10%",
  category: "11%",
  stats: "20%",
  enabled: "40px",
  metric: "72px",
  narrow: "64px",
  actions: "90px",
  actionsWide: "100px",
  select: "48px"
} as const;

/**
 * Quasar QTable：表头 th 读 headerStyle，tbody td 读 style。
 * 列宽由各页面 columns 定义；此处仅同步 style → headerStyle。
 */
export function normalizeRegistryColumns(columns: QTableProps["columns"]): QTableProps["columns"] {
  if (!Array.isArray(columns)) return columns;
  return columns.map((col) => {
    if (!col || typeof col !== "object") return col;
    const entry = col as RegistryColumn;
    if (entry.headerStyle != null && entry.headerStyle !== "") return entry;
    if (typeof entry.style === "string" && entry.style !== "") {
      return { ...entry, headerStyle: entry.style };
    }
    return entry;
  });
}

/**
 * 列宽 helper：同时写入 style + headerStyle（Quasar 表头/表体各读一处）。
 * @param width CSS width 值，如 "14%" 或 "108px"；含 ":" 时视为完整 CSS 片段
 */
export function registryColWidth(width: string) {
  const css = width.includes(":") ? width : `width: ${width}`;
  return { style: css, headerStyle: css };
}

/**
 * 统一列定义写法：name → label → field → align → width → 其它属性。
 */
export function registryCol<T extends Record<string, any>>(
  name: string,
  label: string,
  field: RegistryTableColumn<T>["field"],
  align: NonNullable<RegistryTableColumn<T>["align"]>,
  width: string,
  extra?: Omit<RegistryTableColumn<T>, "name" | "label" | "field" | "align" | "style" | "headerStyle">
): RegistryTableColumn<T> {
  return { name, label, field, align, ...registryColWidth(width), ...extra };
}

/** 操作列 preset：右对齐 + sticky 类名（sessions 等特殊表可复用） */
export function registryColActions<T extends Record<string, any>>(
  width: string = REGISTRY_COL_W.actions,
  label = "操作",
  field: RegistryTableColumn<T>["field"] = "id"
) {
  return registryCol<T>("actions", label, field, "right", width, {
    sortable: false,
    classes: "app-registry-col-actions",
    headerClasses: "app-registry-col-actions"
  });
}

/** Toggle 列 preset：居中 + 固定宽度 */
export function registryColEnabled<T extends Record<string, any>>(
  label = "启用",
  field: RegistryTableColumn<T>["field"] = "enabled"
) {
  return registryCol<T>("enabled", label, field, "center", REGISTRY_COL_W.enabled, { sortable: false });
}

export function registryFieldValue(row: Record<string, unknown>, field: RegistryTableColumn["field"]): unknown {
  if (typeof field === "function") return field(row);
  if (typeof field === "string") return row[field];
  return "";
}
