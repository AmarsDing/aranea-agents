import { computed, type ComputedRef } from 'vue';
import { useI18n } from 'vue-i18n';
import type { ChannelPlatformField, ChannelPlatformSection, ChannelFieldHelp } from './channelPlatformFields';
import type { ChannelTypeItem } from './types';

export function useChannelEditorLabels(selectedCatalog: ComputedRef<ChannelTypeItem | null>) {
  const { t, te } = useI18n();

  const catalogDescription = computed(() => {
    const item = selectedCatalog.value;
    if (!item) return '';
    const key = `channelEditor.catalogDesc.${item.type}`;
    return te(key) ? t(key) : item.description;
  });

  function sectionTitle(section: ChannelPlatformSection): string {
    const key = `channelEditor.section.${section.id}.title`;
    return te(key) ? t(key) : section.title;
  }

  function sectionHint(section: ChannelPlatformSection): string {
    const raw = section.hint ?? '';
    if (!raw) return '';
    if (te(raw)) return t(raw);
    return raw;
  }

  function fieldLabel(field: ChannelPlatformField): string {
    const key = `channelEditor.fields.${field.museKey}.label`;
    return te(key) ? t(key) : field.museKey;
  }

  function fieldHelp(field: ChannelPlatformField): ChannelFieldHelp | undefined {
    const descKey = `channelEditor.fields.${field.museKey}.help`;
    if (te(descKey)) {
      const exampleKey = `channelEditor.fields.${field.museKey}.example`;
      return {
        description: t(descKey),
        example: te(exampleKey) ? t(exampleKey) : undefined,
      };
    }
    if (field.hint) {
      const hint = te(field.hint) ? t(field.hint) : field.hint;
      return { description: hint };
    }
    return undefined;
  }

  function fieldStatusLabel(statusKey: string): string {
    if (!statusKey) return '';
    const key = `channelEditor.status.${statusKey}`;
    return te(key) ? t(key) : statusKey;
  }

  function selectOptions(field: ChannelPlatformField) {
    if (field.bind.source === 'feishu' && field.bind.key === 'region') {
      return [
        { label: t('channelEditor.feishuRegion.feishu'), value: 'feishu' },
        { label: t('channelEditor.feishuRegion.lark'), value: 'lark' },
      ];
    }
    if (field.options?.length) {
      return field.options.map((opt) => {
        const i18nKey = `channelEditor.fields.${field.museKey}.options.${opt.value}`;
        if (te(i18nKey)) return { label: t(i18nKey), value: opt.value };
        if (te(opt.label)) return { label: t(opt.label), value: opt.value };
        return { label: opt.label, value: opt.value };
      });
    }
    return [];
  }

  function fieldPlaceholder(field: ChannelPlatformField): string {
    if (!field.placeholder) return '';
    if (te(field.placeholder)) return t(field.placeholder);
    return field.placeholder;
  }

  return {
    catalogDescription,
    sectionTitle,
    sectionHint,
    fieldLabel,
    fieldHelp,
    fieldStatusLabel,
    fieldPlaceholder,
    selectOptions,
    t,
    te,
  };
}
