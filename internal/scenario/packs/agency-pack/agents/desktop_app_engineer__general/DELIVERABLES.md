## 📋 你的技术交付物
### Electron：锁定窗口 + 类型化 IPC

```typescript
// main.ts —— 唯一接触 OS 的进程
const win = new BrowserWindow({
  webPreferences: {
    contextIsolation: true,        // renderer 拿到的是桥，不是你的内部
    nodeIntegration: false,        // web 内容里永远没有 require()
    sandbox: true,                 // Chromium OS 级沙盒
    preload: path.join(__dirname, 'preload.js'),
  },
});

// IPC：窄动词、校验输入、没有通用的文件系统/shell 透传
import { z } from 'zod';
const ExportRequest = z.object({
  format: z.enum(['csv', 'json']),
  projectId: z.string().uuid(),
});

ipcMain.handle('project:export', async (event, raw) => {
  const req = ExportRequest.parse(raw);                    // 在边界处拒绝垃圾输入
  const dest = await dialog.showSaveDialog(win, {          // 用户选择路径 —— 应用永不
    defaultPath: `export.${req.format}`,                   // 从 renderer 接受任意路径
  });
  if (dest.canceled) return { ok: false };
  await exportProject(req.projectId, req.format, dest.filePath);
  return { ok: true };
});
```

```typescript
// preload.ts —— renderer 将能看到的全部 API
import { contextBridge, ipcRenderer } from 'electron';
contextBridge.exposeInMainWorld('app', {
  exportProject: (req: unknown) => ipcRenderer.invoke('project:export', req),
  onUpdateReady: (cb: () => void) => ipcRenderer.on('update:ready', cb),
});
```

### Tauri：能力作用域命令（默认拒绝）

```rust
// src-tauri/src/main.rs —— commands 是整个攻击面；保持它们足够窄
#[tauri::command]
async fn export_project(project_id: String, format: String, state: tauri::State<'_, Db>)
    -> Result<ExportReceipt, String> {
    let format = Format::parse(&format).map_err(|e| e.to_string())?;   // 校验
    let id = Uuid::parse_str(&project_id).map_err(|_| "bad id")?;      // 一切都校验
    exporter::run(&state, id, format).await.map_err(|e| e.to_string())
}
```

```json
// src-tauri/capabilities/main.json —— 前端拿到的就这些，多一点都不给
{
  "identifier": "main-window",
  "windows": ["main"],
  "permissions": [
    "core:default",
    "dialog:allow-save",
    { "identifier": "fs:allow-write-file", "allow": [{ "path": "$APPDATA/exports/*" }] }
  ]
}
```

### 发布流水线：签名、公证、分阶段、回滚

```yaml
# release.yml —— 每个构建在用户看到之前都要跑完这道关
jobs:
  build-sign:
    strategy:
      matrix: { os: [macos-14, windows-2022, ubuntu-22.04] }
    steps:
      - run: npm run build && npm run package
      - name: 签名 (Windows)                       # 通过云 HSM 使用 EV/OV 证书 —— CI 里不放证书文件
        if: runner.os == 'Windows'
        run: azuresigntool sign -kvu $VAULT_URI -kvc $CERT_NAME -tr http://timestamp.digicert.com out/*.exe
      - name: 签名 + 公证 (macOS)              # 公证要求 hardened runtime
        if: runner.os == 'macOS'
        run: |
          codesign --deep --options runtime --entitlements entitlements.plist --sign "$IDENTITY" out/App.app
          xcrun notarytool submit out/App.dmg --keychain-profile ci --wait
          xcrun stapler staple out/App.dmg
  publish:
    needs: build-sign
    steps:
      - run: node scripts/publish-update.js --channel stable --rollout 1
        # 1% 持续 24h → 自动检查崩溃率 ≥ 99.5% → 10% → 100%
        # 回滚 = 重新发布上一个 manifest；N+1 上的客户端干净地降级
```

### Electron vs Tauri 决策表

| 关注点 | Electron | Tauri |
|---------|----------|-------|
| 安装包体积 | ~80–150MB（捆绑 Chromium） | ~3–15MB（系统 webview） |
| 空闲内存 | 较高 —— 每个应用自带 Chromium | 较低 —— 共享系统 webview |
| 渲染一致性 | 各处完全一致（你发布的就是浏览器） | 随 OS webview 变化（WebView2/WKWebView/WebKitGTK）—— 要测矩阵 |
| 特权侧语言 | Node.js（生态庞大，易招人） | Rust（内存安全，攻击面更小） |
| 生态成熟度 | 深厚：更新器、崩溃上报、原生模块 | 较年轻，演进快；逐个验证插件需求 |
| 何时选择 | 需要像素级渲染一致、重度原生模块需求、团队 JS 原生 | 体积/内存预算敏感、欢迎 Rust、webview 差异可测 |

### 资源占用预算（CI 强制）

| 指标 | 预算 | 度量方式 |
|--------|--------|-------------|
| 冷启动到可交互 | 在参考低端机上 < 2s | CI 中的启动 trace，10 次运行的 p95 |
| 空闲内存（所有进程） | Electron < 300MB / Tauri < 150MB | 启动后 5 分钟空闲采样 |
| 安装包体积 | 每次发布无声增长不超过 5% | 与上一版发布产物做 diff |
| 空闲时后台 CPU | ~0%（没有定时器让机器保持唤醒） | soak 测试中的 powerMetrics / ETW 采样 |
