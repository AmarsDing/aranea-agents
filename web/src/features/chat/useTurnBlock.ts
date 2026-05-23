const STORAGE_KEY = "feature_turn_block";

/** TurnBlock UI enabled by default; set localStorage feature_turn_block=0 to disable. */
export function useTurnBlockEnabled(): boolean {
  if (typeof localStorage === "undefined") return true;
  return localStorage.getItem(STORAGE_KEY) !== "0";
}

export function setTurnBlockEnabled(enabled: boolean) {
  if (typeof localStorage === "undefined") return;
  localStorage.setItem(STORAGE_KEY, enabled ? "1" : "0");
}
