import { ref } from 'vue';

const STORAGE_KEY = 'aranea.mcp.guide.dismissed';

/** MCP 页顶部引导提示条的显隐控制；关闭状态持久化到 localStorage，避免反复打扰。 */
export function useMcpGuide() {
  const dismissed = readDismissed();
  const mcpGuideVisible = ref(!dismissed);

  function dismissMcpGuide() {
    mcpGuideVisible.value = false;
    try {
      localStorage.setItem(STORAGE_KEY, '1');
    } catch {
      /* 私密模式等写入失败时静默 */
    }
  }

  return { mcpGuideVisible, dismissMcpGuide };
}

function readDismissed(): boolean {
  try {
    return localStorage.getItem(STORAGE_KEY) === '1';
  } catch {
    return false;
  }
}
