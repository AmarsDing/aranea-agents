import { computed, type ComputedRef } from "vue";
import { useI18n } from "vue-i18n";
import type { ChannelPlatformField, ChannelPlatformSection } from "./channelPlatformFields";
import type { ChannelCatalogItem } from "./types";

export function useChannelEditorLabels(selectedCatalog: ComputedRef<ChannelCatalogItem | null>) {
  const { t, te } = useI18n();

  const catalogDescription = computed(() => {
    const item = selectedCatalog.value;
    if (!item) return "";
    const key = `channelEditor.catalogDesc.${item.type}`;
    return te(key) ? t(key) : item.description;
  });

  function sectionTitle(section: ChannelPlatformSection): string {
    const key = `channelEditor.section.${section.id}.title`;
    return te(key) ? t(key) : section.title;
  }

  function sectionHint(section: ChannelPlatformSection): string {
    const key = `channelEditor.section.${section.id}.hint`;
    if (te(key)) return t(key);
    return section.hint ?? "";
  }

  function fieldStatusLabel(statusKey: string): string {
    if (!statusKey) return "";
    const key = `channelEditor.status.${statusKey}`;
    return te(key) ? t(key) : statusKey;
  }

  function selectOptions(field: ChannelPlatformField) {
    if (field.bind.source === "feishu" && field.bind.key === "region") {
      return [
        { label: t("channelEditor.feishuRegion.feishu"), value: "feishu" },
        { label: t("channelEditor.feishuRegion.lark"), value: "lark" }
      ];
    }
    return field.options ?? [];
  }

  return { catalogDescription, sectionTitle, sectionHint, fieldStatusLabel, selectOptions, t, te };
}
