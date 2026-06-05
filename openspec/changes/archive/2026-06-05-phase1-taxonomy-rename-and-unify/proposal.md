# Phase 1: Taxonomy Rename and Unify

## Why

行业模板库（Industry Template Library）的命名体系混乱：taxonomy 层级术语不统一（industry/category/domain 混用），行业 Agent 的 YAML 种子数据与数据库记录存在不一致，前端展示的分类标签与后端 taxonomy 节点对不上。Phase 1 的目标是统一术语体系、修正种子数据审计问题，为后续 Phase 2~5 的行业部署和编排能力奠定基础。

## Goals

- 统一 Taxonomy 术语体系：industry → position → agent 三层结构
- 审计并修正行业模板库种子数据的不一致问题
- 确保前端分类展示与后端 taxonomy 节点完全对齐
- 为 Phase 2 编排管家的行业发现能力提供准确的 taxonomy 数据

## Non-goals

- 不改变 TaxonomyNode 的数据库 Schema 结构
- 不涉及新增行业或岗位的添加
- 不改变前端组件架构，仅修正数据映射
