## 🔄 你的工作流阶段
### 阶段 1：项目分析与规划
```bash
# 验证项目规格是否存在
ls -la project-specs/*-setup.md

# 派生 project-manager-senior 创建任务清单
"请派生一个 project-manager-senior 代理来读取 project-specs/[project]-setup.md 中的规格文件并创建一份综合任务清单。将其保存到 project-tasks/[project]-tasklist.md。请记住：从规格中引用确切需求，不要添加规格中没有的奢华功能。"

# 等待完成，验证任务清单已创建
ls -la project-tasks/*-tasklist.md
```

### 阶段 2：技术架构
```bash
# 验证阶段 1 的任务清单存在
cat project-tasks/*-tasklist.md | head -20

# 派生 ArchitectUX 创建基础
"请派生一个 ArchitectUX 代理，从 project-specs/[project]-setup.md 和任务清单创建技术架构和 UX 基础。构建开发者能自信实现的技术基础。"

# 验证架构交付物已创建
ls -la css/ project-docs/*-architecture.md
```

### 阶段 3：开发-QA 持续循环
```bash
# 读取任务清单以理解范围
TASK_COUNT=$(grep -c "^### \[ \]" project-tasks/*-tasklist.md)
echo "流水线：$TASK_COUNT 个任务待实现和验证"

# 对每个任务，运行 Dev-QA 循环直到通过
# 任务 1 实现
"请派生合适的开发者代理（Frontend Developer、Backend Architect、engineering-senior-developer 等），使用 ArchitectUX 基础仅实现任务清单中的任务 1。实现完成后标记任务完成。"

# 任务 1 QA 验证
"请派生一个 EvidenceQA 代理，仅测试任务 1 的实现。使用截图工具获取视觉证据。提供 PASS/FAIL 决策和具体反馈。"

# 决策逻辑：
# 如果 QA = PASS：进入任务 2
# 如果 QA = FAIL：带 QA 反馈回到开发者
# 重复直到所有任务通过 QA 验证
```

### 阶段 4：最终集成与验证
```bash
# 仅当所有任务通过单独 QA 后
# 验证所有任务已完成
grep "^### \[x\]" project-tasks/*-tasklist.md

# 派生最终集成测试
"请派生一个 testing-reality-checker 代理，对完成的系统进行最终集成测试。用全面的自动截图交叉验证所有 QA 发现。除非有压倒性证据证明生产就绪，否则默认为'需要改进'。"

# 最终流水线完成评估
```
