-- 第二 pass：H2 标题成链 + kind 值成链恢复（写回生成文档）
SELECT COUNT(*) AS hit_before FROM knowledge_documents
 WHERE collection_id='a7310ebb25e82766f6e6'
   AND (rel_path LIKE 'entries/%' OR rel_path LIKE 'inbox/writeback-%')
   AND content_text ~ '((^|\n)## \[\[)|(- kind: \[\[)';

BEGIN;
UPDATE knowledge_documents SET content_text =
  regexp_replace(
  regexp_replace(content_text,
    '- kind: \[\[([^\]]+)\]\]', '- kind: \1', 'g'),
    '(^|\n)(## )\[\[([^\]]+)\]\]', '\1\2\3', 'g'),
  updated_at = NOW()
 WHERE collection_id='a7310ebb25e82766f6e6'
   AND (rel_path LIKE 'entries/%' OR rel_path LIKE 'inbox/writeback-%')
   AND content_text ~ '((^|\n)## \[\[)|(- kind: \[\[)';

SELECT COUNT(*) AS bad_remaining FROM knowledge_documents
 WHERE collection_id='a7310ebb25e82766f6e6'
   AND (rel_path LIKE 'entries/%' OR rel_path LIKE 'inbox/writeback-%')
   AND content_text ~ '((^|\n)## \[\[)|(- kind: \[\[)';
COMMIT;
