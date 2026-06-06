# Scenarios 专业场景文档

> 存放行业/业务场景的完整方案文档。每个场景一个子目录。

---

## 目录结构

```
docs/scenarios/
├── README.md                          # 本说明文件
├── daily-stock-analysis/              # 每日股票分析场景
│   ├── README.md                      # 场景索引（一句话定义 + 文档导航）
│   ├── daily-stock-analysis.md        # 需求文档
│   ├── daily-stock-analysis.design.md # 设计文档
│   └── daily-stock-analysis.development.md # 开发计划
└── industry-template-library/         # 行业模板库场景
    ├── industry-template-library.md        # 需求文档（待补充）
    ├── industry-template-library.design.md # 设计文档
    └── industry-template-library.development.md # 开发计划
```

## AI 存放规则

### 场景目录结构
每个场景必须创建独立子目录，目录名使用 kebab-case（连字符命名）：

```
docs/scenarios/<scenario-name>/
├── README.md                        # 场景索引
├── <scenario-name>.md               # 需求文档
├── <scenario-name>.design.md        # 设计文档
└── <scenario-name>.development.md   # 开发计划
```

### 命名规范
- **目录名**：kebab-case（`daily-stock-analysis`，禁止下划线）
- **文件名**：与目录名一致，后缀区分类型：
  - `<name>.md` — 需求文档（用户故事、验收标准、非功能需求）
  - `<name>.design.md` — 设计文档（架构、数据模型、API 设计）
  - `<name>.development.md` — 开发计划（Phase、里程碑、任务清单）
- **README.md** — 场景索引，包含一句话定义、文档导航、关键能力清单

### 新增场景流程
1. 创建 `docs/scenarios/<scenario-name>/` 目录
2. 创建 README.md 索引文件
3. 按顺序编写需求 → 设计 → 开发计划
4. 每个文件头部标注版本号和日期

### 禁止事项
- 禁止在 scenarios/ 根目录直接放置场景文档（必须放入子目录）
- 禁止使用下划线命名目录或文件
- 禁止跳过需求文档直接写设计或开发计划
