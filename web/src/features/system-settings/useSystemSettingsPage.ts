import { useSystemSettingsStore } from "../../stores/system-settings";
import { useA2AStore } from "../../stores/a2a";

export function useSystemSettingsPage() {
  const settingsStore = useSystemSettingsStore();
  const a2aStore = useA2AStore();
  return { settingsStore, a2aStore };
}
