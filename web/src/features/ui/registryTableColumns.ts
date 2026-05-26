import type { QTableColumn, QTableProps } from "quasar";

type RegistryColumn = NonNullable<QTableProps["columns"]>[number];

/** AppRegistryTable / AppRegistryMarkupTable 统一列定义类型 */
export type RegistryTableColumn<T extends Record<string, any> = Record<string, any>> = QTableColumn<T>;

/** 常用列宽 token（与 ChannelsTable 等 registry 列表对齐） */
export const REGISTRY_COL_W = {
  name: "14%",
  nameWide: "18%",
  status: "9%",
  time: "11%",
  agent: "10%",
  session: "10%",
  enabled: "64px",
  metric: "72px",
  narrow: "64px",
  actions: "108px",
  actionsWide: "148px"
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
 * 在各 table 页面的 columns 定义里单独使用，勿放入全局样式。
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

export function registryFieldValue(row: Record<string, unknown>, field: RegistryTableColumn["field"]): unknown {
  if (typeof field === "function") return field(row);
  if (typeof field === "string") return row[field];
  return "";
}
