# Telemetry — 开发计划

> **版本**：2026-05-17 | **状态**：📄 占位  
> **进度真相**：[`guides/execution-plan.md`](../guides/execution-plan.md) 附录 A · **关联 EP**：EP-OBS-02 ✅

---

## 1. 模块定位

见 [`0 系统框图.md`](./0%20系统框图.md) 与 `docs/README.md` §7 对应需求/设计文档。

---

## 2. 现状评估

| 维度 | 状态 |
|------|------|
| 整体 | 📄 占位 |

对照需求文档「2026-05-17 现状对齐」段与附录 A 各列（Proto/Biz/Data/Service/Server/Runtime/前端）。

---

## 3. 差距与优化

- 未完成验收项纳入 Phase。
- 遵守双框架边界：Kratos 传输 / trpc 运行时（AI-DEV-SPEC §1）。
- 复用既有 Usecase，避免平行实现。

---

## 4. 开发阶段

- **Phase 1**：补 telemetry.md
- **Phase 2**：采样策略
- **Phase 3**：与 monitor 边界

---

## 5. 任务清单（可拆 PR）

| 序号 | 任务 | 优先级 | EP |
|------|------|--------|-----|
| 1 | Phase 1 首项 | P1 | EP-OBS-02 ✅ |
| 2 | 单测 + make lint + 更新现状对齐 | P1 | §7 |
| 3 | 前端闭环（若适用） | P2 | EP-FE-* |

---

## 6. 验收标准

- [ ] `go build ./cmd/admin`
- [ ] 相关 `go test`；并发改动用 `-race`
- [ ] 附录 A + 需求「现状对齐」一致
- [ ] changelog 引用 EP

---

## 7. 依赖与风险

M2 多租户可能触及本模块写路径；按 admin→agent→session 分批合入。
