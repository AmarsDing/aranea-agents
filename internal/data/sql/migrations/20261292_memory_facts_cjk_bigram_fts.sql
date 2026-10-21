-- 20261292 memory_facts_cjk_bigram_fts (AMC C3-B, Postgres only)
-- L3 召回 CJK 全文通道地基。FTS 'simple' 把连续中文整段当单一 lexeme，
-- 「喜欢什么颜色」类查询无法命中「喜欢蓝色」证据；既有 trgm 单通道（20261242）
-- 在 38 万行/8.2GB memory_facts 上走 GIN 全量候选扫描，酿成 2026-10-20
-- 32 并发 10-16 分钟拖死全站事故。本迁移建 bigram_text()：连续 CJK 段
-- （[一-鿿]+）展开为滑窗二元组、ASCII 字母数字段保留小写原词、
-- 空格连接，对 query 与 statement 同构分词；statement_fts_cjk 生成列 +
-- GIN 索引使 CJK 查询走索引而非全表 similarity 扫描。
-- bigram_text 纯 SQL 实现且 IMMUTABLE STRICT（生成列/索引表达式可用）。
-- 函数体为单条 SELECT（无分号、无 $$ dollar-quoting——splitDDLStatements
-- 契约禁止 $$ 体，复杂 DDL 才走 Func 迁移），单引号按 SQL换义成 ''。
-- Idempotent: CREATE OR REPLACE FUNCTION / IF NOT EXISTS；存量库重跑安全。
CREATE OR REPLACE FUNCTION bigram_text(p_text text) RETURNS text
IMMUTABLE STRICT LANGUAGE sql
AS '
SELECT COALESCE(string_agg(tok, '' ''), '''')
FROM (
  SELECT CASE
           WHEN seg ~ ''^[一-鿿]+$'' AND char_length(seg) > 1 THEN
             (SELECT string_agg(substr(seg, g.i, 2), '' '')
              FROM generate_series(1, char_length(seg) - 1) AS g(i))
           WHEN seg ~ ''^[一-鿿]+$'' THEN seg
           ELSE lower(seg)
         END AS tok
  FROM (
    SELECT (regexp_matches(p_text, ''[一-鿿]+|[a-zA-Z0-9]+'', ''g''))[1] AS seg
  ) AS segs
) AS tokens
';

ALTER TABLE memory_facts
  ADD COLUMN IF NOT EXISTS statement_fts_cjk tsvector
  GENERATED ALWAYS AS (to_tsvector('simple', bigram_text(statement))) STORED;

CREATE INDEX IF NOT EXISTS idx_memory_facts_fts_cjk
  ON memory_facts USING GIN (statement_fts_cjk);
