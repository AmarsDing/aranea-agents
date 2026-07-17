# Windows 安装包与静默运行方案（2026-07-15，更新 2026-07-17）

## 问题

1. 桌面快捷方式指向 `start.bat` → 黑框 + `pause`
2. 安装到 Program Files → UAC / 写权限问题
3. Electron 跨端口 API → Cookie/CORS 脆弱
4. Launcher 单实例互斥竞态：二次点击跳过 Postgres，后端连不上 `:5433`
5. 未复用本机已安装的 PostgreSQL(:5432) / Redis
6. `pg_ctl -w` 在 Windows 捕获管道时挂起
7. 日志/预检中文乱码（UTF-8 无 BOM，记事本按 ANSI/GBK 打开）
8. 缺少可见的环境引导（用户不知如何配系统 PG/Redis）

## 综合方案

| 环节 | 方案 |
|------|------|
| 安装目录 | `%LOCALAPPDATA%\AraneaAgents`（免管理员） |
| 启动入口 | `AraneaLauncher.exe`（`-H windowsgui`） |
| 状态控制台 | 启动时 `AllocConsole` + `WriteConsoleW`：环境检查 / DB / Redis / 配置引导 |
| 环境探测 | 优先系统 PG `:5432` + Redis `:6379`；否则内置实例 |
| pgvector | `CREATE EXTENSION vector`；失败则复制内置 `vector.dll` |
| 预检 | `logs\preflight.txt`（**UTF-8 BOM**）；失败弹窗；`-check` 打开控制台 |
| 配置 | 按探测结果重写 `configs/config.yaml` DSN（内容不变则跳过写盘） |
| 停止 | 只停 Aranea 托管的进程，不动系统 PG/Redis |
| 前端 | Electron 同源反代 API/WS → `:8000` |

### 乱码根因与修复面

| 位置 | 原因 | 修复 |
|------|------|------|
| `logs\launcher.log` / `preflight.txt` | Go 写 UTF-8 无 BOM，记事本按系统 ANSI 打开 | 写 **UTF-8 BOM**；分隔符改 ASCII `-` |
| NSIS 完成页中文 | `.nsi` 编码与构建机代码页不一致 | 完成页/快捷方式用 **英文**；`Unicode True` |
| `start.bat` / `start-silent.vbs` 中文 | bat/vbs 无代码页声明 | 改为英文提示 |
| MessageBox / 启动控制台 | 若用窄字符 API 会乱码 | `MessageBoxW` / `WriteConsoleW`（UTF-16） |

### 系统 PostgreSQL 密码

`configs\pg.password`（单行）或用户环境变量 `ARANEA_PG_PASSWORD`

## 启动加速方案

| 阶段 | 措施 | 预期收益 |
|------|------|----------|
| **P0（已落地）** | 后端已健康 → 快路径直接开桌面 | 二次启动约 1s 内 |
| **P0（已落地）** | Postgres 与 Redis **并行**启动 | 冷启动少 ~0.5–1s |
| **P0（已落地）** | 就绪轮询 200ms；Electron settle 300ms；探测超时缩短 | 少数百 ms |
| **P0（已落地）** | config 未变不重写 | 微优化 |
| **P1（后续）** | 后端冷启动：延后非关键 seed / 降低启动期自检 | 常可再省 1–3s |
| **P2（后续）** | 可选：安装后注册登录项 / 保持 PG+Redis 常驻 | 打开桌面接近秒开 |

说明：首次 `initdb` 仍需数秒，无法省略；之后主要瓶颈在 `aranea-server` 冷启动。

## 用户路径

1. 安装 `AraneaAgents-*-win-x64.exe`
2. 桌面快捷方式启动 → **状态控制台**显示检查与 DB/Redis 配置引导
3. 登录 `admin` / `changeme`
4. 排障：开始菜单「Environment Check」或查看 `logs\preflight.txt`（用记事本/VS Code）
5. 不要控制台：`AraneaLauncher.exe -no-console`

## 代码锚点

| 文件 | 作用 |
|------|------|
| `cmd/launcher/` | 启动器 + 探测 + 控制台 + 预检 |
| `cmd/launcher/status_console_windows.go` | AllocConsole + WriteConsoleW |
| `cmd/launcher/utf8file.go` | UTF-8 BOM 写盘 |
| `installer/aranea.nsi` | 英文完成页 + 用户目录安装 |
| `web/src-electron/electron-main.ts` | 同源反代 |
| `scripts/build-package.ps1` | 打包 |

## 验收

- [x] Launcher PE subsystem = GUI
- [x] v0.1.24+ 静默启动 / 环境探测
- [x] v0.1.28 修复安装期 `psql` 挂起
- [x] v0.1.29 修复 `pg_ctl -w` 挂起 + 完成页乱码
- [ ] 下一版：UTF-8 BOM 日志 + 启动控制台引导 + P0 加速
