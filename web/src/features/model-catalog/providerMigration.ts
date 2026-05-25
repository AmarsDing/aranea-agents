import { getProviderMigrationRules } from "./api";

/** Fallback when API unavailable (mirrors internal/modelcatalog/overlay.go). */
const FALLBACK_LEGACY_TO_CATALOG: Record<string, string> = {
  "aliyun-qwen": "alibaba-cn",
  "tencent-hunyuan": "hunyuan",
  "moonshot-kimi": "moonshotai-cn",
  "zhipu-glm": "zhipuai",
  gemini: "google",
};

let migrationMapCache: Record<string, string> | null = null;
let migrationLoadPromise: Promise<Record<string, string>> | null = null;

/** Load provider migration rules from server (cached). */
export async function ensureProviderMigrationMap(): Promise<Record<string, string>> {
  if (migrationMapCache) return migrationMapCache;
  if (migrationLoadPromise) return migrationLoadPromise;
  migrationLoadPromise = (async () => {
    const map: Record<string, string> = { ...FALLBACK_LEGACY_TO_CATALOG };
    try {
      const res = await getProviderMigrationRules();
      for (const rule of res.rules ?? []) {
        const legacy = rule.legacy?.trim();
        const catalog = rule.catalog?.trim();
        if (legacy && catalog) map[legacy] = catalog;
      }
    } catch {
      /* use fallback */
    }
    migrationMapCache = map;
    return map;
  })();
  return migrationLoadPromise;
}

/** Map legacy provider_code to models.dev catalog provider id. */
export function catalogProviderIdFor(providerCode: string, map?: Record<string, string>): string {
  const code = providerCode.trim();
  if (!code) return "";
  const rules = map ?? migrationMapCache ?? FALLBACK_LEGACY_TO_CATALOG;
  return rules[code] ?? code;
}

/** Reset cache (e.g. after apply-migration). */
export function resetProviderMigrationCache() {
  migrationMapCache = null;
  migrationLoadPromise = null;
}
