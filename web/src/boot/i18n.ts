import { defineBoot } from '#q-app/wrappers';
import { unref } from 'vue';
import { i18n } from '../i18n';
import { setQuasarLangFor } from '../i18n/quasar-lang';

export default defineBoot(async ({ app }) => {
  app.use(i18n);
  const loc = unref(i18n.global.locale);
  setQuasarLangFor(typeof loc === 'string' ? loc : 'zh-CN');
});
