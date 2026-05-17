import { Quasar } from "quasar";
import quasarLangEn from "quasar/lang/en-US";
import quasarLangZh from "quasar/lang/zh-CN";

export function setQuasarLangFor(code: string) {
  if (code === "en-US") {
    Quasar.lang.set(quasarLangEn);
  } else {
    Quasar.lang.set(quasarLangZh);
  }
}
