-- 恢复 autolink 结构字段污染（inbox a7310ebb25e82766f6e6）
-- 1) 写前同条件 COUNT
SELECT COUNT(*) AS hit_before FROM knowledge_documents
 WHERE collection_id='a7310ebb25e82766f6e6'
   AND content_text ~ '\[\[(fact-id|session-id|agent-id|user-id|confidence|kind|source|entry)\]\]';

BEGIN;
UPDATE knowledge_documents SET content_text =
  regexp_replace(
  regexp_replace(
  replace(replace(replace(replace(replace(replace(replace(replace(content_text,
    '- [[fact-id]]:', '- fact_id:'),
    '- [[session-id]]:', '- session_id:'),
    '- [[agent-id]]:', '- agent_id:'),
    '- [[user-id]]:', '- user_id:'),
    '- [[confidence]]:', '- confidence:'),
    '- [[kind]]:', '- kind:'),
    '- [[source]]:', '- source:'),
    '- [[entry]]:', '- entry:'),
    '(- (kind|source): )\[\[([^\]]+)\]\]', '\1\3', 'g'),
    '(^|\n)(## )\[\[([^\]]+)\]\]', '\1\2\3', 'g'),
  updated_at = NOW()
 WHERE collection_id='a7310ebb25e82766f6e6'
   AND content_text ~ '\[\[(fact-id|session-id|agent-id|user-id|confidence|kind|source|entry)\]\]';

-- 2) 写后核验：结构字段污染与 H2 成链应为 0
SELECT COUNT(*) AS bad_remaining FROM knowledge_documents
 WHERE collection_id='a7310ebb25e82766f6e6'
   AND content_text ~ '(- \[\[(fact-id|session-id|agent-id|user-id|confidence|kind|source|entry)\]\]:)|(- (kind|source): \[\[)|(^|\n)## \[\[';
COMMIT;

-- 3) 删除结构标签垃圾实体（别名随行级联；提及表 knowledge_doc_entities 同行级联）
SELECT COUNT(*) AS entity_hit FROM knowledge_entities
 WHERE collection_id='a7310ebb25e82766f6e6'
   AND name_norm IN ('fact_id','session_id','agent_id','user_id','confidence','kind','source');
BEGIN;
DELETE FROM knowledge_entities
 WHERE collection_id='a7310ebb25e82766f6e6'
   AND name_norm IN ('fact_id','session_id','agent_id','user_id','confidence','kind','source');
SELECT COUNT(*) AS entity_remaining FROM knowledge_entities
 WHERE collection_id='a7310ebb25e82766f6e6'
   AND name_norm IN ('fact_id','session_id','agent_id','user_id','confidence','kind','source');
COMMIT;
