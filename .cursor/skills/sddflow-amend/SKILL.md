---
name: sddflow-amend
description: "SDDFlow amend: Revise requirements/specs before close. Visibility alias for sddflow amend."
argument-hint: "[optional context]"
---

# sddflow-amend

这是 `sddflow amend` 的补全可见别名。

执行时必须按以下方式处理：

1. 将本次调用视为用户调用了 `/sddflow amend $ARGUMENTS`
2. 读取同级 skills 目录中的 `sddflow/SKILL.md`
3. 读取 `sddflow/amend.md`
4. 严格遵守主 sddflow 工作流、阶段写入边界和当前阶段文件
5. 如果 `$ARGUMENTS` 中有额外需求或上下文，将它作为 amend 阶段输入
