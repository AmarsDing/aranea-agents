# PostgreSQL + pgvector 初始化

Aranea 的 **L3 语义记忆** 与 **Knowledge RAG** 使用独立 Postgres 库（配置项 `data.postgres`），与 SQLite（Ent CRUD）分离。应用启动时会自动执行 `CREATE EXTENSION vector` 并建表；**前提是数据库服务器已安装 pgvector 扩展包**。

## 1. 配置

`configs/config.yaml` 或 `configs/config1.yaml`：

```yaml
data:
  postgres:
    source: postgres://USER:PASSWORD@HOST:5432/aranea?sslmode=disable
    vector_dim: 1536   # 与 embedding 模型维度一致
```

生产环境请用环境变量覆盖：`DATA__POSTGRES__SOURCE`。

## 2. 一键初始化（扩展已安装时）

```powershell
$env:DEPLOY_ENV = "dev"
$env:KRATOS_HTTP_AUTH_DISABLED = "1"
go run ./cmd/pginit -conf ./configs/config1.yaml
```

等价于应用内 `pgvector.EnsureSchema` + `EnsureKnowledgeSchema`（创建 `agent_memory_1536`、`knowledge_*` 等表）。

## 3. 版本要求（重要）

Aranea 要求 **PostgreSQL 14+**（见根目录 `README.md`）。**pgvector 当前版本至少需要 PostgreSQL 13**。

若 `pgprobe` 显示类似 `PostgreSQL 9.6.x`，仅安装 pgvector 扩展**不够**，必须先**升级数据库大版本**（建议 16），再安装 pgvector。

## 4. 在服务器上安装 pgvector

若报错类似：

```text
could not open extension control file ".../share/extension/vector.control"
```

说明 **PostgreSQL 未安装 pgvector**，需在 **数据库主机**（非本机 Go 项目）安装与 PG 大版本匹配的扩展。

### Linux（源码 / 自定义前缀，如 `/usr/local/pgsql`）

```bash
# 示例：PG 16，安装目录 /usr/local/pgsql
export PG_CONFIG=/usr/local/pgsql/bin/pg_config
git clone --branch v0.8.0 https://github.com/pgvector/pgvector.git
cd pgvector
make
sudo make install
# 以超级用户连接 aranea 库
psql -h HOST -U postgres -d aranea -c "CREATE EXTENSION IF NOT EXISTS vector;"
```

### Docker 官方镜像

使用已带 pgvector 的镜像，例如 `pgvector/pgvector:pg16`，再创建库 `aranea` 并执行 `docs/sql/pgvector_agent_memory.sql` 或 `go run ./cmd/pginit`。

## 5. 验证

```powershell
go run ./cmd/pgprobe "postgres://..."
go run ./cmd/pginit -conf ./configs/config1.yaml
```

`pgprobe` 输出中应包含扩展 `vector`。

## 6. 未安装 pgvector 时

可暂时注释或删除 `data.postgres` 段，应用仅用 SQLite 运行（L3 / Knowledge 不可用）。安装扩展后恢复配置并执行 `pginit`。

## 7. 本机 Docker（可选）

当远程库版本过旧或无法安装扩展时，可用项目根目录 `docker-compose.postgres.yml`（映射 **5433**，用户/库 `aranea`/`aranea`）：

```powershell
docker compose -f docker-compose.postgres.yml up -d
```

将 `data.postgres.source` 改为：

```yaml
source: postgres://aranea:aranea@127.0.0.1:5433/aranea?sslmode=disable
```

再执行 `go run ./cmd/pginit`。
