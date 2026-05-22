import { computed, reactive, ref, watch, type Ref } from "vue";
import { useQuasar } from "quasar";
import { listAgents } from "../agents/api";
import type { Agent } from "../agents/api";
import { listTeams } from "../teams/api";
import type { Team } from "../teams/api";
import { createChannel, testChannel, updateChannel } from "./api";
import {
  buildPlatformSections,
  visibleFields,
  type ChannelPlatformField,
  type ChannelPlatformSection
} from "./channelPlatformFields";
import {
  inferRoutingTargetType,
  isChannelRoutingValid,
  pickDefaultAgentId,
  resolveChannelAgentSelectValue,
  type ChannelRoutingTargetType
} from "./channelRoutingUtils";
import { channelWebhookURL } from "../../components/channels/channelUi";
import { buildChannelWebhookURL, isLocalhostOrigin } from "./publicWebhookOrigin";
import type { ChannelCatalogItem, ChannelConfig, ChannelCredential, ChannelCredentialInput, ChannelMetadata, ChannelRow } from "./types";

type EditorProps = {
  catalog: ChannelCatalogItem[];
  row: ChannelRow | null;
  credentials: ChannelCredential[];
};

export function useChannelEditorForm(props: EditorProps, modelOpen: Ref<boolean>, emit: {
  (e: "saved", row: ChannelRow): void;
  (e: "tested"): void;
  (e: "update:modelValue", value: boolean): void;
}) {
  const $q = useQuasar();
  const saving = ref(false);
  const testing = ref(false);
  const selectedType = ref("");
  const receiveMode = ref("webhook");
  const webhookPath = ref("");
  const defaultAgentId = ref("");
  const defaultTeamId = ref("");
  const dmScope = ref("per-channel-peer");
  const routingTargetType = ref<ChannelRoutingTargetType>("agent");
  const routingOptionsLoading = ref(false);
  const routingAgents = ref<Agent[]>([]);
  const routingTeams = ref<Team[]>([]);
  const externalId = ref("");
  const publicWebhookOrigin = ref("");
  const iconAssetId = ref("");
  const iconPickerOpen = ref(false);
  const feishuAppId = ref("");
  const feishuRegion = ref("feishu");
  const showSecrets = ref(false);
  const configExtraText = ref("{}");
  const metadataExtraText = ref("{}");
  const configDraft = reactive<Record<string, string>>({});
  const configBoolDraft = reactive<Record<string, boolean>>({});
  const credentialDraft = reactive<Record<string, string>>({});
  const form = reactive({ key: "", name: "", description: "", enabled: true });

  const selectedCatalog = computed(() => props.catalog.find((item) => item.type === selectedType.value) ?? null);
  const platformSections = computed(() => buildPlatformSections(selectedType.value, selectedCatalog.value));
  const credentialKeys = computed(() => selectedCatalog.value?.credential_schema?.required ?? []);
  const configError = computed(() => jsonError(configExtraText.value));
  const metadataError = computed(() => jsonError(metadataExtraText.value));
  const feishuSecretReady = computed(() => {
    if (selectedType.value !== "feishu") return true;
    const configured = props.credentials.some((item) => item.credential_key === "app_secret" && item.configured);
    if (configured) return true;
    return Boolean(credentialDraft.app_secret?.trim());
  });
  const canSave = computed(() => {
    const base = Boolean(form.key.trim() && form.name.trim() && selectedType.value && !configError.value && !metadataError.value);
    if (!base) return false;
    if (!isChannelRoutingValid(routingTargetType.value, defaultAgentId.value, defaultTeamId.value)) return false;
    if (selectedType.value === "feishu") {
      return Boolean(feishuAppId.value.trim()) && feishuSecretReady.value;
    }
    return true;
  });
  const webhookPreview = computed(() => {
    if (!selectedCatalog.value?.supports_webhook) return "";
    const path = webhookPath.value.trim();
    const normalized = path ? (path.startsWith("/") ? path : `/${path}`) : (form.key.trim() ? `/webhooks/${form.key.trim()}` : "");
    if (!normalized) return "";
    return buildChannelWebhookURL(normalized, { public_webhook_origin: publicWebhookOrigin.value });
  });
  const webhookIsLocalhost = computed(() => isLocalhostOrigin(publicWebhookOrigin.value || (typeof window !== "undefined" ? window.location.origin : "")));
  const iconPreviewMetadata = computed(() => ({ icon_asset_id: iconAssetId.value || undefined }));

  watch(modelOpen, (open) => {
    if (!open) return;
    resetForm();
    void loadRoutingOptions();
  });
  watch(selectedType, (type, previousType) => {
    const item = props.catalog.find((entry) => entry.type === type);
    if (!item) return;
    if (!receiveMode.value || !item.receive_modes.includes(receiveMode.value)) {
      receiveMode.value = item.receive_modes[0] || "webhook";
    }
    if (!props.row) applyCatalogDefaults(item, previousType);
  });

  function visibleSectionFields(section: ChannelPlatformSection) {
    return visibleFields(section, receiveMode.value).filter((field) => {
      if (field.bind.source === "icon" && section.id !== "avatar") return false;
      if (field.bind.source === "icon" && section.id === "avatar") return true;
      if (section.id === "avatar" && field.bind.source !== "icon") return false;
      return true;
    });
  }

  function fieldKind(field: ChannelPlatformField) {
    if (field.kind === "textarea") return "textarea";
    if (field.kind === "password" || (field.kind === "text" && field.bind.source === "credential")) {
      return showSecrets.value ? "text" : "password";
    }
    return field.kind;
  }

  function readField(field: ChannelPlatformField): string {
    const bind = field.bind;
    switch (bind.source) {
      case "form": return String(form[bind.key] ?? "");
      case "credential": return credentialDraft[bind.key] ?? "";
      case "config": return configDraft[bind.key] ?? "";
      case "feishu": return bind.key === "app_id" ? feishuAppId.value : feishuRegion.value;
      case "routing": return externalId.value;
      case "webhook":
        if (bind.key === "receive_mode") return receiveMode.value;
        if (bind.key === "path") return webhookPath.value;
        if (bind.key === "public_origin") return publicWebhookOrigin.value;
        return webhookPreview.value;
      case "icon": return iconAssetId.value;
      default: return "";
    }
  }

  function writeField(field: ChannelPlatformField, value: string) {
    const bind = field.bind;
    switch (bind.source) {
      case "form":
        if (bind.key === "enabled") form.enabled = value === "true";
        else (form as Record<string, string | boolean>)[bind.key] = value;
        break;
      case "credential": credentialDraft[bind.key] = value; break;
      case "config": configDraft[bind.key] = value; break;
      case "feishu":
        if (bind.key === "app_id") feishuAppId.value = value;
        else feishuRegion.value = value;
        break;
      case "routing":
        externalId.value = value;
        break;
      case "webhook":
        if (bind.key === "receive_mode") receiveMode.value = value;
        else if (bind.key === "path") webhookPath.value = value;
        else if (bind.key === "public_origin") publicWebhookOrigin.value = value;
        break;
    }
  }

  function readFieldBool(field: ChannelPlatformField): boolean {
    if (field.bind.source === "form" && field.bind.key === "enabled") return form.enabled;
    if (field.bind.source === "config") return Boolean(configBoolDraft[field.bind.key]);
    return false;
  }

  function writeFieldBool(field: ChannelPlatformField, value: boolean) {
    if (field.bind.source === "form" && field.bind.key === "enabled") { form.enabled = value; return; }
    if (field.bind.source === "config") configBoolDraft[field.bind.key] = value;
  }

  function fieldStatus(field: ChannelPlatformField): string {
    if (field.bind.source === "credential") {
      const existing = props.credentials.find((item) => item.credential_key === field.bind.key);
      if (credentialDraft[field.bind.key]?.trim()) return "pendingSave";
      if (existing?.configured) return "configured";
      return field.required ? "required" : "";
    }
    if (field.bind.source === "webhook" && field.bind.key === "preview" && webhookPreview.value) return "copyable";
    return "";
  }

  function resetForm() {
    const row = props.row;
    const cfg = parseJSON<ChannelConfig>(row?.config_json, {});
    const metadata = parseJSON<ChannelMetadata>(row?.metadata_json, {});
    selectedType.value = cfg.type || props.catalog[0]?.type || "";
    const item = props.catalog.find((entry) => entry.type === selectedType.value);
    receiveMode.value = cfg.receive_mode || item?.receive_modes[0] || "webhook";
    webhookPath.value = String(cfg.webhook?.path ?? "");
    routingTargetType.value = inferRoutingTargetType(cfg.routing);
    defaultTeamId.value = String(cfg.routing?.default_team_id ?? "").trim();
    defaultAgentId.value = String(cfg.routing?.default_agent_id ?? "").trim();
    dmScope.value = String(cfg.routing?.dm_scope ?? "per-channel-peer").trim() || "per-channel-peer";
    externalId.value = metadata.external_id || "";
    publicWebhookOrigin.value = metadata.public_webhook_origin || "";
    iconAssetId.value = metadata.icon_asset_id || "";
    form.key = row?.key || selectedType.value;
    form.name = row?.name || item?.label || "";
    form.description = row?.description || item?.description || "";
    form.enabled = row?.enabled ?? true;
    loadPlatformConfigFields(cfg, metadata);
    resetCredentialDraft();
    if (!row && item) applyCatalogDefaults(item, "");
  }

  function loadPlatformConfigFields(cfg: ChannelConfig, metadata: ChannelMetadata) {
    const platformConfig = { ...(cfg.config || {}) };
    Object.keys(configDraft).forEach((k) => delete configDraft[k]);
    Object.keys(configBoolDraft).forEach((k) => delete configBoolDraft[k]);
    if (selectedType.value === "feishu") {
      feishuAppId.value = String(platformConfig.app_id ?? "").trim();
      if (!feishuAppId.value && /^cli_/i.test(metadata.external_id || "")) {
        feishuAppId.value = metadata.external_id!.trim();
      }
      feishuRegion.value = String(platformConfig.region ?? "feishu").trim() || "feishu";
      delete platformConfig.app_id;
      delete platformConfig.region;
    } else {
      feishuAppId.value = "";
      feishuRegion.value = "feishu";
    }
    for (const key of ["client_id", "corp_id", "agent_id", "app_id", "onebot_http_server", "allowed_user_ids", "allowed_group_ids", "active_mode", "require_mention"]) {
      if (platformConfig[key] === undefined || platformConfig[key] === null) continue;
      if (key === "active_mode" || key === "require_mention") configBoolDraft[key] = Boolean(platformConfig[key]);
      else if (typeof platformConfig[key] === "object") configDraft[key] = JSON.stringify(platformConfig[key]);
      else configDraft[key] = String(platformConfig[key]);
      delete platformConfig[key];
    }
    configExtraText.value = JSON.stringify(platformConfig, null, 2);
    metadataExtraText.value = JSON.stringify({
      ...metadata,
      icon_url: undefined,
      icon_asset_id: undefined,
      public_webhook_origin: undefined,
      external_id: undefined
    }, null, 2);
  }

  function applyCatalogDefaults(item: ChannelCatalogItem, previousType: string | undefined) {
    form.key = item.type;
    form.name = item.label;
    form.description = item.description;
    if (!item.receive_modes.includes(receiveMode.value)) receiveMode.value = item.receive_modes[0] || "webhook";
    webhookPath.value = item.supports_webhook ? `/webhooks/${form.key.trim() || item.type}` : "";
    externalId.value = "";
    publicWebhookOrigin.value = "";
    iconAssetId.value = "";
    routingTargetType.value = "agent";
    dmScope.value = "per-channel-peer";
    defaultAgentId.value = "";
    defaultTeamId.value = "";
    feishuAppId.value = "";
    feishuRegion.value = "feishu";
    Object.keys(configDraft).forEach((k) => delete configDraft[k]);
    Object.keys(configBoolDraft).forEach((k) => delete configBoolDraft[k]);
    configExtraText.value = JSON.stringify(defaultConfigFor(item), null, 2);
    metadataExtraText.value = JSON.stringify({ catalog_source: "catalog", catalog_group: item.group }, null, 2);
    if (previousType !== item.type) resetCredentialDraft();
  }

  function resetCredentialDraft() {
    Object.keys(credentialDraft).forEach((key) => delete credentialDraft[key]);
    credentialKeys.value.forEach((key) => { credentialDraft[key] = ""; });
    for (const key of ["client_secret", "encoding_aes_key", "corp_secret", "app_token", "token", "receive_token", "send_token"]) {
      if (!(key in credentialDraft)) credentialDraft[key] = "";
    }
  }

  function mergeConfigDraft(extra: Record<string, unknown>) {
    Object.entries(configDraft).forEach(([key, raw]) => {
      const trimmed = raw.trim();
      if (!trimmed) return;
      if (key === "allowed_user_ids" || key === "allowed_group_ids") {
        try { extra[key] = JSON.parse(trimmed); } catch { extra[key] = trimmed.split(",").map((s) => s.trim()).filter(Boolean); }
        return;
      }
      extra[key] = trimmed;
    });
    if ("active_mode" in configBoolDraft) extra.active_mode = configBoolDraft.active_mode;
    if ("require_mention" in configBoolDraft) extra.require_mention = configBoolDraft.require_mention;
  }

  function buildPayload() {
    const extraConfig = parseJSON<Record<string, unknown>>(configExtraText.value, {});
    mergeConfigDraft(extraConfig);
    if (selectedType.value === "feishu") {
      extraConfig.app_id = feishuAppId.value.trim();
      extraConfig.region = feishuRegion.value;
    }
    const config: ChannelConfig = {
      type: selectedType.value,
      receive_mode: receiveMode.value,
      webhook: { path: webhookPath.value },
      routing: {
        dm_scope: dmScope.value.trim() || "per-channel-peer",
        ...(routingTargetType.value === "team"
          ? { default_team_id: defaultTeamId.value.trim() }
          : { default_agent_id: defaultAgentId.value.trim() }),
      },
      config: extraConfig,
      accounts: []
    };
    const metadata: ChannelMetadata = {
      ...parseJSON<Record<string, unknown>>(metadataExtraText.value, {}),
      icon_asset_id: iconAssetId.value.trim() || undefined,
      public_webhook_origin: publicWebhookOrigin.value.trim() || undefined,
      external_id: selectedType.value === "feishu" ? undefined : externalId.value.trim() || undefined,
      catalog_group: selectedCatalog.value?.group,
      schema_version: 1
    };
    const credentials: ChannelCredentialInput[] = Object.entries(credentialDraft)
      .filter(([, secret]) => secret.trim())
      .map(([credential_key, secret]) => ({ credential_key, secret: secret.trim(), metadata_json: "{}" }));
    return {
      key: form.key.trim(),
      name: form.name.trim(),
      description: form.description.trim(),
      enabled: form.enabled,
      config_json: JSON.stringify(config),
      metadata_json: JSON.stringify(metadata),
      credentials
    };
  }

  async function persistChannel() {
    const payload = buildPayload();
    return props.row ? updateChannel(props.row.id, payload) : createChannel(payload);
  }

  async function save() {
    saving.value = true;
    try {
      const saved = await persistChannel();
      emit("saved", saved);
      emit("update:modelValue", false);
      $q.notify({ type: "positive", message: "Channel 已保存" });
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "保存失败" });
    } finally {
      saving.value = false;
    }
  }

  async function saveAndTest() {
    testing.value = true;
    try {
      const saved = await persistChannel();
      emit("saved", saved);
      const result = await testChannel(saved.id);
      emit("tested");
      emit("update:modelValue", false);
      $q.notify({ type: result.ok ? "positive" : "warning", message: result.message || result.status });
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "保存或测试失败" });
    } finally {
      testing.value = false;
    }
  }

  async function copyWebhookPreview() {
    const previewRow: ChannelRow = {
      id: props.row?.id || "",
      key: form.key.trim(),
      name: form.name.trim(),
      config_json: JSON.stringify({ type: selectedType.value, receive_mode: receiveMode.value, webhook: { path: webhookPath.value } }),
      metadata_json: "{}",
      enabled: form.enabled,
      status: props.row?.status || "active",
      resource: "channels",
      description: "",
      sort_order: 0,
      parent_id: "",
      level: "",
      agent_id: "",
      provider: "",
      model: "",
      created_at: "",
      updated_at: "",
      deleted_at: ""
    };
    try {
      const url = webhookPreview.value || channelWebhookURL(previewRow);
      if (typeof navigator !== "undefined" && navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(url);
      }
      $q.notify({ type: "positive", message: `已复制 Webhook URL：${url}` });
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "复制失败" });
    }
  }

  async function loadRoutingOptions() {
    routingOptionsLoading.value = true;
    try {
      const [agents, teams] = await Promise.all([listAgents({ limit: 200 }), listTeams()]);
      routingAgents.value = agents;
      routingTeams.value = teams;
      reconcileRoutingSelection(agents, teams);
    } catch (err) {
      $q.notify({
        type: "warning",
        message: err instanceof Error ? err.message : "加载 Agent / Team 列表失败"
      });
    } finally {
      routingOptionsLoading.value = false;
    }
  }

  function reconcileRoutingSelection(agents: Agent[], teams: Team[]) {
    if (routingTargetType.value === "team") {
      if (!defaultTeamId.value && teams[0]) defaultTeamId.value = teams[0].id;
      return;
    }
    defaultAgentId.value = resolveChannelAgentSelectValue(defaultAgentId.value, agents);
    if (!defaultAgentId.value) defaultAgentId.value = pickDefaultAgentId(agents);
  }

  return {
    saving, testing, selectedType, showSecrets, iconAssetId, iconPickerOpen,
    configExtraText, metadataExtraText, form,
    selectedCatalog, platformSections, credentialKeys,
    configError, metadataError, canSave, webhookPreview, webhookIsLocalhost, iconPreviewMetadata,
    routingTargetType, defaultAgentId, defaultTeamId, dmScope, routingAgents, routingTeams, routingOptionsLoading,
    visibleSectionFields, fieldKind, readField, writeField, readFieldBool, writeFieldBool, fieldStatus,
    save, saveAndTest, copyWebhookPreview
  };
}

function defaultConfigFor(item: ChannelCatalogItem): Record<string, unknown> {
  const base: Record<string, unknown> = {};
  if (item.type === "feishu") base.default_account = "default";
  else if (item.type === "wechat") base.subtype = "official";
  else if (item.type === "telegram") base.allowed_updates = ["message", "callback_query"];
  else if (item.type === "qq") base.protocol = "onebot11";
  return base;
}

function jsonError(value: string) {
  try { JSON.parse(value || "{}"); return ""; } catch (err) { return err instanceof Error ? err.message : "JSON 格式错误"; }
}

function parseJSON<T>(value: string | undefined, fallback: T): T {
  if (!value) return fallback;
  try { return JSON.parse(value) as T; } catch { return fallback; }
}

