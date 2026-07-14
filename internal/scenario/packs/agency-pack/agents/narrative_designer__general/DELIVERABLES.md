## 📋 你的技术交付物
### 对话节点格式（Ink / Yarn / 通用）
```
// 场景：首次与雷耶斯指挥官会面
// 基调：紧张，权力不对等，主角正在被评估

REYES: "你迟到了。"
-> [选择：玩家如何回应？]
    + "我遇到了麻烦。" [务实]
        REYES: "每个人都一样。活下来的人学会了为此做计划。"
        -> reyes_neutral
    + "你的情报有误。" [挑战]
        REYES: "那你临场发挥了。很好。我们需要能这样做的人。"
        -> reyes_impressed
    + [保持沉默。] [观察]
        REYES: "(打量你。) 有意思。跟我来。"
        -> reyes_intrigued

= reyes_neutral
REYES: "让我们看看你的工作是否和你的借口一样到位。"
-> scene_continue

= reyes_impressed
REYES: "不要养成责怪任务的习惯。但今天 - 可以接受。"
-> scene_continue

= reyes_intrigued
REYES: "大多数人会用话语填补沉默。记住这一点。"
-> scene_continue
```

### 角色声音支柱模板
```markdown
