import { createI18n } from 'vue-i18n';
import zh from './locales/zh-CN';
import en from './locales/en-US';
import { uiShellEn, uiShellZh } from './locales/ui-shell';
import { uiDomainsEn, uiDomainsZh } from './locales/ui-domains';

const saved = typeof localStorage !== 'undefined' ? localStorage.getItem('locale') : null;
const defaultLocale = saved === 'en-US' || saved === 'zh-CN' ? saved : 'zh-CN';

export const i18n = createI18n({
  legacy: false,
  locale: defaultLocale,
  fallbackLocale: 'zh-CN',
  globalInjection: true,
  messages: {
    'zh-CN': { ...zh, shell: uiShellZh, domains: uiDomainsZh },
    'en-US': { ...en, shell: uiShellEn, domains: uiDomainsEn },
  },
});

export type AppLocale = 'zh-CN' | 'en-US';
