import { createI18n } from "vue-i18n";
import zh from "./locales/zh-CN";
import en from "./locales/en-US";

const saved = typeof localStorage !== "undefined" ? localStorage.getItem("locale") : null;
const defaultLocale = saved === "en-US" || saved === "zh-CN" ? saved : "zh-CN";

export const i18n = createI18n({
  legacy: false,
  locale: defaultLocale,
  fallbackLocale: "zh-CN",
  globalInjection: true,
  messages: {
    "zh-CN": zh,
    "en-US": en
  }
});

export type AppLocale = "zh-CN" | "en-US";
