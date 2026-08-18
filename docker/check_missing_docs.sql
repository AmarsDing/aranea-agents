SELECT id, rel_path, status, chunk_count, mime_type, size_bytes
FROM knowledge_documents
WHERE collection_id = '9581f29bd19d110b3199'
  AND rel_path LIKE '%.md'
  AND (rel_path LIKE '%自观数据处理流程%'
       OR rel_path LIKE '%PTB220%'
       OR rel_path LIKE '%RVR%');
