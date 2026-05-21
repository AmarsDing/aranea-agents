import { computed, ref, type ComputedRef, type Ref } from "vue";
import { resolveA2UIChildIds } from "../a2uiChildren";
import { resolveActionContext, resolveA2UIBind } from "../a2uiBind";
import type { A2UISurfaceState } from "../a2uiSurfaceState";
import { buildUserActionPayload, type A2UIUserActionPayload } from "../a2uiUserAction";

export type A2UIComponentEmit = (payload: A2UIUserActionPayload) => void;

export function mapA2UIIcon(name: string): string {
  const map: Record<string, string> = {
    arrowBack: "arrow_back",
    arrowForward: "arrow_forward",
    moreVert: "more_vert",
    moreHoriz: "more_horiz",
    shoppingCart: "shopping_cart",
    calendarToday: "event",
    locationOn: "place",
    accountCircle: "account_circle",
  };
  return map[name] ?? (name.replace(/([A-Z])/g, "_$1").toLowerCase().replace(/^_/, "") || "help");
}

export function useA2UIComponent(
  componentId: Ref<string> | ComputedRef<string>,
  surface: Ref<A2UISurfaceState> | ComputedRef<A2UISurfaceState>,
  emitUserAction: A2UIComponentEmit
) {
  const modalOpen = ref(false);
  const activeTab = ref(0);

  const comp = computed(() => surface.value.components[componentId.value]);
  const kind = computed(() => {
    const c = comp.value?.component;
    if (!c) return "";
    return Object.keys(c)[0] ?? "";
  });
  const payload = computed(() => {
    const k = kind.value;
    if (!k || !comp.value) return null;
    return comp.value.component[k] as Record<string, unknown> | undefined;
  });

  const isContainerKind = computed(() => kind.value === "Row" || kind.value === "Column");
  const wrapperClass = computed(() => {
    if (kind.value === "Row") return "row q-gutter-sm items-center";
    if (kind.value === "Column") return "column q-gutter-sm";
    return "";
  });
  const wrapperStyle = computed(() => {
    const w = comp.value?.weight;
    if (w != null && isContainerKind.value) return { flex: String(w) };
    return undefined;
  });

  const childIds = computed((): string[] =>
    resolveA2UIChildIds(payload.value?.children, surface.value)
  );
  const listClass = computed(() => {
    const dir = String(payload.value?.direction ?? "vertical");
    return dir === "horizontal" ? "row q-gutter-sm items-center" : "column q-gutter-sm";
  });

  const cardChildId = computed(() => String(payload.value?.child ?? "").trim());
  const modalEntryId = computed(() => String(payload.value?.entryPointChild ?? "").trim());
  const modalContentId = computed(() => String(payload.value?.contentChild ?? "").trim());

  const tabItems = computed(() => {
    const items = payload.value?.tabItems;
    if (!Array.isArray(items)) return [] as { label: string; childId: string }[];
    return items.map((item) => {
      const row = item as Record<string, unknown>;
      return {
        label: String(resolveA2UIBind(row.title, surface.value.dataModel) ?? "Tab"),
        childId: String(row.child ?? "").trim(),
      };
    });
  });

  const dividerVertical = computed(() => String(payload.value?.axis ?? "horizontal") === "vertical");

  const textContent = computed(() =>
    String(resolveA2UIBind(payload.value?.text, surface.value.dataModel) ?? "")
  );
  const textTag = computed(() => {
    const hint = String(payload.value?.usageHint ?? "body");
    if (hint.startsWith("h") && hint.length === 2) return hint;
    return "p";
  });
  const textClass = computed(() =>
    String(payload.value?.usageHint ?? "body") === "caption" ? "text-caption" : ""
  );

  const buttonPrimary = computed(() => Boolean(payload.value?.primary));
  const actionName = computed(() =>
    String((payload.value?.action as Record<string, unknown>)?.name ?? "")
  );
  const buttonChildId = computed(() => String(payload.value?.child ?? ""));
  const buttonLabel = computed(() => {
    const child = surface.value.components[buttonChildId.value];
    if (!child) return actionName.value || "Button";
    const textKey = Object.keys(child.component).find((k) => k === "Text");
    if (!textKey) return actionName.value || "Button";
    const textPayload = child.component[textKey] as Record<string, unknown>;
    return String(
      resolveA2UIBind(textPayload?.text, surface.value.dataModel) ?? actionName.value ?? "Button"
    );
  });

  const imageUrl = computed(() =>
    String(resolveA2UIBind(payload.value?.url, surface.value.dataModel) ?? "")
  );
  const iconName = computed(() =>
    String(resolveA2UIBind(payload.value?.name, surface.value.dataModel) ?? "")
  );
  const textFieldLabel = computed(() =>
    String(resolveA2UIBind(payload.value?.label, surface.value.dataModel) ?? "")
  );
  const textFieldValue = computed(() =>
    String(resolveA2UIBind(payload.value?.text, surface.value.dataModel) ?? "")
  );
  const textFieldInputType = computed(() => {
    const ft = String(payload.value?.textFieldType ?? "shortText");
    if (ft === "obscured") return "password";
    if (ft === "number") return "number";
    if (ft === "longText") return "textarea";
    return "text";
  });
  const checkBoxLabel = computed(() =>
    String(resolveA2UIBind(payload.value?.label, surface.value.dataModel) ?? "")
  );
  const checkBoxChecked = computed(() =>
    Boolean(resolveA2UIBind(payload.value?.value, surface.value.dataModel))
  );

  function onButtonClick() {
    if (!actionName.value || !surface.value.surfaceId) return;
    const action = payload.value?.action as Record<string, unknown> | undefined;
    const context = resolveActionContext(action?.context, surface.value.dataModel);
    emitUserAction(
      buildUserActionPayload({
        name: actionName.value,
        surfaceId: surface.value.surfaceId,
        sourceComponentId: componentId.value,
        context,
      })
    );
  }

  return {
    modalOpen,
    activeTab,
    kind,
    comp,
    isContainerKind,
    wrapperClass,
    wrapperStyle,
    childIds,
    listClass,
    cardChildId,
    modalEntryId,
    modalContentId,
    tabItems,
    dividerVertical,
    textContent,
    textTag,
    textClass,
    buttonPrimary,
    actionName,
    buttonLabel,
    imageUrl,
    iconName,
    textFieldLabel,
    textFieldValue,
    textFieldInputType,
    checkBoxLabel,
    checkBoxChecked,
    onButtonClick,
  };
}
