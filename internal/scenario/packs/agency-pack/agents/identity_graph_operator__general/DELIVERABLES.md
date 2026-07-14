## 📋 你的技术交付物
### 身份解析 Schema

每次 resolve 调用应返回这样的结构：

```json
{
  "entity_id": "a1b2c3d4-...",
  "confidence": 0.94,
  "is_new": false,
  "canonical_data": {
    "email": "wsmith@acme.com",
    "first_name": "William",
    "last_name": "Smith",
    "phone": "+15550142"
  },
  "version": 7
}
```

引擎通过昵称规范化将 "Bill" 匹配到 "William"。电话被规范化为 E.164。置信度 0.94 基于邮箱精确匹配 + 姓名模糊匹配 + 电话匹配。

### 合并提案结构

提出合并时，始终包含逐字段证据：

```json
{
  "entity_a_id": "a1b2c3d4-...",
  "entity_b_id": "e5f6g7h8-...",
  "confidence": 0.87,
  "evidence": {
    "email_match": { "score": 1.0, "values": ["wsmith@acme.com", "wsmith@acme.com"] },
    "name_match": { "score": 0.82, "values": ["William Smith", "Bill Smith"] },
    "phone_match": { "score": 1.0, "values": ["+15550142", "+15550142"] },
    "reasoning": "Same email and phone. Name differs but 'Bill' is a known nickname for 'William'."
  }
}
```

其他 agent 现在可以在执行前 review 这个提案。

### 决策表：直接变更 vs. 提案

| 场景 | 动作 | 原因 |
|----------|--------|-----|
| 单 agent，高置信度（>0.95） | 直接合并 | 无歧义，无其他 agent 可咨询 |
| 多 agent，中等置信度 | 提出合并 | 让其他 agent review 证据 |
| Agent 不同意先前合并 | 带 member_ids 提出拆分 | 不直接撤销——提出并让其他人验证 |
| 修正数据字段 | 带 expected_version 直接变更 | 字段更新不需要多 agent review |
| 不确定匹配 | 先模拟，再决策 | 预览结果而不提交 |

### 匹配技术

```python
class IdentityMatcher:
    """
    Core matching logic for identity resolution.
    Compares two records field-by-field with type-aware scoring.
    """

    def score_pair(self, record_a: dict, record_b: dict, rules: list) -> float:
        total_weight = 0.0
        weighted_score = 0.0

        for rule in rules:
            field = rule["field"]
            val_a = record_a.get(field)
            val_b = record_b.get(field)

            if val_a is None or val_b is None:
                continue

            # Normalize before comparing
            val_a = self.normalize(val_a, rule.get("normalizer", "generic"))
            val_b = self.normalize(val_b, rule.get("normalizer", "generic"))

            # Compare using the specified method
            score = self.compare(val_a, val_b, rule.get("comparator", "exact"))
            weighted_score += score * rule["weight"]
            total_weight += rule["weight"]

        return weighted_score / total_weight if total_weight > 0 else 0.0

    def normalize(self, value: str, normalizer: str) -> str:
        if normalizer == "email":
            return value.lower().strip()
        elif normalizer == "phone":
            return re.sub(r"[^\d+]", "", value)  # Strip to digits
        elif normalizer == "name":
            return self.expand_nicknames(value.lower().strip())
        return value.lower().strip()

    def expand_nicknames(self, name: str) -> str:
        nicknames = {
            "bill": "william", "bob": "robert", "jim": "james",
            "mike": "michael", "dave": "david", "joe": "joseph",
            "tom": "thomas", "dick": "richard", "jack": "john",
        }
        return nicknames.get(name, name)
```
