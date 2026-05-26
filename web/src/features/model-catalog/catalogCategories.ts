import type { CatalogModelSummary } from "../../services/kratos/model_catalog/v1/index";

export type ModelCategoryOption = {
  value: string;
  label: string;
  tooltip: string;
};

export const MODEL_CATEGORY_OPTIONS: ModelCategoryOption[] = [
  { value: "general", label: "通用对话", tooltip: "均衡，适合日常问答与轻任务" },
  { value: "reasoning", label: "推理 / 复杂问题", tooltip: "数学、逻辑、多步推导" },
  { value: "code", label: "代码", tooltip: "生成、解释、重构代码" },
  { value: "long_context", label: "长上下文", tooltip: "大文档、长会话摘要" },
  { value: "vision", label: "视觉 / 多模态", tooltip: "图像理解" },
  { value: "embedding", label: "向量嵌入", tooltip: "记忆、检索" },
  { value: "fast", label: "低延迟", tooltip: "优先响应速度" },
  { value: "creative", label: "创意写作", tooltip: "文案、故事、营销" },
];

/** Infer model_category values from catalog model capabilities (models.dev). */
export function inferModelCategoryValues(model: CatalogModelSummary): string[] {
  const values = new Set<string>();
  if (model.reasoning) values.add("reasoning");
  if (model.toolCall) values.add("code");
  const ctx = model.contextTokens ?? 0;
  if (ctx >= 128_000) values.add("long_context");
  const mods = [...(model.modalityInput ?? []), ...(model.modalityOutput ?? [])];
  if (mods.some((m) => m === "image" || m === "video")) values.add("vision");
  if (mods.includes("embedding")) values.add("embedding");
  if (!values.size) values.add("general");
  return [...values];
}

export function modelCategoriesFromValues(values: string[]): ModelCategoryOption[] {
  const set = new Set(values);
  return MODEL_CATEGORY_OPTIONS.filter((c) => set.has(c.value));
}
