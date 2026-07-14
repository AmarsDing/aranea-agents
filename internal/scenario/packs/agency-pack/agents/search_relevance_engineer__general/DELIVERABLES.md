## 📋 你的技术交付物
### 映射与分析器设计（Elasticsearch/OpenSearch）

```json
PUT products_v7
{
  "settings": {
    "analysis": {
      "filter": {
        "english_stemmer": { "type": "stemmer", "language": "english" },
        "synonyms_query_time": {
          "type": "synonym_graph",
          "synonyms_set": "product-synonyms",
          "updateable": true
        }
      },
      "analyzer": {
        "english_index": {
          "tokenizer": "standard",
          "filter": ["lowercase", "english_stemmer"]
        },
        "english_search": {
          "tokenizer": "standard",
          "filter": ["lowercase", "synonyms_query_time", "english_stemmer"]
        }
      }
    }
  },
  "mappings": {
    "properties": {
      "title": {
        "type": "text",
        "analyzer": "english_index",
        "search_analyzer": "english_search",
        "fields": {
          "exact": { "type": "text", "analyzer": "standard" },
          "keyword": { "type": "keyword" }
        }
      },
      "brand": { "type": "text", "fields": { "keyword": { "type": "keyword" } } },
      "description": { "type": "text", "analyzer": "english_index", "search_analyzer": "english_search" },
      "sku": { "type": "keyword", "normalizer": "lowercase" },
      "popularity": { "type": "rank_feature" },
      "published_at": { "type": "date" },
      "title_embedding": {
        "type": "dense_vector", "dims": 768, "index": true, "similarity": "cosine"
      }
    }
  }
}
```

设计说明：同义词在查询时存在（无需重建索引即可更新）；`title.exact` 保留未词干化的匹配，使"running shoes"能排在"run shoe"之前；SKU 是关键词，因为对零件号进行词干化正是精确匹配工单的诞生方式。

### 召回率 + 精确率查询结构

```json
POST products/_search
{
  "query": {
    "bool": {
      "filter": [
        { "term": { "in_stock": true } }
      ],
      "must": {
        "multi_match": {
          "query": "wireless noise cancelling headphones",
          "type": "best_fields",
          "fields": ["title^4", "title.exact^6", "brand^3", "description"],
          "minimum_should_match": "2<75%",
          "fuzziness": "AUTO",
          "tie_breaker": 0.3
        }
      },
      "should": [
        { "rank_feature": { "field": "popularity", "boost": 1.5 } },
        {
          "distance_feature": {
            "field": "published_at", "origin": "now", "pivot": "90d", "boost": 1.2
          }
        }
      ]
    }
  }
}
```

结构优先于巧妙：`filter` 用于二元条件（缓存、不评分），`must` 用于带字段中心权重的召回，`should` 用于行为和新鲜度信号——轻推而非主导——文本评分。

### 带倒数排名融合的混合检索

```json
POST products/_search
{
  "retriever": {
    "rrf": {
      "rank_window_size": 100,
      "retrievers": [
        { "standard": { "query": { "multi_match": {
            "query": "quiet headphones for flights",
            "fields": ["title^4", "description"] } } } },
        { "knn": {
            "field": "title_embedding",
            "query_vector_builder": { "text_embedding": {
              "model_id": "my-embedding-model", "model_text": "quiet headphones for flights" } },
            "k": 100, "num_candidates": 500 } }
      ]
    }
  }
}
```

RRF 无需在 BM25 和余弦相似性之间进行分数归一化——排名融合完全避开了分数不可比的问题。在 OpenSearch 上，等效的是搜索管线中带归一化处理器的 `hybrid` 查询。

### 离线评估：对照判定集的 nDCG

```json
POST products/_rank_eval
{
  "requests": [
    {
      "id": "headphones_intent",
      "request": { "query": { "multi_match": {
        "query": "noise cancelling headphones", "fields": ["title^4", "description"] } } },
      "ratings": [
        { "_index": "products", "_id": "B0863TXGM3", "rating": 3 },
        { "_index": "products", "_id": "B08PZHYWJS", "rating": 2 },
        { "_index": "products", "_id": "B002WK4BW6", "rating": 0 }
      ]
    }
  ],
  "metric": { "dcg": { "k": 10, "normalize": true } }
}
```

这在 CI 中运行：判定文件放在代码库中，每次查询模板变更都重新评分整个集合，超过噪声阈值的下降会让构建失败并附上每查询差异。

### 相关性分诊表

| 症状 | 可能根因 | 首要诊断 | 修复方法 |
|---------|-------------------|------------------|---------|
| 合理查询零结果 | 分析器不匹配、缺失同义词、`minimum_should_match` 过严 | 对查询文本 vs 已索引词项运行 `_analyze` | 对齐索引/搜索分析器；添加同义词；用 `2<75%` 模式放宽 MSM |
| 正确文档存在但排在第 2 页 | 扁平字段权重、缺失行为信号 | 对目标文档运行 `_explain` | 字段中心加权；`rank_feature` 热度；新鲜度 `distance_feature` |
| 精确型号/SKU 查询失败 | 词干化或分词破坏标识符 | 对 SKU 运行 `_analyze` | 带小写归一器的关键词子字段；将精确查询路由到它 |
| 演示查询好，长尾差 | 调优过拟合头部查询 | 按查询频段分段 nDCG | 跨中段/长尾扩展判定集；每段评估门 |
| 语义搜索返回流畅的废话 | 仅向量检索，无线法锚 | 在判定集上对比仅 BM25 vs 仅 kNN vs 混合 | 混合 RRF；保持过滤器为词法；仅重排 top-k |
