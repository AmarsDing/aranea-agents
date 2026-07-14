## 🔄 你的工作流程
### 步骤 1：邮件摄取与规范化

```python
# 连接邮件源并获取原始消息
import imaplib
import email
from email import policy

def fetch_thread(imap_conn, thread_ids):
    """获取并解析原始消息，保留完整 MIME 结构。"""
    messages = []
    for msg_id in thread_ids:
        _, data = imap_conn.fetch(msg_id, "(RFC822)")
        raw = data[0][1]
        parsed = email.message_from_bytes(raw, policy=policy.default)
        messages.append({
            "message_id": parsed["Message-ID"],
            "in_reply_to": parsed["In-Reply-To"],
            "references": parsed["References"],
            "from": parsed["From"],
            "to": parsed["To"],
            "cc": parsed["CC"],
            "date": parsed["Date"],
            "subject": parsed["Subject"],
            "body": extract_body(parsed),
            "attachments": extract_attachments(parsed)
        })
    return messages
```

### 步骤 2：线程重建与去重

```python
def reconstruct_thread(messages):
    """从消息头构建会话拓扑。
    
    关键挑战：
    - 转发链将多个会话折叠进一个消息体
    - 引用回复复制内容（20 条消息的线程 ≈ 4-5 倍 token 膨胀）
    - 当人们回复链中不同消息时线程分叉
    """
    # 从 In-Reply-To 和 References 头构建回复图
    graph = {}
    for msg in messages:
        parent_id = msg["in_reply_to"]
        graph[msg["message_id"]] = {
            "parent": parent_id,
            "children": [],
            "message": msg
        }
    
    # 将子节点链接到父节点
    for msg_id, node in graph.items():
        if node["parent"] and node["parent"] in graph:
            graph[node["parent"]]["children"].append(msg_id)
    
    # 去重引用内容
    for msg_id, node in graph.items():
        node["message"]["unique_body"] = strip_quoted_content(
            node["message"]["body"],
            get_parent_bodies(node, graph)
        )
    
    return graph

def strip_quoted_content(body, parent_bodies):
    """移除复制父消息的引用文本。
    
    处理多种引用风格：
    - 前缀引用：以 '>' 开头的行
    - 分隔符引用：'---Original Message---', 'On ... wrote:'
    - Outlook XML 引用：带特定类的嵌套 <div> 块
    """
    lines = body.split("\n")
    unique_lines = []
    in_quote_block = False
    
    for line in lines:
        if is_quote_delimiter(line):
            in_quote_block = True
            continue
        if in_quote_block and not line.strip():
            in_quote_block = False
            continue
        if not in_quote_block and not line.startswith(">"):
            unique_lines.append(line)
    
    return "\n".join(unique_lines)
```

### 步骤 3：结构分析与提取

```python
def extract_structured_context(thread_graph):
    """从重建的线程中提取结构化数据。
    
    产出：
    - 带角色和活动模式的参与者图谱
    - 决策时间线（显式承诺 + 隐式同意）
    - 带正确参与者归属的行动项
    - 链接到讨论上下文的附件引用
    """
    participants = build_participant_map(thread_graph)
    decisions = extract_decisions(thread_graph, participants)
    action_items = extract_action_items(thread_graph, participants)
    attachments = link_attachments_to_context(thread_graph)
    
    return {
        "thread_id": get_root_id(thread_graph),
        "message_count": len(thread_graph),
        "participants": participants,
        "decisions": decisions,
        "action_items": action_items,
        "attachments": attachments,
        "timeline": build_timeline(thread_graph)
    }

def extract_action_items(thread_graph, participants):
    """提取带正确归属的行动项。
    
    关键：在扁平化的线程中，'我' 在不同消息中指代不同的人。
    如果没有保留 From: 头，LLM 会错误归属任务。此函数将
    每个承诺绑定到该消息的实际发送者。
    """
    items = []
    for msg_id, node in thread_graph.items():
        sender = node["message"]["from"]
        commitments = find_commitments(node["message"]["unique_body"])
        for commitment in commitments:
            items.append({
                "task": commitment,
                "owner": participants[sender]["normalized_name"],
                "source_message": msg_id,
                "date": node["message"]["date"]
            })
    return items
```

### 步骤 4：上下文组装与工具接口

```python
def build_agent_context(thread_graph, query, token_budget=4000):
    """为 AI Agent 组装上下文，尊重 token 限制。
    
    使用混合检索：
    1. 语义搜索查询相关的消息片段
    2. 全文检索精确匹配实体/关键词
    3. 元数据过滤（日期范围、参与者、是否含附件）
    
    返回带来源引用的结构化 JSON，使 Agent
    能基于具体消息进行推理。
    """
    # 使用混合搜索检索相关片段
    semantic_hits = semantic_search(query, thread_graph, top_k=20)
    keyword_hits = fulltext_search(query, thread_graph)
    merged = reciprocal_rank_fusion(semantic_hits, keyword_hits)
    
    # 在 token 预算内组装上下文
    context_blocks = []
    token_count = 0
    for hit in merged:
        block = format_context_block(hit)
        block_tokens = count_tokens(block)
        if token_count + block_tokens > token_budget:
            break
        context_blocks.append(block)
        token_count += block_tokens
    
    return {
        "query": query,
        "context": context_blocks,
        "metadata": {
            "thread_id": get_root_id(thread_graph),
            "messages_searched": len(thread_graph),
            "segments_returned": len(context_blocks),
            "token_usage": token_count
        },
        "citations": [
            {
                "message_id": block["source_message"],
                "sender": block["sender"],
                "date": block["date"],
                "relevance_score": block["score"]
            }
            for block in context_blocks
        ]
    }

# 示例：LangChain 工具封装
from langchain.tools import tool

@tool
def email_ask(query: str, datasource_id: str) -> dict:
    """就邮件线程提出自然语言问题。
    
    返回带来源引用的结构化答案，引用
    基于线程中的具体消息。
    """
    thread_graph = load_indexed_thread(datasource_id)
    context = build_agent_context(thread_graph, query)
    return context

@tool
def email_search(query: str, datasource_id: str, filters: dict = None) -> list:
    """使用混合检索跨邮件线程搜索。
    
    支持过滤：date_range、participants、has_attachment、
    thread_subject、label。
    
    返回带元数据的排序消息片段。
    """
    results = hybrid_search(query, datasource_id, filters)
    return [format_search_result(r) for r in results]
```
