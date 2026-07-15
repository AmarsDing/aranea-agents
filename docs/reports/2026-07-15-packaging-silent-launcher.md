# Windows 安装包与静默运行方案（2026-07-15）

## 问题

1. 桌面快捷方式指向 `start.bat` → 黑框 + `pause`
2. 安装到 Program Files → UAC / 写权限问题
3. Electron 跨端口 API → Cookie/CORS 脆弱
4. Launcher 单实例互斥竞态：二次点击跳过 Postgres，后端连不上 `:5433`
5. 未复用本机已安装的 PostgreSQL(:5432) / Redis

## 综合方案

| 环节 | 方案 |
|------|------|
| 安装目录 | `%LOCALAPPDATA%\AraneaAgents`（免管理员） |
| 启动入口 | `AraneaLauncher.exe`（无控制台） |
| 环境探测 | 优先系统 PG `:5432` + Redis `:6379`；否则内置实例 |
| pgvector | `CREATE EXTENSION vector`；失败则复制内置 `vector.dll` |
| 预检 | `logs\preflight.txt`；失败弹窗；警告弹窗后继续；`-check` 专用检查 |
| 配置 | 按探测结果重写 `configs/config.yaml` DSN |
| 停止 | 只停 Aranea 托管的进程，不动系统 PG/Redis |
| 前端 | Electron 同源反代 API/WS → `:8000` |

### 系统 PostgreSQL 密码

`ARANEA_PG_PASSWORD=你的密码`（用户环境变量）

## 用户路径

1. 安装 `AraneaAgents-*-win-x64.exe`
2. 安装结束会弹出「环境检查」
3. 桌面快捷方式启动（无黑框）
4. 登录 `admin` / `changeme`
5. 排障：开始菜单「环境检查」或查看 `logs\preflight.txt`

## 代码锚点

| 文件 | 作用 |
|------|------|
| `cmd/launcher/` | 启动器 + 环境探测 + 预检 |
| `installer/aranea.nsi` | 快捷方式 + 安装后 `-check` |
| `web/src-electron/electron-main.ts` | 同源反代 |
| `scripts/build-package.ps1` | 打包 |

## 验收

- [x] Launcher PE subsystem = GUI
- [x] v0.1.24 发布（静默启动）
- [x] v0.1.25+ 环境探测 / 系统 PG 复用 / 预检弹窗
  - `cmd/launcher`：探测系统 PG `:5432` + Redis `:6379`，否则内置；`ARANEA_PG_PASSWORD`；`logs\preflight.txt`；失败/警告 MessageBox；`-check` / `-stop`
  - `installer/aranea.nsi`：`%LOCALAPPDATA%\AraneaAgents`；桌面/开始菜单 → `AraneaLauncher.exe`；安装后 `-check`；调试入口 `start.bat`
  - `web/src-electron/electron-main.ts`：同源反代 `/v1` `/api` `/healthz` `/v1/ws` → `:8000`
  - 打包：`scripts/build-package.ps1` / release.yml 以 `-H windowsgui` 编译 `AraneaLauncher.exe`
