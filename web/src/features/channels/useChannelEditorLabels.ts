import { computed, type ComputedRef } from "vue";
import { useI18n } from "vue-i18n";
import type { ChannelPlatformField, ChannelPlatformSection, ChannelFieldHelp } from "./channelPlatformFields";
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

  function fieldLabel(field: ChannelPlatformField): string {
    const key = `channelEditor.fields.${field.museKey}.label`;
    return te(key) ? t(key) : field.museKey;
  }

  function fieldHelp(field: ChannelPlatformField): ChannelFieldHelp | undefined {
    const descKey = `channelEditor.fields.${field.museKey}.help`;
    if (!te(descKey)) {
      if (field.hint) return { description: field.hint };
      return undefined;
    }
    const exampleKey = `channelEditor.fields.${field.museKey}.example`;
    return {
      description: t(descKey),
      example: te(exampleKey) ? t(exampleKey) : undefined
    };
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
    if (field.options?.length) {
      return field.options.map((opt) => {
        const i18nKey = `channelEditor.fields.${field.museKey}.options.${opt.value}`;
        return { label: te(i18nKey) ? t(i18nKey) : opt.label, value: opt.value };
      });
    }
    return [];
  }

  return {
    catalogDescription,
    sectionTitle,
    sectionHint,
    fieldLabel,
    fieldHelp,
    fieldStatusLabel,
    selectOptions,
    t,
    te
  };
}
