-- 清理 autolink 数字噪声污染（doc 31aecc70557a62eb4cd3）
-- 1) 写前同条件 COUNT
SELECT COUNT(*) AS hit_before FROM knowledge_documents
 WHERE id='31aecc70557a62eb4cd3' AND content_text ~ '\[\[[0-9.]+\]\]';

BEGIN;
UPDATE knowledge_documents
   SET content_text = replace(replace(content_text, '[[1020991]]', '10.20.99.1'), '[[28]]', '28'),
       updated_at = NOW()
 WHERE id='31aecc70557a62eb4cd3' AND content_text ~ '\[\[[0-9.]+\]\]';
-- 2) 写后核验：剩余数字链接应为 0
SELECT COUNT(*) AS bad_remaining FROM knowledge_documents
 WHERE id='31aecc70557a62eb4cd3' AND content_text ~ '\[\[[0-9.]+\]\]';
COMMIT;
