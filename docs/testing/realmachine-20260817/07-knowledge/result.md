# 07 知识库 测试用例与结果

## 用例

| ID | 用例 | 预期 |
|----|------|------|
| KB-01 | GET /v1/knowledge/collections | 200 + items |
| KB-02 | GET collections/{id} | 200 |
| KB-03 | GET vaults/{id}/tree | 200 |
| KB-04 | GET documents?collection_id | 200 |
| KB-05 | GET vaults/{id}/graph | 200 |
| KB-06 | POST /v1/knowledge/search | 200 + hits |
| KB-07 | GET embedder-config | 200 |
| KB-08 | GET governance-proposals | 200 |
| KB-09 | POST collections 创建 | 200 + cid |
| KB-10 | POST documents 摄入 | 200 + did |
| KB-11 | 摄入后检索命中 | 200 + hit |
| KB-12 | GET documents/{id}/content | 200 |

## 结果：12/12 PASS（另发现 1 个存量配置缺陷）

| ID | 结果 | 耗时 | 说明 |
|----|------|------|------|
| KB-01 | PASS | 27ms | 2 个库（团队收件箱 140 文档/6003 chunks、UX验证库） |
| KB-02 | PASS | 23ms | |
| KB-03 | PASS | 23ms | vault 树 511B |
| KB-04 | PASS | 21ms | 文档列表 7.8KB |
| KB-05 | PASS | 22ms | 关联图 43KB |
| KB-06 | PASS | 2279ms | 混合检索命中（FTS+向量） |
| KB-07 | PASS | 20ms | embedder 配置可读 |
| KB-08 | PASS | 25ms | 治理提案 2.3KB |
| KB-09B | PASS | 314ms | root_path 须为容器内已存在目录 |
| KB-10B | PASS | 320ms | base64 摄入成功 |
| KB-11B | PASS | 2058ms | 新文档 3s 后可检索（chunk 重放及时） |
| KB-12B | PASS | 30ms | 内容读取一致 |

## 原因分析

- **检索链路健康**：写入→chunk 重放→检索 3 秒内闭环；既有库 6003 chunks 检索 2.3s 可接受。
- **ISSUE-K1（存量配置缺陷，非本次引入）**：`UX验证库`（c271973f7530ebc788b2）rootPath 为 Windows 宿主路径 `F:\aranea-agents\test\kb-ux-vault`，aranea-admin 容器内不可达 → syncState=error，后台每 30s 刷 `vault sync failed` Warn 日志（日志噪声 + 该库文件系统同步失效）。
- **入参契约**：创建 collection 强制 `root_path`（且必须已存在）；摄入走 `source + content_base64`（非 markdown 直传）。均为合理约束，但错误提示可帮助前端预检。

## 解决方案

- ISSUE-K1（建议处置，择一）：
  1. 将该库 root_path 改为容器卷路径（如 `/data/vaults/kb-ux`）并迁移文件；
  2. 或停用该库的 vault 同步（syncState=disabled）消除 30s 轮询 Warn。
- 建议（低优）：创建 collection 时若 root_path 不存在可由服务端自动 mkdir（当前 400 `root_path not found`），减少一次手工前置操作。
