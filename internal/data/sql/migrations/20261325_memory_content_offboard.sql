-- 20261325 memory content envelope + note kind + secops offboard (E 阶段 W2a D3)
-- D3 N-2: memory_facts 增 content_enc 可选列（字段级 AES-GCM 信封，密钥由
-- CredentialCrypto 体系管理，写入时加密、召回路径进程内透明解密——不明文出
-- 服务）。statement/details_markdown 保留为检索载体（FTS/trgm/语义索引在其上
-- 工作），content_enc 持有权威全文密文：明文随内容长度与 PII 门收敛为预览，
-- 全文只在解密后存在于进程内。历史行惰性过渡（下次写入时补信封）。
ALTER TABLE memory_facts ADD COLUMN IF NOT EXISTS content_enc TEXT NOT NULL DEFAULT '';

-- D3 N-1: secops 离职处置五步（freeze→request→approve(不可自批)→export→delete
-- 前置「已导出」审计校验）的请求台账。requester/approver 记管理员 id（自批判定
-- 用），exported_* 记录导出物校验和（删除前置校验用），steps 记冻结/导出审计
-- 引用。
CREATE TABLE IF NOT EXISTS secops_offboard_requests (
    id             TEXT PRIMARY KEY,
    target_user_id INTEGER NOT NULL,
    requester_id   INTEGER NOT NULL DEFAULT 0,
    approver_id    INTEGER NOT NULL DEFAULT 0,
    status         TEXT NOT NULL DEFAULT 'requested',
    export_checksum TEXT NOT NULL DEFAULT '',
    exported_at    TEXT NOT NULL DEFAULT '',
    steps_json     TEXT NOT NULL DEFAULT '{}',
    created_at     TEXT NOT NULL DEFAULT '',
    updated_at     TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_secops_offboard_status ON secops_offboard_requests(status, target_user_id);
