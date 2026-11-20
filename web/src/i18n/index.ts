import { createI18n } from 'vue-i18n';
import zh from './locales/zh-CN';
import en from './locales/en-US';
import { uiShellEn, uiShellZh } from './locales/ui-shell';
import { uiDomainsEn, uiDomainsZh } from './locales/ui-domains';
import { uiCapabilityEn, uiCapabilityZh } from './locales/ui-capability';
import { uiConnectEn, uiConnectZh } from './locales/ui-connect';
import { uiOpsEn, uiOpsZh } from './locales/ui-ops';
import { uiLibraryEn, uiLibraryZh } from './locales/ui-library';

const saved = typeof localStorage !== 'undefined' ? localStorage.getItem('locale') : null;
const defaultLocale = saved === 'en-US' || saved === 'zh-CN' ? saved : 'zh-CN';

export const i18n = createI18n({
  legacy: false,
  locale: defaultLocale,
  fallbackLocale: 'zh-CN',
  globalInjection: true,
  messages: {
    'zh-CN': {
      ...zh,
      shell: uiShellZh,
      domains: uiDomainsZh,
      connect: uiConnectZh,
      ops: uiOpsZh,
      cap: uiCapabilityZh,
      library: uiLibraryZh,
    },
    'en-US': {
      ...en,
      shell: uiShellEn,
      domains: uiDomainsEn,
      connect: uiConnectEn,
      ops: uiOpsEn,
      cap: uiCapabilityEn,
      library: uiLibraryEn,
    },
  },
});

export type AppLocale = 'zh-CN' | 'en-US';
