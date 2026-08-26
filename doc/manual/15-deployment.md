# 15 部署运维

## 功能

三种部署形态覆盖从桌面用户到服务器运维的全部场景，全部支持"一条命令/双击即用"。

## 15.1 方式对比

| 方式 | 依赖 | 适用 | 数据位置 |
|------|------|------|----------|
| **Docker 编排** | 仅 Docker | 服务器 / 开发机（推荐） | Docker volumes（postgres-data / redis-data / docker/volumes/*） |
| **Windows 安装包** | 无（内置 PG/Redis） | 零命令行桌面用户 | 安装目录（postgres\data 等） |
| **源码开发** | Go 1.26+ / Node 20+ / PG | 二次开发 | 本地 PG 实例 |

## 15.2 Docker 编排（推荐）

### 架构

```text
aranea compose：
  postgres（pgvector/pgvector:pg18，含 pg_stat_statements）
  redis（redis:7-alpine，AOF）
  admin（aranea-runtime:local，后端主服务）
  k6（profile=perf，压测执行器，可选）
```

中间件 PG/Redis **不映射宿主机端口**，仅 araneanet 内网互访；服务端口按 PORT-PLAN 1:1 映射：

| 端口 | 用途 |
|------|------|
| `8810` | HTTP（前端代理 / 监控探测 / TwinMonitor voice） |
| `9910` | gRPC |
| `8812` | WebSocket |

> 📌 **compose 不含前端**。Web 界面三选一：① 源码 `cd web && pnpm dev`（`http://localhost:9301`，自动代理 API/WS）；② `pnpm build` 后用静态服务器托管 `dist/spa` 并反代 8810/8812；③ Windows 安装包（自带全栈）。

### 操作

```bash
# 构建 + 启动（Windows）
powershell -ExecutionPolicy Bypass -File docker/dev-up.ps1

# 通用
docker compose up -d          # 启动
docker compose down           # 停止（加 -v 清空数据卷）
docker compose logs -f admin  # 跟踪后端日志

# 健康检查
curl http://localhost:8810/healthz
```

### 关键环境变量（compose 已内置默认值，生产必须修改）

| 变量 | 说明 |
|------|------|
| `KRATOS_AUTH_SECRET` | JWT/PAT 签名密钥，**更换会使既有会话全部失效**；生产必须 ≥32 字节随机值 |
| `DEPLOY_ENV` | `dev` 启用开发行为 |
| `DAO_VECTOR_PGVECTOR` | `1` 启用 pgvector 向量存储 |
| `KRATOS_HTTP_EXTRA_CORS_ORIGINS` | WS Origin 额外放行（Docker 下前端经宿主机 IP 访问时必须） |
| `ARANEA_PPROF_ADDR` | pprof 端口（仅内网），缺省即关闭 |
| `CODE_EXECUTOR_BACKEND` | `docker` 启用沙箱代码执行（直通宿主机 docker.sock） |

### 排障口诀

- **WS 升级失败先看 Origin**：在 `CheckOrigin` 拒绝路径日志拿真实 origin 再补放行列表；
- **启动卡住查 PG 锁**：并行会话的长时 pg_dump/COPY 会持 ACCESS SHARE 锁阻塞迁移，`pg_stat_activity` 查 `wait_event=ClientWrite`，必要时 `pg_terminate_backend`；
- **生成代码过期**：schema/proto/wire 变更后先重生成再全量构建。

## 15.3 Windows 安装包

### Launcher 自动化

双击安装包后，AraneaLauncher 自动完成：

```text
环境预检 → 探测系统 PostgreSQL/Redis（无则启动内置实例）
  → 启动后端 → 健康检查（要求 200 且 body 含 ws_path 标记）
  → 打开桌面应用
```

健康检查有**防占用误判**三分支：我方启动中 → 等待；外部占用 → 明确报错「端口被其他程序占用」；空闲 → 正常启动链。

### 命令行旗标

| 旗标 | 作用 |
|------|------|
| （默认） | 启动全栈 + 桌面应用（首跑配置向导） |
| `-stop` | 停止内置服务（不动系统 PG/Redis） |
| `-check` | 环境检查并输出报告 |
| `-quiet` | 配合 `-check`：报告仅写文件，不弹 UI |
| `-no-console` | 不打开启动状态控制台 |
| `-setup` | 交互式配置向导（完成后退出） |
| `-headless` | 仅启动后端栈（无控制台、无桌面应用，自启动场景） |
| `-install-autostart` / `-uninstall-autostart` | 注册/移除开机自启 |

自启动两种方式：**Windows 服务**（系统 PG/Redis 模式，开机即启，需管理员）/ **登录计划任务**（内置组件模式，登录后启动）。

### 打包（发布侧）

```bash
powershell -File build/build-package.ps1   # 构建安装包（需 makensis）
```

注意：.nsi 源文件必须纯 ASCII；卸载段用 `RMDir /r $INSTDIR` 全量抹除 + 同盘兄弟目录挪位法保留 PG 数据。

## 15.4 源码开发

```bash
make all                     # proto + wire + ent + 构建

# 后端（免登录模式；必须 -tags pgvector）
$env:DEPLOY_ENV="dev"; $env:KRATOS_HTTP_AUTH_DISABLED="1"
go run -tags pgvector ./cmd/admin -conf ./configs/config.yaml

# 前端（代理到 8810）
cd web
$env:QUASAR_BACKEND_URL='http://127.0.0.1:8810'; pnpm dev
```

账号：免登录模式自动种子 `dev/dev`；真实登录模式全新库首启 `admin/changeme`。

## 15.5 备份与升级

- **备份**：`pg_dump` 全量（组织架构/Agent/记忆全在 PG）+ `docker/volumes/skills` 技能目录 + Artifact 目录；
- **升级**：迁移为版本化 DDL（`ddl_migration_registry.go`），启动自动应用；种子数据幂等（ON CONFLICT DO NOTHING）可安全重跑；
- **回滚**：升级前留全量 dump；软删数据（deleted_at）可恢复。

## 深入阅读

- [docker/dev-up.ps1](../../docker/dev-up.ps1) · [docker-compose.yaml](../../docker-compose.yaml)
- [cmd/launcher](../../cmd/launcher/main.go)
- PORT-PLAN（端口统一规划权威文档，部署包 `app/PORT-PLAN.md`）
