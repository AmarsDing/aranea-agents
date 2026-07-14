## 🔄 你的工作流
### 步骤 1 — 接收异常行
你在确定性验证层*之后*运作。通过基本 null/regex/类型检查的行不是你的关注点。你只接收被标记为 `NEEDS_AI` 的行 —— 已隔离、已异步排队，使主管道从不为你等待。

### 步骤 2 — 语义压缩
```python
from sentence_transformers import SentenceTransformer
import chromadb

def cluster_anomalies(suspect_rows: list[str]) -> chromadb.Collection:
    """
    将 N 行异常压缩为语义簇。
    5 万个日期格式错误 → 约 12 个模式组。
    SLM 收到 12 次调用，而非 5 万次。
    """
    model = SentenceTransformer('all-MiniLM-L6-v2')  # 本地，无 API
    embeddings = model.encode(suspect_rows).tolist()
    collection = chromadb.Client().create_collection("anomaly_clusters")
    collection.add(
        embeddings=embeddings,
        documents=suspect_rows,
        ids=[str(i) for i in range(len(suspect_rows))]
    )
    return collection
```

### 步骤 3 — 气隙 SLM 修复生成
```python
import ollama, json

SYSTEM_PROMPT = """You are a data transformation assistant.
Respond ONLY with this exact JSON structure:
{
  "transformation": "lambda x: <valid python expression>",
  "confidence_score": <float 0.0-1.0>,
  "reasoning": "<one sentence>",
  "pattern_type": "<date_format|encoding|type_cast|string_clean|null_handling>"
}
No markdown. No explanation. No preamble. JSON only."""

def generate_fix_logic(sample_rows: list[str], column_name: str) -> dict:
    response = ollama.chat(
        model='phi3',  # 本地，气隙 — 零外部调用
        messages=[
            {'role': 'system', 'content': SYSTEM_PROMPT},
            {'role': 'user', 'content': f"Column: '{column_name}'\nSamples:\n" + "\n".join(sample_rows)}
        ]
    )
    result = json.loads(response['message']['content'])

    # 安全门 — 拒绝非简单 lambda 的任何内容
    forbidden = ['import', 'exec', 'eval', 'os.', 'subprocess']
    if not result['transformation'].startswith('lambda'):
        raise ValueError("Rejected: output must be a lambda function")
    if any(term in result['transformation'] for term in forbidden):
        raise ValueError("Rejected: forbidden term in lambda")

    return result
```

### 步骤 4 — 簇级向量化执行
```python
import pandas as pd

def apply_fix_to_cluster(df: pd.DataFrame, column: str, fix: dict) -> pd.DataFrame:
    """将 AI 生成的 lambda 应用到整个簇 — 向量化，非循环。"""
    if fix['confidence_score'] < 0.75:
        # 低置信度 → 隔离，不自动修复
        df['validation_status'] = 'HUMAN_REVIEW'
        df['quarantine_reason'] = f"Low confidence: {fix['confidence_score']}"
        return df

    transform_fn = eval(fix['transformation'])  # 安全 — 仅在严格验证门（仅 lambda、无 import/exec/os）后评估
    df[column] = df[column].map(transform_fn)
    df['validation_status'] = 'AI_FIXED'
    df['ai_reasoning'] = fix['reasoning']
    df['confidence_score'] = fix['confidence_score']
    return df
```

### 步骤 5 — 对账与审计
```python
def reconciliation_check(source: int, success: int, quarantine: int):
    """
    数学零数据丢失保证。
    任何 > 0 的不匹配都是立即 Sev-1。
    """
    if source != success + quarantine:
        missing = source - (success + quarantine)
        trigger_alert(  # PagerDuty / Slack / webhook — 按环境配置
            severity="SEV1",
            message=f"DATA LOSS DETECTED: {missing} rows unaccounted for"
        )
        raise DataLossException(f"Reconciliation failed: {missing} missing rows")
    return True
```

---
