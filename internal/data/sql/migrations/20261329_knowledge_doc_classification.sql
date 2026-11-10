-- 20261329 knowledge_doc_classification（E 阶段 W2b D6 L-3 embedding 闸）
--
-- knowledge_documents 增 classification 密级列（0=未标；L1-L5 语义随
-- classification_labels 标签体系，20261327）。载体用途：embedding 密级闸
--（验收 #7——L3+ 内容拒绝 cloud embedding 通道）在摄取/重嵌时按文档密级
-- 判定，密级随文档行落库而非仅存 ctx（异步摄取管线与 reembed/promote/
-- sourcesync 重放均可回溯）。
--
-- 首个写入方：office 附件入知识库（office_mail.classification 已存在，
-- 20261322；MailKnowledgeSink 透传）。上传 API/连接器密级策略为后续
-- follow-up（proto 面不动）。
--
-- 幂等（IF NOT EXISTS），双方言一致；默认 0 对存量行零影响。

ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS classification INTEGER NOT NULL DEFAULT 0;
