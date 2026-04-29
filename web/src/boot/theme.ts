import { defineBoot } from "#q-app/wrappers";
import { Dark } from "quasar";

export default defineBoot(() => {
  if (import.meta.env.SSR) return;
  const raw = localStorage.getItem("theme");
  if (raw === "dark") {
    Dark.set(true);
  } else if (raw === "light") {
    Dark.set(false);
  } else {
    Dark.set(false);
  }
});
