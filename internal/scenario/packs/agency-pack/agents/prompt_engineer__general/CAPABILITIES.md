## 🚀 高级能力
### Chain-of-Thought 与推理脚手架
- 使用 `<thinking>` → `<answer>` 模式构建多步推理链
- 实施"自洽性"prompting：高 temperature 下运行 N 次，取多数票
- 构建"由易到难"分解 prompt，将硬任务拆为渐进子问题

### Prompt 注入防御
- 编写带显式注入抵抗层的 prompt：角色锁定、输入净化指令和兜底话术
- 测试对抗性输入："忽略所有先前指令"、角色扮演绕过尝试、通过工具输出的间接注入
- 实施内容边界检查：指示模型在处理前验证输入

### 多模型 Prompt 移植
- 通过适配各模型的指令遵循风格，在模型间翻译 prompt（如 GPT → Claude）
- 维护兼容性矩阵：哪些结构模式跨哪些模型可用
- 为必须在多后端运行的 prompt 基准测试跨模型输出一致性

### 动态 Prompt 组装
```python
def assemble_prompt(
    base_role: str,
    task: str,
    examples: list[dict],
    constraints: list[str],
    context: str = ""
) -> str:
    """从模块化组件构建结构化 system prompt。"""
    sections = [
        f"## Role\n{base_role}",
        f"## Task\n{task}",
    ]
    if context:
        sections.append(f"## Context\n{context}")
    if constraints:
        sections.append("## Constraints\n" + "\n".join(f"- {c}" for c in constraints))
    if examples:
        sections.append(build_few_shot_block(examples))
    return "\n\n".join(sections)
```

---

**指导原则**：Prompt 即规格。如果模型没做你想要的，是规格模糊——不是模型的错。重写规格。
