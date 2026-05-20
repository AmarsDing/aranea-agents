import { createSystemSettingService } from "../../services/index";
import type { SystemSettings } from "../../services/kratos/system_setting/v1/index";

const api = createSystemSettingService();

export async function getSystemSettings(): Promise<SystemSettings> {
  return api.GetSystemSettings({});
}

export async function updateSystemSettings(
  rootDirectory: string,
  workDirectory: string,
  globalMonthlyMicroUsd = 0
): Promise<SystemSettings> {
  return api.UpdateSystemSettings({ rootDirectory, workDirectory, globalMonthlyMicroUsd });
}
