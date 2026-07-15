# Windows 安装包与静默运行方案（2026-07-15）

## 问题

当前 Release（如 v0.1.23）安装后体验差、易不可用：

1. 桌面快捷方式指向 `start.bat` → 弹出黑色命令框 + `pause`，反人类
2. 安装到 `Program Files` → 每次启动 UAC / 写权限问题
3. Electron 页面在随机端口，API 直连 `:8000`，Cookie/CORS 边界脆弱，易出现登录失败

## 综合方案

| 环节 | 方案 |
|------|------|
| 安装目录 | 默认 `%LOCALAPPDATA%\AraneaAgents`，`RequestExecutionLevel user`（免管理员） |
| 启动入口 | `AraneaLauncher.exe`（`-H windowsgui`，无控制台） |
| 桌面/开始菜单 | 指向 Launcher；另提供「调试控制台」→ `start.bat` |
| 停止 | `AraneaLauncher.exe -stop` |
| 运行时 | PG → Redis → `aranea-server` → 健康检查 → Electron |
| 前端通信 | Electron 本地 HTTP + 反代 `/v1` `/api` `/healthz` `/v1/ws` → `:8000`（同源 Cookie） |
| 故障 | MessageBox + `logs\launcher.log` / `server.log` |

## 用户路径

1. 下载 `AraneaAgents-*-win-x64.exe` → 安装（无需管理员）
2. 双击桌面「Aranea-Agents」→ 无黑框，等待窗口出现（首次 10~30s）
3. 登录 `admin` / `changeme`
4. 退出：开始菜单「停止 Aranea-Agents」

## 代码锚点

| 文件 | 作用 |
|------|------|
| `cmd/launcher/` | 静默启动器 |
| `installer/aranea.nsi` | NSIS：用户目录 + Launcher 快捷方式 |
| `installer/scripts/start.bat` | 调试入口（有 Launcher 时委托之） |
| `web/src-electron/electron-main.ts` | 同源反代 |
| `scripts/build-package.ps1` | 打包纳入 Launcher |
| `.github/workflows/release.yml` | CI 构建 Launcher 并更新 Release 说明 |

## 验收

- [x] `AraneaLauncher.exe` PE subsystem = GUI (2)
- [ ] CI Release 产出新版本安装包
- [ ] 安装后桌面快捷方式目标为 `AraneaLauncher.exe`
- [ ] 双击无控制台黑框，桌面窗口可登录
