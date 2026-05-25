import type { CatalogModelSummary } from "../../services/kratos/model_catalog/v1/index";
import { runtimeProfileFor } from "../../config/providerRuntimeOverlay";

export type CapabilityChip = {
  key: string;
  label: string;
  source: "catalog" | "custom";
};

export type CatalogCostForm = {
  input_usd_per_1m: number;
  output_usd_per_1m: number;
  cache_read_usd_per_1m: number;
  cache_write_usd_per_1m: number;
  reasoning_usd_per_1m: number;
  embedding_usd_per_1m: number;
};

export function buildCapabilityChips(model: CatalogModelSummary): CapabilityChip[] {
  const chips: CapabilityChip[] = [];
  const add = (key: string, label: string) => chips.push({ key, label, source: "catalog" });
  if (model.toolCall) add("tool_call", "工具调用");
  if (model.reasoning) add("reasoning", "推理");
  if (model.attachment) add("attachment", "附件");
  if (model.structuredOutput) add("structured_output", "结构化输出");
  if (model.temperature) add("temperature", "Temperature");
  if (model.openWeights) add("open_weights", "开放权重");
  const mods = [...(model.modalityInput ?? []), ...(model.modalityOutput ?? [])];
  if (mods.some((m) => m === "image" || m === "video")) add("vision", "视觉");
  if (model.status === "deprecated") add("deprecated", "已废弃");
  if (model.status === "beta") add("beta", "Beta");
  if (model.status === "alpha") add("alpha", "Alpha");
  return chips;
}

export function catalogModelToCost(model: CatalogModelSummary): CatalogCostForm {
  const reasoningUsd =
    (model.reasoningUsdPer1m ?? 0) > 0
      ? model.reasoningUsdPer1m ?? 0
      : model.reasoning
        ? model.outputUsdPer1m ?? 0
        : 0;
  return {
    input_usd_per_1m: model.inputUsdPer1m ?? 0,
    output_usd_per_1m: model.outputUsdPer1m ?? 0,
    cache_read_usd_per_1m: model.cacheReadUsdPer1m ?? 0,
    cache_write_usd_per_1m: model.cacheWriteUsdPer1m ?? 0,
    reasoning_usd_per_1m: reasoningUsd,
    embedding_usd_per_1m: 0,
  };
}

export type CatalogApplyTarget = {
  provider_code: string;
  provider_display_name: string;
  provider_type: string;
  variant: string;
  api_base_url: string;
  model_api_id: string;
  model_display_name: string;
  context_window_k: number | null;
  max_output_tokens: number;
  input_price_usd_per_1m: number;
  output_price_usd_per_1m: number;
  cache_read_usd_per_1m: number;
  cache_write_usd_per_1m: number;
  reasoning_price_usd_per_1m: number;
  embedding_price_usd_per_1m: number;
  capability_chips: CapabilityChip[];
  metadata_source: string;
  raw_metadata_json: string;
  catalog_managed: boolean;
};

export function applyCatalogProviderFields(
  providerId: string,
  providerName: string,
  catalogApi?: string
): Pick<
  CatalogApplyTarget,
  "provider_code" | "provider_display_name" | "provider_type" | "variant" | "api_base_url"
> {
  const rt = runtimeProfileFor(providerId);
  const base = (catalogApi || rt.apiBaseUrl || "").trim();
  return {
    provider_code: providerId,
    provider_display_name: providerName || providerId,
    provider_type: rt.providerType,
    variant: rt.variant ?? "openai",
    api_base_url: base,
  };
}

export function applyCatalogModelFields(
  providerId: string,
  model: CatalogModelSummary,
  overwrite = false
): Partial<CatalogApplyTarget> & { reasoning_backfill?: boolean } {
  const cost = catalogModelToCost(model);
  const ctxK = model.contextTokens ? Math.round(model.contextTokens / 1000) : null;
  const chips = buildCapabilityChips(model);
  let interleaved: unknown;
  if (model.interleavedJson?.trim()) {
    try {
      interleaved = JSON.parse(model.interleavedJson);
    } catch {
      interleaved = model.interleavedJson;
    }
  }
  const reasoningBackfill =
    model.reasoning && interleaved != null && typeof interleaved === "object" && interleaved !== null
      ? Boolean((interleaved as { field?: string }).field?.trim())
      : model.reasoning
        ? true
        : undefined;
  return {
    model_api_id: model.id ?? "",
    model_display_name: model.name || model.id || "",
    context_window_k: ctxK,
    max_output_tokens: model.outputTokens ? Number(model.outputTokens) : 4096,
    input_price_usd_per_1m: cost.input_usd_per_1m,
    output_price_usd_per_1m: cost.output_usd_per_1m,
    cache_read_usd_per_1m: cost.cache_read_usd_per_1m,
    cache_write_usd_per_1m: cost.cache_write_usd_per_1m,
    reasoning_price_usd_per_1m: cost.reasoning_usd_per_1m,
    embedding_price_usd_per_1m: cost.embedding_usd_per_1m,
    capability_chips: chips,
    metadata_source: "models.dev",
    raw_metadata_json: JSON.stringify({
      source: "models.dev",
      provider: providerId,
      model: {
        id: model.id,
        name: model.name,
        family: model.family,
        knowledge: model.knowledge,
        release_date: model.releaseDate,
        last_updated: model.lastUpdated,
        interleaved,
        reasoning: model.reasoning,
        tool_call: model.toolCall,
        open_weights: model.openWeights,
        cost: {
          input_usd_per_1m: cost.input_usd_per_1m,
          output_usd_per_1m: cost.output_usd_per_1m,
          cache_read_usd_per_1m: cost.cache_read_usd_per_1m,
          cache_write_usd_per_1m: cost.cache_write_usd_per_1m,
          reasoning_usd_per_1m: cost.reasoning_usd_per_1m,
        },
        limit: {
          context_tokens: model.contextTokens ?? 0,
          output_tokens: model.outputTokens ?? 0,
        },
        modalities: {
          input: model.modalityInput ?? [],
          output: model.modalityOutput ?? [],
        },
      },
      overwrite,
    }),
    catalog_managed: true,
    reasoning_backfill: reasoningBackfill,
  };
}
