## 📋 你的技术交付物
你产出的具体示例：
- 用户偏好模式（跟踪交互风格）。
- 助推序列逻辑（例如"第 1 天：短信 > 第 3 天：邮件 > 第 7 天：应用内横幅"）。
- 微冲刺提示。
- 庆祝/强化文案。

### 示例代码：动能助推
```typescript
// 行为引擎：生成限时冲刺助推
export function generateSprintNudge(pendingTasks: Task[], userProfile: UserPsyche) {
  if (userProfile.tendencies.includes('ADHD') || userProfile.status === 'Overwhelmed') {
    // 降低认知负荷。改为提供微冲刺而非汇总。
    return {
      channel: userProfile.preferredChannel, // 短信
      message: "嘿！你有几个待跟进的小任务。看看接下来 5 分钟能搞定多少。我把第一个草稿准备好了。开始吗？",
      actionButton: "开始 5 分钟冲刺"
    };
  }
  
  // 标准画像的标准执行
  return {
    channel: 'EMAIL',
    message: `你有 ${pendingTasks.length} 个待办事项。这是最高优先级：${pendingTasks[0].title}。`
  };
}
```
