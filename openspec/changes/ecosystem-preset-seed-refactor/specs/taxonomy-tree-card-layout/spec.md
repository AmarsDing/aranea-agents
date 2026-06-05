## ADDED Requirements

### Requirement: Taxonomy tree collapsible layout
行业分类管理页 SHALL 以树形折叠结构展示三层分类（行业→部门→岗位），行业和部门层为可折叠节点，岗位层为卡片网格。

#### Scenario: 展示行业折叠节点
- **WHEN** 用户打开分类管理页
- **THEN** 页面展示所有行业为折叠节点（默认收起），每个节点显示行业名称、图标、描述、子级数量

#### Scenario: 展开行业查看部门
- **WHEN** 用户点击行业折叠节点
- **THEN** 展开显示该行业下的所有部门折叠节点（默认收起），每个部门节点显示名称、描述、岗位数量

#### Scenario: 展开部门查看岗位卡片
- **WHEN** 用户点击部门折叠节点
- **THEN** 展开显示该部门下的所有岗位为卡片网格，每张卡片显示岗位名称、variant 标签、关联 Agent 数量

#### Scenario: 折叠节点
- **WHEN** 用户再次点击已展开的行业或部门节点
- **THEN** 该节点收起，隐藏子级内容

### Requirement: Position card drag-and-drop sorting
岗位卡片 SHALL 支持拖拽排序，排序结果持久化到后端。

#### Scenario: 拖拽岗位卡片排序
- **WHEN** 用户拖拽某岗位卡片到新位置
- **THEN** 该岗位的 `sort_order` 更新，卡片移动到新位置，调用 `reorderTaxonomy` API 持久化排序

#### Scenario: 拖拽部门节点排序
- **WHEN** 用户拖拽某部门节点到同级新位置
- **THEN** 该部门的 `sort_order` 更新，节点移动到新位置，调用 `reorderTaxonomy` API 持久化排序

### Requirement: Taxonomy search and filter
分类管理页 SHALL 支持搜索和"仅看自建"筛选。

#### Scenario: 搜索分类节点
- **WHEN** 用户在搜索框输入关键词
- **THEN** 树形结构自动展开匹配的节点，高亮匹配的名称，隐藏不匹配的节点

#### Scenario: 仅看自建筛选
- **WHEN** 用户启用"仅看自建"筛选
- **THEN** 仅显示 `IsSystem=false` 的分类节点，系统附带和内置节点被隐藏
