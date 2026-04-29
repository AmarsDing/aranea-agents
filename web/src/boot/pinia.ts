import { defineBoot } from "#q-app/wrappers";

/** Pinia 由 `src/stores/index` 的 default export 在 Quasar 管线中 `app.use`；此 boot 仅占位以保持 boot 顺序。 */
export default defineBoot(() => {});
