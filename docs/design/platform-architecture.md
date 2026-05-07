# Aranea 平台架构与 Agent 编排（统一稿）

> **文档地位**：本文为评审实现、迁移与编排时以**本文为唯一真理源**。  
> **产品 KPI / 路线图**：[`docs/需求/产品需求总览.md`](../需求/产品需求总览.md)。**proto / Wire / Ent 流程**：[`docs/guides/AI-全栈新功能开发规范.md`](../guides/AI-全栈新功能开发规范.md)。**框架 API**：[`pkg/adk-go`](../../pkg/adk-go)。**ADK import 红线**：`.cursor/rules/adk-framework-first.mdc`。

---

## 总目录（按篇跳转）

| 篇 | 内容 |
|----|------|
| **第一篇** | [编排速查](#第一篇-编排速查)（Kratos、ADK-Go、`pkg/adk-go`、MCP/A2A、建设顺序、红线、网关在本项目中的落点） |
| **第二篇** | [LLM Gateway（凝练）](#第二篇-llm-gateway-设计参考凝练) |
| **第三篇** | [平台目标态架构全文](#第三篇-平台架构与目标态实现全文)（限界上下文、Kernel、Capability 执行链、迁移映射与附录） |

---

# 第一篇 编排速查

## 第一篇 · 硬约束 · AI 自检

| # | 检查项 | 若不满足 |
|---|--------|-----------|
| 1 | Agent **编排语义**（Runner、会话、Agent 树、Tool、事件流）是否落在 **`pkg/adk-go`** + **薄 `adkruntime` 适配**，而非在 `biz` 重写运行时？ | 停 |
| 2 | 新 **HTTP/API** 是否走 **`api/**/*.proto`** → `internal/service` → `internal/biz` → `internal/data`？ | 禁止未入库 proto 的私有业务路由 |
| 3 | `google.golang.org/adk` / `pkg/adk-go` **内部导入**是否**仅**出现在 **`internal/conversation/adapters/adkruntime/**`**（与第三篇一章 §6 / §12 一致）？ | 停 |
| 4 | 跨模块协作是否经 **`kernel/contracts`**，而非 Context 互 import `domain`？ | 停 |
| 5 | Capability 执行是否 **`application → executor → middleware → backends`**，**不**绕过 executor？ | 停 |
| 6 | LLM **路由 / 熔断 / 配额 / 降级**是否有明确归属（见下节「网关三层 → 本项目落点」），而非散落在各 handler？ | 逐项勾 |

**服务与编排硬性约定**：

- **服务基座**：新对外能力以 **Kratos** 为准（`api/**/*.proto`、`internal/service` → `biz` → `data`），见 [**迁移主线**](../migration/pkg-backend-to-kratos.md) 与全栈规范。
- **Agent 编排**：以 **`pkg/adk-go` 为框架真相源**；**不以** `pkg/backend` 或旧手写编排为范本（见 `.cursor/rules/adk-framework-first.mdc`）。
- **集成**：业务经**薄适配**调用 ADK **公共 API**，避免在业务包复制 ADK 内部逻辑。

---

## 第一篇 · 目标形态映射（专栏 → 工程）

| 专栏 / 业界要点 | 在本仓库中的落点 |
|-----------------|-------------------|
| **Agent = LLM + 规划 + 记忆 + 工具** | ADK：`model` + `memory`/`session` + `tool`；复杂规划用 **Workflow** 或显式 **Plan-and-Execute**。 |
| **单 Agent → 多 Agent** | ADK：`SubAgents`、树与转移；跨进程协作 **A2A**（`pkg/adk-go/server/adka2a` 等），Kratos 仅会话/任务管理面。 |
| **MCP** | **工具 / 上下文资源**接入面 → ADK **Tool**；社区 MCP Server 独立适配；Kratos 管配置、鉴权边界。已有统一工具网关时允许直连网关 API。 |
| **A2A** | **多实例 / 多角色**协议 → ADK A2A；Kratos 映射注册、启停、观测。 |
| **ReAct / Plan-and-Execute** | 工具循环 **ReAct**；长链路、人审 **先规划再执行**；设计上必须写 **终止条件**。 |
| **Human-in-the-loop** | 暂停/恢复/人工提交在 **`biz`** 与表中表达；可与 ADK `EndInvocation`/会话衔接；禁止仅进程内「假暂停」。 |
| **可观测** | Kratos 中间件 + ADK **事件流**；运行关联 `trace_id`、会话、Agent、工具、错误。 |

---

## 第一篇 · Kratos 与 ADK 分层（示意）

```mermaid
flowchart TB
  subgraph edge [Edge / Kratos]
    proto[api proto HTTP gRPC]
    svc[internal/service]
    biz[internal/biz]
    data[internal/data]
  end
  subgraph adkframe [ADK-Go / pkg/adk-go]
    runner[Runner]
    agents[Agent tree Workflow LLM]
    tools[Tool adapters]
    sess[Session Memory Artifact]
  end
  proto --> svc --> biz
  biz --> data
  svc -->|thin adapter| runner
  runner --> agents
  agents --> tools
  runner --> sess
```

- **Kratos**：鉴权、限流、审计、持久化契约、与前端的 HTTP/JSON、gRPC。**不**承载第二套 Planner/事件循环。  
- **`pkg/adk-go`**：**Invocation** 内编排语义。  
- **适配层**：`proto`/`biz` ↔ `Runner.Run`，事件迭代 → SSE/任务状态。

---

## 第一篇 · 多 Agent 拓扑（默认）

1. **Host / Supervisor**：意图分类与任务分发，子 Agent 为专家。  
2. **专家**：**单一职责**，可独立发布与回滚。  
3. **工具 / 外部世界**：Tool；标准生态 **MCP**；跨服务 **A2A**。  

ADK：**根 + `SubAgents`** 或 **Workflow Agent**；动态选题时用根 LLM Agent + `SubAgents` 描述。

---

## 第一篇 · 思考框架（须写明终止条件）

| 模式 | 适用 | 要点 |
|------|------|------|
| **CoT / 慢思考** | 复杂推理、可接受延迟 | 模型侧或提示词；**不**替代工具反馈。 |
| **ReAct** | 工具密集 | **LLM → Tool → Observation** 至结束；超时/重试在 Kratos/biz 策略化。 |
| **Plan-and-Execute** | 长任务、解释性、HITL | **计划可口持久化于 biz/表**；每步内可 ReAct；失败可局部重规划。 |

---

## 第一篇 · MCP / A2A / FC

- **Function calling**：ADK `model` 抽象，业务不绑死单一厂商 JSON。  
- **MCP**：配置驱动连接，Kratos 管密钥与网络策略；无成熟基建时不强行依赖 MCP。  
- **A2A**：独立部署专家、跨团队复用 → ADK A2A；管理面 RPC 仍在 Kratos。

---

## 第一篇 · LLM Gateway 三层 → 本项目落点

| Gateway 逻辑层 | 通用职责 | **Aranea 建议落点** |
|----------------|----------|---------------------|
| **接入** | 归一协议、鉴权、限流、配额 | **Kratos HTTP 中间件 + Identity/Capability**；入口构造 `RuntimeContext`。 |
| **决策** | 路由、健康、熔断、降级 | **`Capability.Provider` / ModelProfile** + **Operations**（开关、告警）；可为独立路由服务演进。 |
| **出口** | 厂商适配、流式统一、计费日志 | **`adkruntime`/model_bridge** + Provider 适配器；结构化日志与 span。 |

路由策略：**能力 / 成本（级联）/ 延迟（P95+RTT）/ 语义（可选）**；降级：**有限重试 + 指数退避 → 跨厂商 → 降级规格 → 缓存/保底**；熔断阈值对 LLM **放宽**以防误杀长尾。负载均衡：**优先按令牌量与队列深度加权**，慎用纯轮询。语义缓存：**强时效或个人化场景禁用或极短 TTL**。

---

## 第一篇 · 新能力建设顺序 · 红线

**顺序**：① `proto` ② `biz/data`（可恢复状态入表）③ **ADK / Agent / Tool**④ `service` 适配 ⑤ trace/指标 ⑥ HITL 的 RPC 与状态机。

**一票否决**：在 `biz` 重写运行时；私自 HTTP 路由；把 `pkg/backend` 编排习惯套入新链路；在非 `adkruntime` import ADK **实现包**绕过约定；跨 Context **直引 domain**；**绕过 executor** 调 backend。

---

## 第一篇 · 延伸参考

- 概念稿：[`docs/reference/zhihu-ai-agent-development-guide.md`](../reference/zhihu-ai-agent-development-guide.md)  
- **`docs`** 分区索引：[`README.md`](../README.md)

---

---

﻿# 第二篇 LLM Gateway 设计参考（原始讲义）

> 由原 **`docs/需求/设计需求.md` 第二部分** 迁移至此。建设目标与路线图见 [**`docs/需求/产品需求总览.md`**](../需求/产品需求总览.md) §6。

---
初期只调用一个模型、一个服务厂商时，代码编写简单直接，拼好Prompt，发个HTTP请求，就能拿到结果。可当业务复杂度提升，需要同时使用高阶大模型处理复杂推理、长文本模型做长文档分析、开源模型承担延迟敏感的轻量任务，甚至在不同云厂商部署多个推理实例时

各个调用方在自己的代码里硬编码模型名称和接口密钥，这些代码散落在几十个微服务中，维护成本极高。更危险的是，一旦外部模型服务突然限流，整条业务链路可能直接挂掉，排查半天才能发现是某个服务没有做降级处理。而这，正是LLM Gateway要解决的核心问题。

大模型面试二面抛出如何设计一个LLM Gateway这道题，考察的不仅仅是对概念的理解，更看重候选人对生产级系统的整体把控能力，能否从架构设计、核心模块、细节优化等层面，完整拆解LLM Gateway的实现逻辑，体现出技术深度和工程实践思维。路由、故障降级、负载均衡，设计考量和实现细节。今天，我们就结合实战经验，一步步拆解生产级LLM Gateway的设计思路，帮你吃透这道高频面试题，也为实际项目落地提供参考。

一、先搞懂，LLM Gateway到底是什么，它解决了什么核心问题
在正式拆解设计方案之前，我们先明确一个核心概念，LLM Gateway到底是什么。简单来说，它的角色类似于传统微服务架构中的API网关，但专门针对大模型调用这个特殊场景。它是所有大模型调用的统一入口，所有业务服务的模型调用请求，都需要经过这一层中间件，由它来统一处理这个请求该发给哪个模型、调用失败了怎么办、多个实例之间如何分配流量等核心问题。

如果没有LLM Gateway，企业在大模型应用过程中会面临四大痛点，这也是LLM Gateway的核心价值所在。

第一个痛点是接口混乱。不同服务厂商的API格式差异很大，比如通用开放模型接口、长文本专用模型接口、开源模型的接口格式各不相同，业务开发人员需要针对每个厂商编写不同的调用代码，不仅增加了开发成本，还容易出现兼容性问题。

第二个痛点是运维成本高。模型调用逻辑散落在各个微服务中，一旦需要切换模型、更新接口密钥，或者排查调用异常，就需要逐个修改各个服务的代码，效率极低，还容易出现遗漏。

第三个痛点是容错能力弱。没有统一的降级、重试机制，一旦某个外部模型服务出现故障，依赖该服务的业务就会直接瘫痪，无法快速切换到备用方案，影响业务可用性。

第四个痛点是成本和性能难以管控。不同模型的调用成本差异巨大，没有统一的路由和缓存策略，容易出现用高端模型处理简单任务的浪费情况。同时，多个实例之间的负载分配不合理，会导致部分实例过载、部分实例空闲，影响整体响应速度。

而LLM Gateway的核心作用，就是通过统一的入口，解决上述所有痛点，实现统一管控、智能路由、容错兜底、成本优化，让大模型调用更稳定、更高效、更经济。接下来，我们从整体架构入手，逐步拆解每个模块的设计细节。

二、生产级LLM Gateway整体架构，三层架构各司其职
一个可落地、高可用的生产级LLM Gateway，核心分为三层，接入层、决策层、出口层。这三层自上而下，形成了完整的请求处理链路，每一层都有明确的职责，既相互独立，又协同工作。我们结合请求的流转过程，详细讲解每一层的设计逻辑。

2.1 接入层，请求的第一站，负责标准化和基础管控
接入层是LLM Gateway的入口，主要负责接收上游业务服务的调用请求，完成请求标准化和基础管控，为后续的决策和调用打下基础。它的核心职责可以概括为4点，协议适配、参数规范化、鉴权、限流与配额检查。

首先是协议适配和参数规范化。这是接入层最核心的工作之一。由于不同业务团队可能使用不同的SDK，调用请求的格式也各不相同，有的团队按通用开放接口格式发请求，有的按长文本模型接口格式，还有的可能使用自定义格式。如果直接将这些不同格式的请求传递给决策层和出口层，会导致后续处理逻辑异常复杂。

因此，接入层需要将所有外部请求，统一转换为Gateway内部的标准格式。比如，我们可以选择主流开放模型的API格式作为内部标准，因为它的生态最完善，大多数开发者最熟悉，接入层接收请求后，将非标准格式的请求转换为统一的内部格式，包括Prompt结构、参数字段、请求方式等，确保后续决策层和出口层只需要处理一种标准格式，降低系统复杂度。

其次是鉴权。LLM Gateway作为统一入口，必须做好权限管控，防止非法请求调用模型资源。鉴权的实现方式有多种，常见的包括接口密钥鉴权、令牌鉴权、IP白名单等。我们可以为每个业务服务分配唯一的接口密钥，接入层接收请求时，先验证密钥的有效性，只有通过鉴权的请求，才能进入下一层处理。同时，还可以结合业务需求，设置细粒度的权限控制，比如某个密钥只能调用特定的模型，或者只能处理特定类型的请求。

最后是限流与配额检查。大模型调用的成本较高，同时外部服务厂商通常会对接口调用频率、并发数有严格限制，如果不做限流，可能会出现某个业务服务大量调用模型，导致成本失控，或者触发厂商的限流机制，影响所有业务的正常使用。接入层需要实现限流功能，比如基于令牌桶算法或漏桶算法，限制每个密钥的调用频率和并发数。同时，还可以设置配额管理，比如为每个业务团队分配每月的调用配额，超过配额后拒绝请求，确保资源的合理分配。

接入层的设计要点是简单、高效、安全，它不涉及复杂的决策逻辑，只需要完成请求的标准化和基础管控，确保进入决策层的请求都是合法、规范的。

2.2 决策层，Gateway的大脑，负责核心决策逻辑
决策层是LLM Gateway的核心，也是整个系统最有技术含量的部分。它接收接入层传递过来的标准化请求，基于预设的规则和实时数据，做出三个关键决策，该请求路由给哪个模型、调用失败时的故障降级链路是什么、该请求发给哪个模型实例。这三个决策直接决定了系统的可用性、成本和性能，需要结合多维度的因素综合考量。

决策层的核心组件包括路由规则引擎、模型健康状态监控、负载均衡器。其中，路由规则引擎负责路由决策和故障降级编排，模型健康状态监控负责实时采集各模型、各实例的运行状态，为决策提供数据支持，负载均衡器负责将请求合理分配到各个实例。

这里需要注意的是，决策层的所有决策都需要是动态的、可配置的，不能硬编码。比如，路由规则、故障降级链路、负载均衡策略等，都应该支持通过配置中心动态调整，不需要重启系统，这样才能快速适应业务变化和模型状态变化。

2.3 出口层，请求的最后一站，负责执行和响应反馈
出口层接收决策层的决策结果，负责将请求发送给目标模型服务厂商，处理响应结果，并将其反馈给上游业务服务。它的核心职责包括，发起实际API调用、处理流式响应、响应格式反向转换、日志和指标记录。

首先是发起实际API调用。出口层根据决策层指定的模型和实例，向对应的服务厂商发起API调用。这里需要注意的是，不同厂商的API格式不同，出口层需要根据模型类型，将内部标准格式的请求转换为该厂商支持的格式，比如将内部标准格式转换为通用模型格式、长文本模型格式，或者开源模型的格式。

其次是处理流式响应。很多大模型调用场景需要支持流式输出，比如智能客服、实时内容生成，这就要求出口层能够处理流式响应如SSE事件流。出口层需要接收厂商返回的流式数据，然后按照内部标准格式，将流式数据转发给上游业务服务，确保业务服务能够实时获取响应结果。

然后是响应格式反向转换。和接入层的参数规范化相反，出口层需要将厂商返回的响应格式，转换为内部标准格式，再转发给上游业务服务。这样，上游业务服务只需要处理一种标准响应格式，不需要关心背后是哪个厂商在提供服务，实现了对业务服务的透明化。

最后是日志和指标记录。出口层作为请求处理的最后一站，需要记录整个调用链路的详细日志，包括请求参数、路由结果、调用时长、响应状态、错误信息等，方便后续排查问题。同时，还需要采集核心指标，比如调用成功率、响应延迟、令牌消耗量等，为决策层的优化和运维监控提供数据支持。

至此，LLM Gateway的三层架构就清晰了。接入层负责入口管控和标准化，决策层负责核心决策，出口层负责执行和反馈，三层协同工作，形成了完整的请求处理链路。接下来，我们深入拆解每个核心模块的设计细节，这也是面试中考察的重点。

三、核心模块深度拆解，从路由到容错每一步都要考虑周全
在LLM Gateway的设计中，路由策略、故障降级机制、负载均衡是三个最核心的模块，也是区别于传统API Gateway的关键。传统API Gateway的路由、容错、负载均衡逻辑相对简单，而LLM Gateway由于面对的是大模型调用的特殊场景，需要结合模型特性、业务需求、成本控制等多方面因素，做更精细的设计。

3.1 路由策略，不止是转发，更是智能匹配
路由是LLM Gateway最基础也是最核心的功能之一，但它远比传统API Gateway的路由复杂。传统API Gateway通常按照URL路径或请求头来做路由，规则非常确定，而LLM Gateway的路由决策需要综合考虑多个维度，这些维度之间还经常存在权衡，比如性能优先和成本优先的冲突。

在生产级系统中，我们通常不会只使用一种路由策略，而是组合多种策略，根据业务场景动态调整。下面我们介绍四种最常用的路由策略，以及它们的适用场景和实现思路。

3.1.1 基于能力的路由，让合适的模型做合适的事
基于能力的路由是最基础、最常用的一种策略，核心逻辑是根据请求的任务类型，将其路由到最擅长处理该任务的模型。不同大模型的能力差异很大，没有一款模型能适配所有场景。

高阶通用模型在复杂推理、代码生成、多轮对话等场景下表现出色，适合处理需要深度思考的任务。
长文本专用模型在超长上下文处理、指令遵循、敏感内容审核等场景下有优势，适合处理超长篇幅文档分析任务。
轻量级模型在简单分类、关键词提取、情感分析等轻量任务上性价比极高，响应速度快，适合处理对延迟敏感、复杂度低的任务。
开源部署模型可以私有化部署，不受外部厂商限流限制，适合处理对数据隐私要求高、需要自定义优化的任务。

基于能力的路由实现思路有两种，一种是让调用方在请求中携带任务类型标签， Gateway根据标签直接路由到对应的模型。另一种是Gateway自动判断任务类型，比如根据输入的令牌长度，将超长文本请求自动路由到长文本模型，将代码相关的请求自动路由到高阶通用模型，将简单分类请求路由到轻量模型。

这种策略的优势是能够充分发挥不同模型的优势，提升响应质量和效率。缺点是需要预先定义任务类型和模型的对应关系，对于复杂的任务类型，判断难度较大。

3.1.2 基于成本的路由，用最少的成本达到预期效果
在生产环境中，成本控制是一个非常现实的问题。不同模型的调用成本差距可以达到10倍甚至100倍，比如高阶通用模型的调用成本远高于轻量模型，更是远超私有化部署的开源模型。如果所有请求都用高端模型处理，成本会快速失控，因此基于成本的路由就显得尤为重要。

基于成本的路由最常用的实现方式是分级路由，业界也称之为级联路由，核心逻辑是先用便宜的小模型尝试处理，若输出质量不达标，再升级到大模型。这种方式可以在保证业务质量的前提下，大幅降低调用成本，实际项目中通常能节省50%以上的调用成本。

具体实现思路如下。
第一，为不同类型的任务，定义模型分级体系。
第二，接收请求后，先将请求路由到最低级的便宜模型，让其处理并返回结果。
第三，对小模型的输出结果进行质量校验，校验方式可以是置信度评分，也可以是规则检查，比如检查输出是否符合业务要求、是否存在错误。
第四，若质量达标，则直接将结果返回给调用方。若质量不达标，则将请求升级到上一级的高端模型，重复上述过程，直到得到合格的结果，或者达到最高级模型仍无法达标，则触发故障降级机制。

比如，在智能客服场景中，用户的简单咨询可以先用轻量模型处理，成本低、响应快。如果用户的问题比较复杂，轻量模型的输出质量不达标，再路由到高阶通用模型处理，既保证了响应质量，又控制了成本。

3.1.3 基于延迟的路由，优先保障实时场景的响应速度
在很多实时交互场景中，响应延迟是核心指标，比如实时聊天机器人、直播内容生成、在线代码辅助等，用户对延迟的容忍度很低，一旦延迟超过3秒，用户体验就会大幅下降。而不同服务厂商、不同实例的响应延迟，在不同时间段可能差异很大，比如某个厂商在高峰期的延迟可能是平时的3倍以上，因此基于延迟的路由就非常有必要。

基于延迟的路由核心逻辑是维护各服务厂商、各实例的实时延迟统计，将对延迟敏感的请求，优先路由到当前响应最快的厂商或实例。具体实现思路如下。
第一，出口层在每次调用模型后，记录该厂商、该实例的响应延迟，从发起请求到接收第一个响应字节的时间。
第二，决策层维护一个实时延迟统计池，用滑动窗口平均值或P95延迟来表征每个厂商、每个实例的当前延迟状态，避免单次异常延迟影响决策。
第三，接收请求时，判断该请求是否为延迟敏感型，可以由调用方标记，或由Gateway根据任务类型判断。
第四，对于延迟敏感型请求，从实时延迟统计池中筛选出延迟最低的厂商和实例，将请求路由过去。对于非延迟敏感型请求，则可以结合成本、能力等因素，综合决策路由目标。

这里需要注意的是，延迟统计需要实时更新，滑动窗口的大小要合理，既要保证数据的实时性，又要避免因短期波动导致决策频繁变化。同时，还要考虑网络延迟的影响，跨区域部署的实例，虽然自身处理延迟低，但网络延迟高，整体响应延迟可能反而更高，因此需要综合计算处理延迟加网络延迟，再做路由决策。

3.1.4 语义路由，无需人工定义规则自动识别请求类型
前面三种路由策略，都需要人工定义规则，比如任务类型与模型的对应关系、延迟阈值、分级体系，当业务场景变得复杂，任务类型增多时，规则维护成本会越来越高，而且容易出现规则遗漏或误判的情况。而语义路由，正是为了解决这个问题而出现的。

语义路由的核心逻辑是不依赖人工定义规则，而是通过向量嵌入模型将请求内容向量化，然后与预定义的任务类别向量做相似度匹配，自动判断请求类型，进而路由到对应的模型。这种方式的好处是对调用方完全透明，调用方不需要显式指定任务类型，Gateway就能自动识别，大幅降低了调用方的接入成本。

具体实现思路如下。
第一，预先定义业务中常见的任务类别，比如代码生成、长文档分析、情感分析、多轮对话等。
第二，用一个轻量级的向量嵌入模型，将每个任务类别的描述文本向量化，得到任务类别向量，存储在向量数据库中。
第三，接收请求后，用同一个向量嵌入模型，将请求的提示词内容向量化，得到请求向量。
第四，在向量数据库中，查找与请求向量相似度超过阈值的任务类别向量，确定该请求的任务类型。
第五，根据任务类型，结合能力、成本、延迟等因素，路由到对应的模型。

语义路由的优势是规则维护成本低，能够适应复杂的业务场景，对调用方透明。缺点是需要维护一个向量嵌入模型和向量数据库，且相似度阈值的设定需要不断优化，否则可能出现路由误判。

在实际项目中，我们通常会组合使用这四种路由策略，先用语义路由识别请求类型，再用基于能力的路由确定候选模型，然后用基于延迟和成本的路由，从候选模型中选择最优的目标模型和实例，确保响应质量、速度和成本的平衡。

3.2 故障降级机制，不止是重试，更是多层次的容错体系
很多人对故障降级的理解停留在调用失败了就重试一次，或者换个模型再试，但在生产级系统中，故障降级的设计远比这复杂。它本质上是一套多层次的容错体系，核心目标是在模型调用失败时，尽可能保证业务可用性，减少对用户的影响。一个完善的故障降级机制，需要考虑不同的失败场景，设计不同的应对策略，形成层层兜底的瀑布结构。

我们将故障降级机制分为四层，从简单到复杂，从局部到全局，确保在各种失败场景下，都能有对应的应对方案。同时，还需要引入熔断器机制，避免无效重试，提升系统效率。

3.2.1 第一层，同模型重试，针对临时故障的快速恢复
同模型重试是故障降级的最基础一层，核心逻辑是当请求调用某个模型失败时，在同一个服务厂商的同一个实例或其他实例上，再次发起调用。这种方式主要用于应对临时故障，比如网络波动、厂商接口短暂不可用等，重试后大概率能成功。

但同模型重试并不是简单的失败就重试，需要注意三个关键细节，否则可能导致系统雪崩或无效消耗。

第一个细节是重试次数。重试次数不能太多，通常设置为2到3次即可。如果重试次数过多，不仅会增加调用成本，还可能导致请求排队，加重厂商的负担，甚至触发厂商的限流机制，适得其反。

第二个细节是指数退避。重试时需要加入指数退避策略，也就是每次重试的间隔时间呈指数增长，避免短时间内多次重试，减少对厂商的冲击，同时也给临时故障留出恢复时间。

第三个细节是失败类型区分。并不是所有失败都适合重试，超时、限流类错误可以重试，因为这些失败通常是临时的。而参数错误、权限错误、禁止访问类错误，重试一百次也不会成功，反而会浪费资源，因此这类失败不需要重试，直接返回错误信息。

3.2.2 第二层，跨厂商降级，核心模型故障后的备用方案
如果同模型重试都失败了，说明该服务厂商的该模型可能出现了严重故障，此时需要切换到备用厂商的同类型模型，这就是跨厂商降级。

跨厂商降级的核心难点在于不同厂商的API格式、能力不同，直接切换会导致调用失败。因此，Gateway需要维护一个预定义的降级链，同时在出口层做好请求格式的适配转换。

Gateway会预设好模型降级优先级，当主力模型调用失败且重试无效后，自动切换到同级备用模型，同时在出口层完成请求格式自动转换，对调用方完全无感。

这里需要注意的是，降级链的顺序需要根据业务需求设定，优先保证响应质量，再保证可用性。同时，需要为每个模型设置对应的格式适配规则，确保格式转换的准确性，避免因格式错误导致降级失败。

3.2.3 第三层，跨模型等级降级，高端模型不可用时的兜底保障
如果跨厂商降级也失败了，说明所有同类型的高端模型都不可用，或者排队太长，此时需要降级到低一级的模型，这就是跨模型等级降级。这种方式的核心是牺牲部分响应质量，换取业务可用性，虽然输出质量会下降，但至少能保证服务不中断。

高阶模型不可用时，自动降级到同厂商的轻量版本模型，超长文本模型故障时，降级到精简版长文本模型。这种降级通常需要业务方预先确认哪些场景允许降级、降级到什么程度，避免因降级导致业务体验严重下降。

3.2.4 第四层，兜底策略，所有模型不可用时的最后防线
如果所有模型都不可用，或者所有降级策略都失败了，就需要触发兜底策略，这是保障业务可用性的最后防线。兜底策略的设计需要根据业务场景灵活选择，常见的有三种。

第一种是返回缓存的历史结果。如果该请求在过去一段时间内有过成功调用，且请求内容相似度很高，可以通过语义缓存判断，则直接返回缓存的历史结果。这种方式适合对实时性要求不高的场景，比如常见问题查询、固定内容生成等。

第二种是返回预设的默认回答。根据业务场景，预先设置一些默认回答，告知用户当前服务繁忙，请稍后再试，避免用户长时间等待。

第三种是优雅降级到人工服务。对于核心业务场景，可以将请求转发给人工客服，由人工处理，确保业务不中断。

将这四层串起来，故障降级的完整链路大致是，同模型重试带指数退避，跨厂商切换，降级到小模型，兜底返回。每一层之间需要设置合理的超时阈值，避免用户等待太久，影响体验。


3.2.5 关键补充，熔断器机制
在故障降级机制中，还有一个不可或缺的组件，熔断器。它的核心作用是防止无效重试，保护系统资源。当某个服务厂商在短时间内连续失败达到阈值，熔断器会断开，后续请求会直接跳过这个厂商，走降级链路，不再浪费时间去尝试一个大概率会失败的调用。

熔断器的工作流程分为三个状态。
第一，闭合状态，正常工作，所有请求都可以调用该厂商，同时记录失败次数。
第二，断开状态，当失败次数达到阈值，熔断器断开，后续请求直接走降级链路，不再调用该厂商。
第三，半开状态，断开一段时间后，熔断器进入半开状态，放少量请求去探测厂商是否恢复。如果探测成功，熔断器恢复闭合状态。如果探测失败，继续保持断开状态。

这里需要注意的是，LLM调用的正常延迟本身就比较高，几秒到十几秒，所以熔断器的超时阈值设定要比传统微服务宽松，避免因正常延迟导致误判为失败。同时，失败次数的阈值也要根据厂商的稳定性和业务需求合理设置，避免因少量失败就触发熔断器断开。

3.3 LLM场景下的负载均衡，区别于传统服务的智能分配
负载均衡是分布式系统的核心组件，传统微服务架构中，常用的负载均衡策略有轮询、最少连接等。但这些策略在LLM场景下会遇到一些独特的问题，无法直接使用，因为LLM请求的处理时间差异极大，这是传统服务所没有的特点。

传统服务的请求处理时间相对均匀，用轮询或最少连接策略，能实现较好的负载均衡效果。但LLM请求的处理时间差异可以达到几十倍，短生成请求一秒即可完成，长文本生成请求可能耗时几十秒。如果用简单的轮询策略，可能把几个长请求都分到同一个实例上，导致该实例排队严重，而其他实例却很空闲，出现负载不均的情况，影响整体响应速度。

因此，LLM Gateway的负载均衡需要更聪明的策略，结合LLM请求的特点，实现更精细的负载分配。下面介绍两种常用的负载均衡策略，以及它们的实现思路。





3.3.1 加权负载均衡，基于多维度指标的动态加权
加权负载均衡的核心逻辑是为每个实例分配一个动态权重，权重根据实例的当前状态实时调整，请求根据权重分配到各个实例。这里的权重不是固定的，而是综合考虑多个维度的指标，确保负载分配更合理。

需要考虑的核心指标包括。
第一，实例当前的并发请求数，并发数越少，权重越高。
第二，实例的队列深度，队列深度越浅，权重越高，队列深度指等待处理的请求数。
第三，预估处理时长，根据请求的输入令牌数，粗略预估该请求的处理时长，预估时长越短，权重越高。

具体实现思路如下。
第一，决策层实时采集每个实例的并发请求数、队列深度等指标。
第二，根据预设的算法，将这些指标转换为实例的权重。
第三，请求到来时，根据各实例的权重，将请求分配到权重最高的实例。

这种策略的优势是能够动态适应实例的负载变化，避免某个实例因承接过多长请求而过载，同时也能充分利用各个实例的资源，提升整体处理效率。

3.3.2 基于令牌吞吐量的均衡，更精细的负载分配
对于处理时间差异极大的LLM请求，基于请求数的负载均衡仍然不够精细，长文本请求的处理量远大于普通短请求。此时，基于令牌吞吐量的均衡策略会更合适，核心逻辑是不再按请求数来均衡，而是按令牌处理量来均衡，让每个实例处理的总令牌数大致相当。

具体实现思路如下。
第一，决策层实时统计每个实例当前正在处理的请求的总令牌数，包括输入令牌和输出令牌的预估数。
第二，对于新到来的请求，计算该请求的预估令牌处理量。
第三，将请求分配到当前总令牌处理量加预估令牌处理量最小的实例。

这种策略的优势是能够更精准地平衡各个实例的负载，避免某个实例因处理多个长请求而承担过多的令牌处理量，确保每个实例的负载都处于合理范围，提升整体响应速度。

3.3.3 跨区域部署的特殊考量，就近路由与负载平衡
如果企业的业务分布在多个区域，LLM Gateway通常会跨区域部署多个实例，此时还需要考虑就近路由策略。核心逻辑是优先把请求发给物理距离近的实例，减少网络延迟。

但就近路由不能绝对化，需要和实例负载做平衡。如果最近的实例已经满载，宁可绕远路发给一个空闲的实例，也不要将请求分配到满载的实例，避免请求排队导致延迟大幅增加。

3.4 统一接口与协议适配，屏蔽差异降低接入成本
一个好的LLM Gateway，应该对调用方暴露统一的API接口，屏蔽掉后端不同服务厂商的协议差异，让调用方不需要关心背后是哪个模型、哪个厂商在提供服务，只需要专注于自身业务逻辑。这件事听起来简单，做起来却有很多细节需要考虑，因为不同厂商之间的差异，远不止JSON字段名不同那么表面。

3.4.1 统一接口设计，以通用开放格式为事实标准
目前，主流开放模型的API格式是业界最通用的大模型接口格式，大多数开发者都熟悉，而且生态完善，很多SDK和工具都兼容该格式。因此，LLM Gateway对外提供的统一接口，建议以该格式为标准，对外暴露标准化对话接口地址。

这样一来，调用方只需要将原有接口地址修改为LLM Gateway的地址，不需要修改任何代码，就能实现对不同模型的调用，接入成本极低。

示例代码

```python
import openai

openai.api_base = "https://api.openai.com/v1"
openai.api_key = "sk-xxx"

response = openai.ChatCompletion.create(
    model="gpt-4o",
    messages=[{"role": "user", "content": "Hello World"}]
)
```

修改后只改地址即可

```python
import openai

openai.api_base = "https://llm-gateway.example.com/v1"
openai.api_key = "sk-gateway-xxx"

response = openai.ChatCompletion.create(
    model="gpt-4o",
    messages=[{"role": "user", "content": "Hello World"}]
)
```

Gateway内部会根据模型标识参数，路由到对应的模型和服务厂商，调用方完全不需要关心背后的逻辑。

3.4.2 协议适配的细节，处理厂商之间的深层差异
不同服务厂商之间的差异，远不止接口格式那么简单，还包括以下几个方面，需要在协议适配层妥善处理。

第一，流式响应的差异。不同厂商的SSE事件格式不同，协议适配层需要将这些差异统一，确保调用方接收到的流式响应格式一致。

第二，函数调用的差异。不同厂商的函数调用参数结构不同，协议适配层需要将调用方发送的函数调用请求，转换为目标厂商支持的格式。

第三，令牌计算方式的差异。不同厂商的令牌统计规则不同，协议适配层需要统一令牌计算口径，为调用方返回一致的令牌消耗统计。

第四，错误码体系的差异。不同厂商的错误码定义各不相同，协议适配层需要将各类厂商错误码，统一转换为Gateway标准错误码，并返回清晰的错误描述，方便调用方排查问题。

第五，特有功能的适配。部分厂商拥有专属高级能力，协议适配层需要做兼容处理，要么统一封装为Gateway扩展能力，要么通过扩展参数允许业务方按需使用专属特性。

协议适配层的设计要点是兼容为主，灵活扩展，既要保证大部分调用方能够无缝接入，又要为需要使用厂商特有功能的调用方，提供灵活的扩展方式。

3.5 缓存与可观测性，生产级系统的必备能力
除了路由、故障降级、负载均衡这三个核心模块，生产级LLM Gateway还需要两个不可或缺的能力，缓存和可观测性。缓存主要用于降低成本和延迟，可观测性主要用于运维监控和策略优化，两者都是保证系统稳定、高效运行的关键。





3.5.1 语义缓存，大幅降低重复调用成本
传统的缓存是精确匹配缓存，只有当请求内容完全一致时，才能命中缓存。但在LLM场景下，用户的表述方式千变万化，相同需求会有多种不同话术表达，精确匹配很难命中，会造成不必要的重复调用、成本浪费和延迟增加。

而语义缓存，正是为了解决这个问题而出现的。它的核心逻辑是将请求做向量嵌入，在向量数据库中查找相似度超过阈值的历史请求，如果命中就直接返回缓存结果，不需要请求内容完全一致，只要语义相近，就能命中缓存。

语义缓存的实现思路如下。
第一，接收请求后，用轻量级向量嵌入模型将提示词向量化，得到请求向量。
第二，在向量数据库中，查找与请求向量相似度达标的历史请求向量。
第三，如果找到匹配的历史请求，且缓存未过期，就直接返回缓存响应。如果未匹配或缓存过期，再调用模型处理，并存入向量数据库设置过期时间。

目前开源社区已有成熟的语义缓存框架，支持多种向量模型和向量数据库，可快速集成到LLM Gateway中落地使用。

但需要注意的是，语义缓存并不是万能的，它有两个明显的局限性。
第一，时效性问题。实时资讯、天气查询类请求不适合长期缓存，需要设置短过期时间或直接禁用缓存。
第二，精度问题。创意写作、个性化定制类请求，不适合启用语义缓存，避免返回重复内容影响体验。

因此，在实际应用中，需要根据业务场景，灵活配置语义缓存的生效范围和过期时间，平衡成本、延迟和用户体验。

3.5.2 可观测性，运维监控和策略优化的基础
LLM Gateway是所有大模型调用的必经之路，天然就是最好的监控数据采集点。可观测性的核心目标是全面掌握系统的运行状态，快速排查问题，优化路由策略和系统性能。一个完善的可观测性体系，需要采集三类核心数据，日志、指标、链路追踪。

首先是日志。日志需要记录整个调用链路的详细信息，包括请求信息、路由信息、调用信息、故障降级信息。包含请求唯一标识、调用方凭证、请求时间、提示词参数、模型标识、任务类型，路由决策过程、目标模型、目标厂商、部署实例，调用起止时间、响应状态、错误详情、令牌消耗、调用成本，是否触发故障降级、降级层级、降级触发原因。

日志需要结构化存储，方便后续检索分析，出现故障时可通过请求标识全链路溯源，快速定位问题根因。

其次是指标。指标是运维监控和策略优化的核心数据，需要实时采集可视化展示，核心指标包括调用指标、延迟指标、成本指标、路由指标、故障降级指标、系统指标。包含各厂商各模型调用量、成功率、失败率，P50/P95/P99分位延迟，令牌总消耗量、日均月度成本统计，各类路由策略命中率、小模型升级大模型比例，故障降级触发频次、各层级降级占比、降级成功率，网关服务自身CPU、内存、网络负载及各推理实例负载状态。

这些指标通过监控看板实时展示，配置合理告警阈值，出现成功率下滑、延迟飙升时及时告警。同时利用指标数据反向调优路由权重、降级规则、缓存策略，实现系统自适应优化。

最后是链路追踪。链路追踪用于串联请求从发起、网关处理、模型调用到结果返回的全流程，清晰展示各环节耗时与状态，快速定位延迟过高、调用失败等问题根因。可接入开源链路追踪组件，实现全链路可视化排查。

四、开源方案选型，快速落地LLM Gateway的捷径
如果从零开始开发LLM Gateway，需要投入大量的人力和时间，而且容易出现各种细节问题。对于大多数团队来说，基于开源方案进行二次开发，是更高效、更稳妥的选择。目前业界有多款成熟开源LLM网关组件，各有能力侧重，可按需选型落地。

4.1 轻量开源网关，快速统一多模型接口
这类开源组件生态适配极强，支持上百类大模型与服务厂商，对外统一兼容标准开放接口，内置故障降级、基础负载均衡、成本统计能力，代码轻量化易部署，适合中小团队快速搭建基础网关能力，几行代码即可完成多模型统一接入。

4.2 智能路由开源组件，专注成本优化
由知名开源社区推出，不偏重协议转发和网关基础能力，核心聚焦智能模型分级路由，通过轻量判别模型自动区分简单请求与复杂请求，优先调度低成本小模型，复杂请求再调度高阶大模型，在不损失回答质量的前提下显著节约调用成本，适合已有基础网关，只想叠加智能路由成本优化的团队。

4.3 企业级开源网关，生产环境开箱即用
偏向企业级私有化部署，内置完整可观测性、语义缓存、熔断降级、权限管控、多厂商协议适配全套能力，无需额外开发监控日志体系，开箱即用稳定性强，适合中大型企业核心业务生产落地，看重合规管控、高可用、全链路运维能力的场景。

五、面试标准作答总结，考场直接可用
面试被问到如何设计LLM Gateway时，可以按这套逻辑流畅作答，条理清晰、深度足够。

LLM Gateway本质上是大模型场景下的专用API网关，统一收口所有模型调用请求，解决多模型多服务厂商散乱调用、无容错、难管控、成本不可控的问题。整体我会设计三层架构，接入层、决策层、出口层。

接入层负责协议归一、鉴权限流、请求参数标准化，屏蔽不同SDK和厂商的请求格式差异，统一转为内部标准协议，同时做基础安全和流量管控。

决策层是核心，包含智能路由、多级故障降级、负载均衡、健康状态感知。路由会组合四种策略，按模型能力做业务场景路由，按级联分级路由控制成本，按实时P95延迟做动态调度，高阶场景用语义路由自动识别用户意图。故障降级设计四层容错，同模型带指数退避重试、跨厂商切换、降级低规格小模型、最后兜底缓存或默认应答，同时引入熔断器避免无效调用雪崩。负载均衡不适用传统轮询，而是基于并发队列、缓存占用、令牌总量做加权均衡，私有化部署模型还能感知推理引擎内部批次和缓存状态做调度。

出口层负责协议反向适配、流式SSE转发、统一响应格式、全链路日志指标采集。对外统一兼容通用开放接口，业务侧零改造即可接入多模型。

---

# 第三篇 平台架构与目标态实现全文

> **架构指导（请作为本项目约定记忆）**  
> **`docs/design/platform-architecture.md` 第三篇** 为 **Aranea Agents** **平台目标态实现设计**（原 `platform-architecture.md` 全文）：模块划分、分层、路由与跨模块协作原则以本篇为准。实现、重构与代码评审应优先对照本文；`docs/需求/` 下各专题需求（tools、skills、memory 等）在本原则内细化延伸；若有冲突以**本文架构原则**优先，并回写专题文档以消除矛盾。**产品目标与非功能 KPI**见 `docs/需求/产品需求总览.md`。  
> **Go module path**：文中示例 import 路径与当前仓库一致，使用 `arenea/backend/...`（历史命名）。若日后全仓库统一改为 `aranea/backend/...`，应同步替换示例与 `go.mod`。

> **AI 速查（动手前先读这八条）**  
> 1. **目标态目录**：所有新代码落到「二、§3」的目标 Context 下，**禁止**在 `internal/{repository,runtime,transport,service,tools,domain,middleware}` 等旧路径新增文件。  
> 2. **Context 边界**：跨 Context 协作只走 `kernel/contracts/`；`<context>/domain` 与 `<context>/application` 不允许 import 其它 Context（一章 §8 编译期红线）。  
> 3. **ADK 隔离**：`google.golang.org/adk` 仅出现在 `internal/conversation/adapters/adkruntime/**`。  
> 4. **SQL 归属**：表前缀必须等于 Context 名（`identity_*` / `catalog_*` / `capability_*` / `conversation_*` / `memory_*` / `operations_*`），SQL 仅在 `<context>/adapters/sqlite/**` 出现（三章 §2）。  
> 5. **迁移粒度**：按一章 §12.1.1 映射表，**一行 = 一个 PR**；五步流程见 §12.1.2，违反红线立即停手。  
> 6. **Kernel 准入**：把接口提到 `kernel/contracts/` 必须满足「≥3 Context 实现/消费 + 已稳定 ≥2 个 PR 周期」（一章 §5、§9）。  
> 7. **能力执行链**：Capability 的运行时调用必须经 `application → executor → middleware → backends`，**禁止**跳过 executor（四章 §4、§13）。  
> 8. **冲突即停**：若任务与本文 §2 设计原则、§5 Kernel 边界、§8 依赖红线冲突，先在本文档登记例外或修正本文，再写代码。

---
# 一、主体架构设计

> 本章是 Aranea 的**目标架构（Target Architecture）**：以「**限界上下文 + 端口与适配器（Ports & Adapters）+ go-adk 内嵌运行时**」重新立项设计，与当前 `internal/` 目录的实现并不一一对应。后续章节（路由、数据库、模块功能）在本章框架下展开；与现有实现的差异及迁移路径见 §12。

---

## 1. 系统定位

Aranea 是一个**以 Agent 为中心的多智能体编排平台**：用户在 Web/CLI 中创建 Agent、装配能力（Tool/Skill/MCP）、配置记忆（L0~L4）与提供商，然后通过统一会话与 Agent 进行多轮、多模态、可流式的协作；后台基于自演化机制持续优化 Agent 行为。

平台本身**不实现**：模型推理、向量检索引擎、对象存储、IM 协议——这些经端口接入外部依赖。

执行底座是 [go-adk](https://google.golang.org/adk)：Aranea 把 ADK 的 `agent / runner / session / memory / artifact / tool / plugin / model` 当作**内嵌引擎**用，但**不让 ADK 类型穿透到业务代码**。

---

## 2. 设计原则

下面 9 条是后续所有设计的**判定基准**。任何新模块、PR、专题文档若与之冲突，必须在本章登记例外或修正本章。

1. **限界上下文（Bounded Context）**：按业务概念切分，而不是按技术层切分。系统由若干高内聚、低耦合的 Context 组成；Context 内部允许丰富，Context 之间只能通过**端口、命令、事件、查询**协作。
2. **端口与适配器（Hexagonal）**：每个 Context 对外只暴露**端口（Port）**，所有实现细节是**适配器（Adapter）**——HTTP、CLI、SQLite、go-adk、LLM SDK、向量库等都是"被适配"的，不是"内嵌"的。
3. **接口清晰（四要素显式）**：模块边界（哪个 Context）、调用协议（命令/查询/事件签名）、数据格式（领域类型 + JSON schema）、依赖方向（谁依赖谁），四者必须**写进 `ports/` 包并在文档中固化**。
4. **不变与变化分离**：协议、领域类型、事件信封、能力契约属于**不变层**；后端实现、运行时、SDK、UI 属于**变化层**。两者在目录、包名、变更流程上必须区分。
5. **单一职责**：一个包只解决一类技术问题；一个 Context 只解决一类业务问题。功能跨包靠**组合**，禁止"上下文穿透"（不要为了图省事把别的 Context 的类型直接拉进来）。
6. **共享内核（Shared Kernel）最小化**：内核只放**所有 Context 都依赖**且**长期稳定**的内容（ID、时间、错误、事件信封、运行上下文、Module 接口）。绝不放业务策略与领域逻辑。
7. **依赖方向单向收敛**：`adapter → context → kernel`、`runtime adapter → context.port`，**绝不反向**。同层 Context 之间只能依赖对方在 `kernel` 中暴露的端口，不能直接 import 对方包。
8. **可观测优先**：tracing、结构化事件、SSE、审计、用量从 Day 1 起即作为**架构约束**而非补丁，由内核统一定义信封，由 ADK Plugin / 运行时 middleware 注入。
9. **可裁剪部署**：通过 launcher 装配不同 Context 子集（console、web、full、headless agent），**业务代码不感知 launcher**。

---

## 3. 顶层架构（Hexagonal）

Aranea 整体是一个**六边形架构**：业务 Context 在内圈，端口（Ports）在边界上，外圈由各类 Adapter 接入；go-adk 是 Conversation Context 的内嵌引擎，本身也通过端口被业务调用。

```text
                    ┌─────────────── DRIVING ADAPTERS ───────────────┐
                    │  REST API · SSE · CLI · Cron · A2A · Webhook   │
                    └───────────────────────┬───────────────────────┘
                                            │ 命令 / 查询
                                            ▼
   ┌────────────────────────────── BUSINESS CONTEXTS ──────────────────────────────┐
   │                                                                              │
   │   Identity   │  Catalog   │ Capability │ Conversation │  Memory  │ Operations │
   │   ───────    │  ───────   │  ───────   │  ──────────  │  ──────  │  ───────── │
   │   user       │  agent     │  tool      │  session     │  L0..L4  │  cron      │
   │   team       │  evolution │  skill     │  message     │  recall  │  monitor   │
   │   workspace  │  prompt    │  mcp/plugin│  channel     │  decay   │  audit     │
   │   role       │            │  hook      │  team-run    │          │  budget    │
   │                                                                              │
   │   每个 Context 对内：domain · application · ports · 内部组件                    │
   │   每个 Context 对外：仅 ports（命令/查询/事件/能力契约）                          │
   └──────────────────────────────────┬──────────────────────────────────────────┘
                                      │
                ┌─────────────────────┴─────────────────────┐
                │                                           │
                ▼                                           ▼
   ┌────────────── KERNEL（共享内核·稳定层） ──────┐  ┌──────────── DRIVEN ADAPTERS ─────────────┐
   │ ids · clock · errs · event · runctx ·          │  │ 基础设施 ：SQLite · Postgres · pgvector ·  │
   │ contracts · module · telemetry · pkg           │  │            FS · S3 · HTTP                  │
   └────────────────────────────────────────────────┘  │ 外部 SDK ：LLM provider · MCP 客户端       │
                                                       │ 内嵌引擎：adkruntime（Conversation 私有，  │
                                                       │           唯一允许 import google.../adk） │
                                                       └────────────────────────────────────────────┘
```

阅读约定：

- **Driving**：把请求"驱动进来"的入口（REST/CLI/Cron/A2A）。
- **Driven**：被业务"驱动出去"的依赖（DB/LLM/FS/外部 HTTP）。
- **go-adk** 在 Aranea 中是 **Driven Adapter 的一种特殊形态**——一个内嵌引擎：业务 Context 通过 `ConversationRuntime` 端口请求"运行 Agent 一轮对话"，由 `adk-adapter` 实现该端口并调用 `adk.Runner`。
- **adkruntime 的归属是 Conversation Context 私有**：Catalog / Capability / Memory / Operations / Identity **不**直接依赖它，跨 Context 触发会话执行只能经 `kernel/contracts.ConversationRuntime`。
- **Kernel** 是六边形的中心，所有 Context 都可以依赖它，但**它不依赖任何 Context**。

---

## 4. 限界上下文（Bounded Contexts）

Aranea 的现有功能（agents、sessions、tools、skills、L0~L4 memory、provider、avatar、cron、agent-evolution、channel、multi-agent、mcp、plugin/hook、ecosystem、token、monitor、cli、frontend BFF）按业务语义重组为 **6 个限界上下文**：

| Context        | 业务范围                                                                 | 核心聚合                                       | 主要文件来源（旧 → 重新归位）                                                |
| -------------- | ------------------------------------------------------------------------ | ---------------------------------------------- | ------------------------------------------------------------------------------ |
| **Identity**   | 用户、团队、工作区、角色、租户、API Key、配额                            | `User`、`Team`、`Workspace`、`Role`            | team / token 中的身份与配额部分                                                |
| **Catalog**    | Agent 编目与生命周期：定义、配置、模板、提示文件、演化、头像             | `Agent`、`AgentVersion`、`Avatar`、`Evolution` | agents、agent_evolution、avatar、agent-title、agent-type、agent-setting        |
| **Capability** | 可被 Agent 装配的能力：Tool、Skill、MCP、Plugin/Hook、Provider/Model     | `Tool`、`Skill`、`MCPServer`、`Plugin`、`Provider` | tools、skills、mcp、plugin/hook、provider                                     |
| **Conversation** | 一次会话从开始到结束的全链路：会话、消息、通道、流式、多 Agent 协作    | `Session`、`Message`、`Channel`、`TeamRun`     | sessions、chat、channel、multi-agent、team_runtime、runtime                    |
| **Memory**     | L0~L4 五层记忆体系，独立可演进                                           | `L0Snapshot`、`L1Slot`、`L2Episode`、`L3Fact`、`L4Node` | memory_l0..l4、memory-L*-* 文档                                            |
| **Operations** | 一切"非用户实时路径"：调度、监控、审计、用量与成本、生态适配            | `Job`、`Schedule`、`AuditLog`、`UsageEvent`、`CostLedger` | cron、monitor、token（用量部分）、ecosystem、hook（运行期）              |

每个 Context 对内由 4 个子层组成（DDD 风格，详见「四、模块功能设计」）：

```text
<context>/
├── domain/        # 实体、值对象、聚合、领域事件
├── application/   # 用例（Command/Query handler）、事务边界
├── ports/         # 对外端口（input port = 用例接口；output port = 依赖接口）
├── adapters/      # 输入适配器（HTTP/CLI/Cron）+ 输出适配器（SQL/HTTP/ADK 等）
└── module.go      # Context Shell：装配 + 注册 + 生命周期
```

Context 之间**不允许**互相 import `domain` 或 `adapters`；只能通过对方在 `kernel/contracts/` 中暴露的端口协作。

---

## 5. 共享内核（Kernel）

Kernel 是六边形的中心，**最小、稳定、无业务语义**。建议位置：

```text
internal/kernel/
├── ids/             # ULID/UUID 生成与解析
├── clock/           # 时间抽象、可注入 fake clock
├── errs/            # 错误码常量、Domain Error、Wrap/Is 工具
├── event/           # 事件信封定义 + Bus 接口
├── runctx/          # RuntimeContext：tenant/user/session/agent/trace/budget
├── contracts/       # 跨 Context 端口集合（见下）
├── module/          # Module 接口、装配协议
├── telemetry/       # tracer/meter/instrumentation 名约定
└── pkg/             # 中性工具：jsonutil、httpx、validate 等
```

`kernel/contracts/` 定义**跨上下文端口**，例如：

```go
// kernel/contracts/catalog.go
type AgentReader interface {
    GetAgentByID(ctx context.Context, id AgentID) (AgentSnapshot, error)
}

// kernel/contracts/capability.go
type ToolResolver interface {
    ResolveForAgent(ctx context.Context, agent AgentID) ([]ToolDescriptor, error)
}

// kernel/contracts/memory.go
type MemoryAssembler interface {
    AssembleForTurn(ctx context.Context, in TurnInput) (MemoryView, error)
}

// kernel/contracts/conversation.go
type ConversationRuntime interface {
    Run(ctx context.Context, req TurnRequest) (<-chan event.Envelope, error)
}
```

**硬约束**：

- Kernel **不 import** 任何 `internal/<context>` 包（编译期防回环）。
- Kernel 只允许依赖 `context`、`time`、`encoding/json`、OpenTelemetry API、`google/uuid` 等中性库，**不依赖 go-adk、SQLite、chi**。
- Kernel 中不放策略与算法，只放接口与稳定数据类型。
- 提升进入 Kernel 的门槛：**≥3 个 Context 已实现或将要实现该端口；接口经过 ≥2 次 PR 周期未变更**。

### 5.1 事件 Bus 最小契约

`kernel/event.Bus` 是异步协作（§7.1 Event）的统一通道。所有 Context 的事件订阅与发布**必须**经它，禁止 Context 之间用裸 channel 互通。

| 项               | 约束                                                                                  |
| ---------------- | ------------------------------------------------------------------------------------- |
| **接口**         | `Publish(ctx, env Envelope) error` + `Subscribe(typeGlob, h Handler) Cancel`           |
| **订阅注册时机** | 在 `Module.Start` 内一次性注册；`Shutdown` 反向 Cancel；禁止运行期动态增删订阅          |
| **投递语义**     | 默认 at-least-once；handler 必须**幂等**（按 `Envelope.ID` 去重）                       |
| **失败重试**     | 内置指数退避；超过阈值进入 `operations_event_dead_letter` 死信表，由 Operations 复盘     |
| **顺序保证**     | 单一 `subject.id` 内有序；跨 subject 不保证                                             |
| **类型命名**     | `<context>.<aggregate>.<verb>`，例如 `conversation.turn.completed`、`capability.tool.executed` |

实现策略：

- **单进程**：`kernel/event/memory_bus.go`（缓冲通道 + worker pool），launcher 默认装配。
- **多进程**：实现 `kernel/event/nats_bus.go` 等 adapter，**接口不变**；切换由 `app.Container` 决定。

### 5.2 可观测最小契约

| 维度        | 约定                                                                              |
| ----------- | --------------------------------------------------------------------------------- |
| Tracer Name | 与包路径一致：`arenea/backend/internal/<context>` 或 `<context>/<subsystem>`        |
| Span Name   | `<context>.<verb>` 或 `<context>.<aggregate>.<verb>`                              |
| 必填属性    | `tenant.id` `user.id` `session.id` `agent.id` `trace.id`（来自 `runctx`）          |
| 错误标记    | error 返回时 `span.SetStatus(codes.Error)` + `span.SetAttributes("error.code", …)` |
| Metric 命名 | `aranea.<context>.<metric>` 全小写蛇形；直方图必带 `_seconds` / `_bytes` 等单位后缀 |
| Log 字段    | 结构化；必带 `tenant_id` `trace_id`；**禁止**打印密钥与原始 prompt 全文              |

属性键集中在 `kernel/telemetry/names.go` 定义为常量；Context **禁止**在自家代码中硬编码字符串属性键。

### 5.3 RuntimeContext 注入路径

`kernel/runctx.RuntimeContext` 由 driving adapter 创建，沿调用链传递；ADK callback 进入业务前由 `adkruntime/plugins` 重建同一份 RuntimeContext。

```text
HTTP / CLI / Cron driving adapter
   │ 中间件解析 token / API Key → 构造 RuntimeContext（tenant/user/agent/trace/budget）
   │ ctx = runctx.With(parent, rc)
   ▼
<context>.application.Service.Method(ctx, …)        # ctx 已带 RuntimeContext
   ▼
<context>.ports.Output（contracts.*Reader/Writer/Resolver/Runtime）
   ▼
conversation.adapters.adkruntime.Runner.Run(ctx, …)
   │ adk.plugin.Before*：从 ctx 重建 RuntimeContext，再回调
   ▼
capability.application.ExecuteTool(ctx, …)          # 同一份 RuntimeContext
```

强制条款：

- 所有跨包公共函数**首参**必须是 `context.Context`；从 `runctx.From(ctx)` 取 RuntimeContext，**禁止**通过结构体字段透传。
- 派生子 ctx（带超时 / 带值）必须基于父 ctx；**不允许** `context.Background()` 覆盖上游 ctx。
- ADK callback 进入业务之前由 `adkruntime/plugins` 重建 ctx；丢失 RuntimeContext 视为 P0 缺陷。
- HTTP 中间件构造 RuntimeContext 失败（无身份 / token 过期）→ 直接 401，不允许继续下游。

---

## 6. 与 go-adk 的关系（运行时层）

go-adk 是 Aranea 唯一的"会话执行引擎"，但**仅服务于 Conversation Context**，不是全局基础设施。它在六边形里的位置如下：

```text
Conversation.application
        │  command/query: RunTurn
        ▼
Conversation.ports.ConversationRuntime（输出端口，定义在 Conversation 内）
        │
        ▼
adapters/adkruntime/                          ← 唯一引用 google.golang.org/adk 的位置
   ├── runner.go        实现 ConversationRuntime
   ├── agent_builder.go 把 Catalog/Capability/Memory 的视图组装成 adk.Agent
   ├── tool_bridge.go   Capability.ToolDescriptor → adk.Tool
   ├── skill_bridge.go  Capability.SkillDescriptor → adk.skilltoolset
   ├── memory_bridge.go Memory.View → adk.memory.Service
   ├── model_bridge.go  Provider.ModelProfile → adk.model.Model
   └── plugins/         审计/脱敏/用量/成本以 adk.Plugin 形式注入
```

**对应关系（Aranea ↔ go-adk）**：

| Aranea 概念                            | go-adk 抽象                                  | 适配责任                                                        |
| -------------------------------------- | -------------------------------------------- | --------------------------------------------------------------- |
| `Conversation.Session`                 | `adk.session.Service` + `adk.session.Session` | 自家持久化为主，必要时实现 `adk.session.Service` 提供给 Runner |
| `Conversation.RunTurn` 命令            | `adk.Runner.Run(...)`                        | adkruntime 把 TurnRequest → `adk.Runner` 输入                  |
| `Catalog.Agent` 定义                   | `adk.agent.Agent`（含 `llmagent`、`workflowagents`、`remoteagent`） | agent_builder 按 Agent 类型选择具体 ADK Agent     |
| `Capability.Tool/Skill/MCP`            | `adk.tool.Tool`、`skilltoolset`、`mcptoolset` | 桥接器持有"原生 backend"，转出 `adk.Tool`，回调再回到自家执行链 |
| `Memory.View`（L0~L4 已聚合）          | `adk.memory.Service`                         | memory_bridge 实现 `Service`：Search/Add 路由到 L2/L3/L4         |
| `Capability.Plugin/Hook`、Operations 审计 | `adk.plugin.Plugin`                       | 审计/脱敏/用量/成本/超时统一以 ADK Plugin 注入                 |
| `Capability.Provider/Model`            | `adk.model.Model`                            | model_bridge 把 ModelProfile 转为具体 Model 实现               |
| `Conversation.Event` 流                | `adk.event.Event` 流                         | adkruntime 订阅 ADK 事件 → 转换为 Aranea event.Envelope         |

**集成约束**：

- **adkruntime/ 是 Aranea 中唯一允许 import `google.golang.org/adk` 的包**。其他业务 Context（Catalog/Capability/Memory/Identity/Operations）**禁止**直接 import ADK。
- ADK 触发的回调（before_tool/after_tool/before_model_request/…）在 adkruntime 内**重建 RuntimeContext**，再回调到对应 Context 的 application 用例（例如 Capability.ExecuteTool），不是直接触发 backend。这样校验、预算、事件、tracing 始终走业务路径。
- ADK 版本与 replace 路径在根 `go.mod` 显式声明；升级 ADK = 修改 adkruntime + 升级 go.mod，**不应改业务 Context**。
- 未来若需替换 ADK（多运行时、自研引擎），只新增一个 `adapters/<other>runtime/`，业务 Context 完全不感知。

---

## 7. 接口与数据契约

### 7.1 三种跨 Context 协作方式

```text
Command      （状态变更）   A.application ──► B.ports.<Action>     同步、强一致
Query        （只读检索）   A.application ──► B.ports.<Reader>     同步、可缓存
Event        （事实通告）   A.publish ──► event.Bus ──► B.handler  异步、最终一致
```

判定规则：

- 修改 B 内部状态 → **Command**（B 暴露动词式接口）。
- 仅读取 B 数据 → **Query**（B 暴露 Reader 接口）。
- "我做了什么" → **Event**（A 发布，B 自愿订阅；A 不知道 B 是谁）。

### 7.2 数据格式契约（Kernel 强制）

- **标识符**：所有持久实体使用 ULID（`01JABC...`，时间可排序）；外部友好 key/slug 使用 `^[a-z][a-z0-9_]{0,63}$`。
- **时间**：API 与事件统一 `time.Time` + `RFC3339Nano UTC`；DB 存原生 timestamp 或 ISO8601 字符串，不允许混存"本地时间"。
- **JSON Schema**：所有 Tool/Skill/Capability 的 input/output schema 由 typed Go struct 通过 `kernel/pkg/schemagen` 统一生成，禁止手写。
- **错误响应**：

  ```json
  {
    "error": {
      "code": "CONVERSATION_TURN_BUDGET_EXCEEDED",
      "message": "agent 'researcher' exceeded per-turn tool budget",
      "details": { "budget": 8, "used": 8 },
      "retryable": false
    }
  }
  ```

- **事件信封**（`kernel/event/envelope.go`）：

  ```json
  {
    "id": "01JABC...",
    "type": "capability.tool.executed",
    "context": "capability",
    "version": "1",
    "occurred_at": "2026-04-27T10:00:00Z",
    "actor": { "tenant_id": "...", "user_id": "...", "agent_id": "..." },
    "subject": { "kind": "tool", "id": "read_file" },
    "trace_id": "...",
    "session_id": "...",
    "payload": { "...": "..." }
  }
  ```

- **SSE 帧**：`event: <type>\ndata: <json envelope>\n\n`，前端按 `context + type` 路由。
- **分页**：列表统一 `?limit=&cursor=`；返回 `{ "items": [...], "next_cursor": "..." }`，禁止使用 offset 持久化游标。

### 7.3 错误码 → HTTP 状态映射

`kernel/errs.Code` 是稳定枚举；HTTP 状态由 `kernel/pkg/httpx.MapErr` 统一翻译，业务 handler **禁止**直接写 `http.StatusXxx`。

| 错误码族              | HTTP | 触发场景示例                                  |
| --------------------- | ---- | --------------------------------------------- |
| `*_INVALID_INPUT`     | 400  | 参数校验失败、JSON 解析失败                   |
| `*_UNAUTHENTICATED`   | 401  | 缺失 token / API Key 失效                     |
| `*_FORBIDDEN`         | 403  | 权限不足、跨租户访问被拒                      |
| `*_NOT_FOUND`         | 404  | 资源不存在 / 软删除后访问                     |
| `*_CONFLICT`          | 409  | 唯一约束冲突、状态机非法跳转                  |
| `*_PRECONDITION`      | 412  | If-Match / 乐观锁版本不一致                   |
| `*_PAYLOAD_TOO_LARGE` | 413  | 单次请求体或附件超限                          |
| `*_BUDGET_EXCEEDED`   | 429  | turn budget / token quota 超限                |
| `*_RATE_LIMITED`      | 429  | 限流命中                                      |
| `*_DEPENDENCY_FAILED` | 502  | 下游 LLM / MCP / 外部 HTTP 失败              |
| `*_TIMEOUT`           | 504  | 上游 ctx 超时                                 |
| 其它                  | 500  | 兜底；响应必含 `trace_id`，详细栈仅入日志    |

约束：

- 错误码命名采用 `<CONTEXT>_<DOMAIN>_<REASON>` 大写蛇形，例如 `CONVERSATION_TURN_BUDGET_EXCEEDED`。
- 错误响应（§7.2 JSON 结构）必含 `trace_id`，便于通过 `kernel/telemetry` 反查。
- 内部错误（5xx）禁止把堆栈或 SQL 原文返回客户端；只允许在结构化日志中保留。
- 同一错误**只在最靠近用户的层级翻译一次**：domain 抛 `errs.Error`，application 用 `%w` 包装上下文，adapter 调 `httpx.MapErr` 写出 HTTP 响应。

---

## 8. 依赖方向（编译期约束）

```text
                  ┌───────────────────────────┐
                  │           Kernel          │   ← 不依赖任何 Context
                  └───────────────────────────┘
                              ▲
                              │（每个 Context 单向依赖 kernel）
        ┌──────────┬──────────┼──────────┬──────────┬──────────┐
        ▼          ▼          ▼          ▼          ▼          ▼
   Identity    Catalog   Capability Conversation  Memory   Operations
        ▲          ▲          ▲          ▲          ▲          ▲
        │          │          │          │          │          │
        └──────────┴──── 通过 kernel/contracts 端口互相协作 ────┘
                              ▲
                              │
                  ┌───────────────────────────┐
                  │   Adapters（Driving + Driven）  │
                  │   HTTP/CLI/Cron · SQL · adkruntime · LLM SDK · FS  │
                  └───────────────────────────┘
                              ▲
                              │
                  ┌───────────────────────────┐
                  │       External world      │
                  └───────────────────────────┘
```

**编译期红线**（CI 静态检查必须守住）：

1. `kernel/**` 不允许出现 `import "arenea/backend/internal/<context>/..."`。
2. `<context>/domain` 不允许 import 任何 adapter、任何外部 SDK、任何其它 Context 包。
3. `<context>/application` 只允许 import 本 Context 的 `domain`、`ports`，加 `kernel/**`。
4. 跨 Context import 仅允许出现在 `<context>/adapters/**`，且 import 的目标必须是 **`kernel/contracts`** 中的接口（不是对方的 domain/application）。
5. `google.golang.org/adk` 只允许出现在 `adapters/adkruntime/**`。
6. `chi`、`net/http` 路由相关只允许出现在 `<context>/adapters/http/**` 与 `cmd/**`。
7. `database/sql`、`modernc.org/sqlite` 只允许出现在 `<context>/adapters/sqlite/**` 与 `kernel/pkg/db/**`。

工具建议：在 CI 加入 `gomodguard` 或自定义 `go list -deps` 校验。

---

## 9. 不变与变化的分离

| 类别                        | 类型     | 位置                                  | 变更约束                                       |
| --------------------------- | -------- | ------------------------------------- | ---------------------------------------------- |
| `kernel/event/Envelope`     | 不变     | `kernel/event/`                       | 仅新增字段；删除字段需弃用周期 + 双发           |
| `kernel/contracts/*` 端口   | 不变     | `kernel/contracts/`                   | 破坏性变更需所有实现方同步迁移                  |
| `kernel/runctx.RuntimeContext` | 不变 | `kernel/runctx/`                      | 字段只增不删；含义不可重定义                    |
| 错误码命名规则              | 不变     | `kernel/errs/`                        | 旧码保留 ≥1 个发布周期                          |
| HTTP 协议骨架（`/api/v1/*`）| 不变     | `<context>/adapters/http/` 公共部分   | 破坏性变更走 `/api/v2`                          |
| `<context>/domain` 实体     | 半稳定   | 各 Context 内部                       | 通过 application 暴露视图，禁止外泄结构        |
| `<context>/ports` 输出端口  | 半稳定   | 各 Context 内部                       | 跨次要版本兼容，必要时双轨                      |
| 数据库 schema               | 半稳定   | 各 adapter 的 `migrations/`           | 仅向前迁移；破坏性需旁路表 + 双写               |
| Adapter 实现                | 易变     | `<context>/adapters/**`               | 自由替换，前提是端口契约不变                    |
| go-adk 集成                 | 易变     | `adapters/adkruntime/**`              | 升级 ADK 不应改业务 Context                     |
| 前端 / UI                   | 易变     | `frontend/**`                         | 与 API 协议解耦，可独立发版                     |

判定"是否提升为不变"的统一标准：**接口被 ≥3 个 Context 实现/消费 + 回滚成本高 + 已稳定 ≥2 个 PR 周期**。

---

## 10. 跨 Context 协作的具体场景

下面 4 个场景定义了 Aranea 大部分核心交互，作为后续模块设计的参考样板。

### 10.1 一次普通对话（同步路径）

```text
HTTP POST /api/v1/conversations/{id}/turns
   │
   ▼
Conversation.adapters.http
   │ command: RunTurn(turnReq)
   ▼
Conversation.application.RunTurnHandler
   ├─► Catalog.AgentReader.GetAgentByID         (kernel/contracts)
   ├─► Capability.ToolResolver.ResolveForAgent  (kernel/contracts)
   ├─► Capability.SkillResolver.ResolveForAgent (kernel/contracts)
   ├─► Memory.MemoryAssembler.AssembleForTurn   (kernel/contracts)
   └─► Conversation.ports.ConversationRuntime.Run
            │
            ▼
       adapters.adkruntime
            ├─► agent_builder：装配 adk.Agent + plugins
            ├─► adk.Runner.Run(ctx, …)
            └─► 订阅 adk.Event ► 转 Envelope ► event.Bus + SSE
                     │
        ┌────────────┴────────────────┐
        ▼                             ▼
  Memory.handler.OnTurnEvent     Operations.handler.OnTurnEvent
   （写 L2 episode、L0 snapshot） （计费、审计、监控）
```

要点：

- 所有上下文都通过 `kernel/contracts/` 协作；Conversation 不知道 Capability 用 SQLite 存 Tool。
- Memory 不在 Conversation 的同步路径上写 L2，而是订阅事件异步写入。
- Operations 完全旁路监听，可独立开关。

### 10.2 工具调用（同步嵌入式）

```text
adk.Runner ─► before_tool plugin ─► 回到 Capability.application.ExecuteTool
                                                 │
                                                 ▼
                                Capability.ports.ToolExecutor （chain: validate/budget/audit/exec）
                                                 │
                                                 ▼
                                       adapters/<tool-backend>
```

ADK 永不直接执行 Aranea 的 tool backend；中转点是 **Capability.application.ExecuteTool**，由 plugin 把 ADK 的 tool 调用桥接回业务用例。

### 10.3 后台演化（事件驱动）

```text
event.Bus ── conversation.turn.completed ──► Catalog.handler.AgentEvolutionScanner
                                                  │
                                                  ▼
                                          analyze ─► propose ─► persist
                                                  │
                                                  ▼
                                event.Bus ── catalog.agent.evolution.proposed
                                                  │
                              ┌───────────────────┴────────────────────┐
                              ▼                                        ▼
                         frontend SSE                         Operations.audit
```

### 10.4 调度任务（Driving = Cron）

```text
Operations.adapters.cron ─► command: RunJob(memory_consolidation)
                              │
                              ▼
                  Memory.application.ConsolidateL2toL3
                              │
                              ▼
                 Memory.adapters.{sqlite, vectorstore, llm}
```

Cron 是 driving adapter，与 HTTP 平级；同一 application 用例既能被 HTTP 触发也能被 Cron 触发。

---

## 11. 部署与运行模式

入口由 `cmd/aranea/launcher/` 提供四种装配，**业务 Context 不感知 launcher**：

| Launcher  | 启用 Driving               | 启用 Driven                    | 启用 Context                                 | 典型用途                  |
| --------- | -------------------------- | ------------------------------ | -------------------------------------------- | ------------------------- |
| `console` | CLI                        | SQLite                         | Identity / Catalog / Capability              | 本地脚本、CI、批操作      |
| `web`     | HTTP / SSE                 | SQLite + provider SDK + adkruntime | 全部                                         | 单机部署、Web 端用户       |
| `full`    | HTTP / SSE + CLI + Cron    | SQLite + provider SDK + adkruntime + FS | 全部                                         | 开发与单机生产默认         |
| `agent`   | A2A / Webhook              | provider SDK + adkruntime      | Identity / Catalog / Capability / Conversation | 无人值守 Agent（headless） |

**装配协议**：每个 Context 提供 `module.Module` 实现：

```go
type Module interface {
    Name() string
    Version() string
    RegisterPorts(reg *contracts.Registry)   // 把本 Context 的 output port 注册到 kernel
    ResolvePorts(reg *contracts.Registry)    // 从 kernel 拿到本 Context 需要的端口
    RegisterDriving(driving DrivingRegistry) // HTTP/CLI/Cron 子路由
    Start(ctx context.Context) error
    Shutdown(ctx context.Context) error
}
```

Launcher 流程严格固定：

```text
LoadConfig → BuildKernel(clock/event-bus/db/telemetry)
           → InstantiateModules(配置中启用的 Context)
           → 阶段 1：所有 Module.RegisterPorts
           → 阶段 2：所有 Module.ResolvePorts（此时端口已就绪，避免循环依赖）
           → 阶段 3：所有 Module.RegisterDriving（绑定 HTTP/CLI/Cron）
           → 阶段 4：Start（启动后台任务）
           → 等待信号
           → 反向 Shutdown
```

配置加载：环境变量 > 配置文件 > 默认值；密钥仅来自环境变量或外部 secret manager。

### 11.1 CLI 子命令归位

当前 `cmd/internal/{agent,tool,skill,session,mcp,monitor,plugin,cron,login,...}/` 是基于 `apiclient` 调 HTTP 的 BFF 风格 CLI 子命令；它们**不属于业务 Context**，归入 `cmd/aranea/cli/`：

```text
cmd/aranea/
├── main.go                      # 入口（§11）
├── launcher/                    # 服务端装配（console/web/full/agent）
└── cli/                         # 客户端子命令（旧 cmd/internal/* 的目标位置）
    ├── root.go                  # cobra root
    ├── apiclient/client.go      # HTTP / SSE 客户端
    ├── output/output.go         # 表格 / JSON / YAML 渲染
    ├── completion/completion.go # 补全
    ├── login/login.go           # 鉴权
    ├── config/config.go         # CLI 配置（与服务端 cfg 不同）
    └── cmd/                     # 一个 Context = 一个文件
        ├── agent_cmd.go         # 旧 cmd/internal/agent/agent.go
        ├── tool_cmd.go
        ├── skill_cmd.go
        ├── session_cmd.go
        ├── mcp_cmd.go
        ├── plugin_cmd.go
        ├── cron_cmd.go
        └── monitor_cmd.go
```

约束：

- CLI 子命令**只调 HTTP API**，不直接 import 任何 `internal/<context>/...`；CLI 进程与服务端进程独立。
- CLI 命名与 Context 一一对应；新增 Context 时配套新增一个 `<context>_cmd.go`。
- `launcher/` 与 `cli/` 共用 `cmd/aranea/` 顶层但 import 树**互不依赖**——CLI 二进制不应链接服务端代码。
- `console` launcher（CLI-only 装配）走的是「服务端管理脚本」路径，与 `cli/` 是两件事：前者直连 application，后者只调 HTTP。

---

## 12. 演进策略

### 12.1 与现有实现的差异（迁移路径）

新架构不是"重写一切"，而是**目标态**。以下迁移按可独立合并的小步推进：

1. **建立 Kernel**：把 `internal/runtime/runtime_context.go`、`internal/domain/errors.go`、未来事件信封提到 `internal/kernel/`；先建包、再逐步迁移引用。
2. **抽取 contracts**：把当前 service 之间互相依赖的接口（如 `repository.Store`、各 service Reader）按本章 §5 重新归类到 `kernel/contracts/`，作为 Context 间唯一通道。
3. **拆分 runtime → adkruntime**：`internal/runtime/` 重构为 `internal/conversation/adapters/adkruntime/`；现有 `tools_bridge`、`adk_plugin_*`、`adk_model_adapter` 全部迁入此处。
4. **Capability 整合**：把 `tools/`、`skills/`（待建）、`mcp/`（待建）、`plugin/hook` 合并到 `internal/capability/`，对外只通过 `kernel/contracts` 暴露 Resolver/Executor。
5. **Memory 重组**：把 `service/memory_l*.go` 与 `domain/memory_l*.go` 重组为 `internal/memory/`，对外只通过 `MemoryAssembler` / `MemoryRecall` 端口。
6. **Operations 集中**：cron、token（计费部分）、监控、审计统一到 `internal/operations/`。
7. **Catalog 收敛**：agents、agent_evolution、avatar 合并到 `internal/catalog/`。
8. **Identity 抽离**：从 team / token 中提炼 Identity。

每一步迁移都应保留旧 import 的 alias（`v1` 包），完成后再删旧路径，避免大爆炸 PR。

#### 12.1.1 文件级迁移映射表

下表的每一行**至多对应一个 PR**：粒度细到一对 `.go + _test.go`，AI 应严格按此表把现有文件搬到目标位置，不要在迁移过程中顺手改实现。`*` 表示同名通配（如 `memory_l*` 涵盖 L0~L4 的全部文件）。

| 现路径（迁移源）                                              | 目标路径                                                                                       | 备注                                                |
| ------------------------------------------------------------- | --------------------------------------------------------------------------------------------- | --------------------------------------------------- |
| `internal/runtime/runtime_context.go`                         | `internal/kernel/runctx/runtime_context.go`                                                    | 字段只增不删                                         |
| `internal/domain/errors.go`                                   | `internal/kernel/errs/{codes.go,error.go}`                                                     | 拆码与类型                                           |
| `internal/domain/models.go`                                   | 各 Context `domain/`                                                                           | 按聚合切分，不要整文件搬                             |
| `internal/runtime/adk_*.go`                                   | `internal/conversation/adapters/adkruntime/`                                                   | 唯一允许 import `google.golang.org/adk` 的位置       |
| `internal/runtime/tools_bridge.go`                            | `conversation/adapters/adkruntime/tool_bridge.go` + `capability/adkbridge/adapter.go`          | 拆为「桥接 ADK」与「桥接到业务 executor」两步       |
| `internal/runtime/tool_policy.go`                             | `internal/capability/middleware/policy.go`                                                     | 转 middleware                                       |
| `internal/tools/**` 全树                                      | `internal/capability/{tooldef,toolctx,middleware,executor,registry,backends,adkbridge,schema,telemetry}/` | 按子包整树搬，文件名保持不变               |
| `internal/tools/storage/sqlite.go` + `internal/repository/sqlite_tools.go` | `internal/capability/adapters/sqlite/repository_tool.go`                          | 二者合一                                             |
| `internal/repository/sqlite_skills.go`                        | `internal/capability/adapters/sqlite/repository_skill.go`                                      | —                                                   |
| `internal/service/tool_service.go`                            | `internal/capability/application/tool_service.go`                                              | —                                                   |
| `internal/service/agent_service.go`                           | `internal/catalog/application/agent_service.go`                                                | —                                                   |
| `internal/service/agent_evolution_*.go`                       | `internal/catalog/application/{evolution_service.go,evolution_scanner.go}`                     | —                                                   |
| `internal/repository/sqlite_agent_evolution.go`               | `internal/catalog/adapters/sqlite/repository_evolution.go`                                     | —                                                   |
| `internal/repository/sqlite_agents.go`                        | `internal/catalog/adapters/sqlite/repository_agent.go`                                         | —                                                   |
| `internal/repository/avatar.go`                               | `internal/catalog/adapters/sqlite/repository_avatar.go`                                        | —                                                   |
| `internal/service/memory_l*_service.go`                       | `internal/memory/application/l*_service.go`                                                    | —                                                   |
| `internal/service/memory_l4_extractor.go`                     | `internal/memory/application/l4_extractor.go`                                                  | —                                                   |
| `internal/service/pii_filter.go`                              | `internal/memory/application/pii_filter.go`                                                    | 跨 Context 复用时再上提 `kernel/pkg/pii/`            |
| `internal/domain/memory_l*.go`                                | `internal/memory/domain/l*.go`                                                                 | —                                                   |
| `internal/repository/sqlite_memory_l*.go`                     | `internal/memory/adapters/sqlite/repository_l*.go`                                             | —                                                   |
| `internal/transport/memory_l*.go`                             | `internal/memory/adapters/http/handler_l*.go`                                                  | —                                                   |
| `internal/service/chat_service.go`                            | `internal/conversation/application/run_turn_handler.go`                                        | 拆为命令处理器                                       |
| `internal/service/session_service.go`                         | `internal/conversation/application/session_service.go`                                         | —                                                   |
| `internal/service/team_runtime.go`                            | `internal/conversation/application/team_runtime_service.go`                                    | —                                                   |
| `internal/repository/sqlite_sessions.go` / `sqlite_messages.go` | `internal/conversation/adapters/sqlite/{repository_session.go,repository_message.go}`        | —                                                   |
| `internal/transport/sessions.go` / `messages.go`              | `internal/conversation/adapters/http/{session_handler.go,message_handler.go}`                  | —                                                   |
| `internal/transport/agent_evolution.go`                       | `internal/catalog/adapters/http/evolution_handler.go`                                          | —                                                   |
| `internal/transport/tools.go`                                 | `internal/capability/adapters/http/tool_handler.go`                                            | —                                                   |
| `internal/transport/handler.go` / `response.go`               | `internal/kernel/pkg/httpx/{response.go,error.go}`                                             | 通用响应工具上提至 kernel                            |
| `internal/middleware/cors.go`                                 | `internal/app/middleware/cors.go`                                                              | 全局 driving 中间件                                 |
| `internal/repository/sqlite_cron.go`                          | `internal/operations/adapters/sqlite/repository_cron.go`                                       | —                                                   |
| `internal/repository/sqlite_cli_admin_seeds.go` / `sqlite_seeds.go` | 拆到各 Context `adapters/sqlite/seeds.go`                                                | 按 seed 内容归属拆分                                 |
| `internal/repository/contracts.go`                            | 公共部分上提 `internal/kernel/contracts/`；Context 私有部分留 `<context>/ports/output.go`      | 严守一章 §5「Kernel 进入门槛 ≥3 Context」              |
| `internal/repository/sqlite.go`                               | `internal/kernel/pkg/db/{open.go,migrate.go}` + 各 Context `adapters/sqlite/init.go`           | 拆分连接池打开与迁移驱动                             |
| `internal/server/server.go`                                   | `internal/app/{router.go,bootstrap.go}` + `cmd/aranea/main.go`                                 | server / router / 装配三段切开                       |

#### 12.1.2 单步迁移操作守则（AI 必读）

每一行映射表的迁移按以下五步执行，**不要把多步合并到一个 PR**：

1. **建壳**：在目标 Context 下创建空的 `module.go` + 4 子层目录 + `migrations/` 占位文件；提交可以是空 PR。
2. **搬文件**：用 `git mv` 把映射表一行内的文件 / 测试对搬到目标路径，**不要顺手改实现**——只动 `package` 名、`import` 路径、`receiver` 包前缀。
3. **过编译**：在新位置 `goimports` 重排 + `go build ./...` + `go test ./...`，确保新旧路径同时可用。
4. **接端口**：若该文件涉及跨 Context 协作，把对外接口提到 `kernel/contracts/<context>.go`；旧使用方改成 `reg.ResolveXxx()` 消费 contracts，**不再直接 import 对方包**。
5. **删旧位**：旧路径只保留一行 `// Deprecated: moved to <new path>` 兼容 alias；当全仓引用归零（CI 静态检查 + `go list -deps` 验证），下个 PR 删除空文件。

**红线**（任何 PR 触碰即拒绝合并）：

- 在迁移过程中**新增**逻辑、修改业务行为、合并多个映射行。
- 把 `google.golang.org/adk` 引入到 `adkruntime/` 之外的包。
- 在 `<context>/domain` 或 `<context>/application` 中 import 其它 Context 包（必须经 `kernel/contracts`）。
- 在 `kernel/**` 中 import `internal/<context>/...`。

#### 12.1.3 单 PR Runbook（AI 可机械执行）

每条迁移行 = 一个 PR；**严格按以下命令序列执行**，不要替换或合并步骤。`<row>` 是 `aranea/docs/migration-status.md` 中的行号（如 `1`）；`<slug>` 是简短英文标识（如 `runctx-to-kernel`）。

**Pre-flight（迁移前 5 分钟）**：

```bash
# 1. 同步主分支
git checkout main && git pull --ff-only

# 2. 读迁移状态文件，找下一个 todo 行
#    AI 必须 cat aranea/docs/migration-status.md 并选定一行
#    若无下一个 todo（或全部 blocked），停手报告

# 3. 基线绿色断言
cd aranea/backend
go build ./...
go test ./... -count=1
go vet ./...

# 4. 创建工作分支
git checkout -b migrate/row-<row>-<slug>
```

**Execute（按 §12.1.2 五步搬代码）**：

```bash
# 5. 若目标目录尚未建壳：先在本 PR 第一个 commit 内建壳
#    （Module 模板见附录 A；commit msg: "scaffold: <context> module shell"）

# 6. 用 git mv 搬文件（一对 .go + _test.go 一起）
git mv internal/<old>/<file>.go      internal/<context>/<sub>/<file>.go
git mv internal/<old>/<file>_test.go internal/<context>/<sub>/<file>_test.go

# 7. 改包名 + import：在新文件顶部 package 改为目标包名
#    用 cursor 全仓搜索替换旧 import 路径 → 新 import 路径
#    然后 goimports 重排
goimports -w internal/<context>/<sub>/

# 8. 若涉及跨 Context 协作：
#    a) 把对外接口提到 internal/kernel/contracts/<context>.go
#    b) 在原使用方改成 reg.ResolveXxx() 消费 contracts
#    c) 在本 Context module.go 的 RegisterPorts 中注册实现

# 9. 旧路径只保留 Deprecated alias 文件（模板见附录 A.4）
```

**Verify（迁移后必跑 4 个绿）**：

```bash
go build ./...                     # 必须绿
go test ./... -count=1             # 必须绿（含旧位与新位测试）
go vet ./...                       # 必须绿
golangci-lint run --timeout=3m     # 必须绿（红线规则见附录 B）
```

**Commit & Track**：

```bash
# 10. 提交（commit msg 模板）
git add -A
git commit -m "migrate(row-<row>): <old> → <new>

Per docs/design/platform-architecture.md §12.1.1 row #<row>.
Source:      <old path>
Destination: <new path>
Cross-context port: <yes/no; 若 yes 列出 contracts 接口名>
Deprecated alias: <yes/no>"

# 11. 更新进度
#     编辑 aranea/docs/migration-status.md：
#     - 把对应行 ☐ 改成 ☑，写 PR 链接 + 日期
#     - 在「操作日志」表追加一行
git add aranea/docs/migration-status.md
git commit -m "track(row-<row>): mark done"

# 12. push & PR
git push -u origin HEAD
gh pr create --fill   # 标题以 "migrate(row-<row>):" 开头
```

**卡住时**：

- 若 step 7 import 改写后出现循环依赖 → 立即停手；在 `migration-status.md` 的「卡点登记」表新增一行；不要尝试在迁移 PR 中调整业务结构。
- 若 step 8 发现需要的 `kernel/contracts` 接口不存在 → 拆成两个 PR：先在「Kernel 上提」阶段加接口，再回到本行。
- 若 verify 4 个绿任一项红 → 回滚到 `git mv` 之前的 commit，分析根因后重新走流程，不允许 `--no-verify` 强推。

**严禁**：

- 在一个 PR 内同时迁移多行映射表。
- 在迁移 PR 中修改业务行为、新增功能、重命名变量、调整签名。
- 跳过 `go test`、`go vet`、`golangci-lint` 任一步。
- 在没有 Deprecated alias 的情况下直接删除旧文件（除非 Alias 清理阶段 P8）。

### 12.2 长期演进

- **从单机到分布式**：所有 Context 通过 `kernel/contracts` 通信，未来把某 Context 的 adapter 替换为 RPC 客户端即可独立成服务。
- **从 SQLite 到 Postgres**：所有 SQL 写在 adapter 内，禁止方言泄露到 application。
- **从 go-adk 到多运行时**：新增 `adapters/<other>runtime/` 实现 `ConversationRuntime` 端口；业务无感知。
- **从单租户到多租户**：`runctx.RuntimeContext.TenantID` 是 Day 1 字段，所有 storage 强制按 `tenant_id` 索引。
- **从 vector 单实现到多向量库**：Memory 通过 `VectorStore` 端口接入，pgvector / Milvus / Qdrant 都是 adapter。

### 12.3 多租户 Day 1 强制约束

`runctx.RuntimeContext.TenantID` 不是「将来再加」的字段，是**立即生效**的硬约束。任何违反以下条款的 PR 不允许合并；CI 必须加入对应静态检查：

1. **Schema**：所有持久表必须包含 `tenant_id TEXT NOT NULL` 列与 `(tenant_id, ...)` 复合索引；公共基础表（如 `schema_migrations`）除外。
2. **查询谓词**：Repository 的所有 `SELECT/UPDATE/DELETE` 必须带 `WHERE tenant_id = ?`；缺失即 P0 缺陷。
3. **写入约束**：插入时 `tenant_id` 由 `runctx` 注入，**禁止**从请求体读取（防止越权）。
4. **事件信封**：`Envelope.actor.tenant_id` 必填；订阅方在跨租户聚合前必须按 `tenant_id` 分桶。
5. **跨租户访问**：仅允许「租户管理员」级别身份显式声明 `cross_tenant=true` 时通过；并写 `operations_audit_log`。
6. **缓存键**：缓存键必须包含 `tenant_id`；否则禁止启用缓存装饰器（三章 §7）。
7. **测试矩阵**：每个 Context 至少一条「跨租户隔离」用例（A 租户写入，B 租户应读不到）。
8. **日志/事件不脱敏字段**：`tenant_id` 始终可见；`user_id` / `session_id` 在多租户复盘时仍要保留以便定位问题。

---

## 13. 章节导航

本章是**总纲**，定义 Context 划分、Kernel 范围、端口契约、依赖方向、ADK 集成边界。后续章节展开实施细节：

- **二、路由设计**：Driving HTTP Adapter 的统一装配（Module 接口、显式聚合、OpenAPI、生命周期）。
- **三、数据库设计**：Driven SQL Adapter 的统一约束（连接池、表前缀按 Context、Repository、迁移、事务边界）。
- **四、模块功能设计**：专论 **Capability Context** 内「可运行能力」执行子系统（registry/executor/middleware/backends/adkbridge/schema/telemetry）；其它 Context 以一章 §5 四子层为主。

新增专题文档（`aranea/docs/<NN> <topic>.md`）必须在开头声明：（a）所属 Context；（b）涉及的 Kernel 端口；（c）若与本章 §2~§9 偏离，给出理由与还债计划。

---

# 二、路由设计

本章描述 **Driving HTTP Adapter** 的统一装配：每个**限界上下文（Context）**实现「一、主体架构设计」§11 的 **`module.Module` 四阶段协议**。HTTP 只是 `RegisterDriving` 阶段挂载的一种 driving 形态；CLI、Cron、A2A 等同理扩展 `DrivingRegistry`，本章以 chi + `/api/v1` 为主。

核心目标：Context 内部自治路由与中间件；launcher/app 只负责**端口注册顺序**、**显式模块列表**、**OpenAPI 聚合**与**生命周期**，保持可预测、可测试、可条件编译。

---

## 1. 设计原则

1. **统一 Module 协议**：所有 Context 实现同一套 `module.Module`（四阶段），launcher 只依赖该抽象（见 §2）。
2. **Context 内聚**：HTTP 子路由、子中间件、handler、service、storage 留在各 Context 的 `adapters/http` 与 `application` 内，不泄漏到全局。
3. **显式装配**：Context 列表在 `app/modules.go`（或等价处）显式写出；禁止 `init()` 自动注册。
4. **launcher 极简**：加载配置 → 构建 `kernel/contracts.Registry` → 实例化 Module → **阶段 1~4**（见 §6）→ 监听信号 → 反向 `Shutdown`。
5. **文档随 Context 交付**：每个 Context 在 `RegisterDriving` 中登记自己的 OpenAPI 片段，由边缘层聚合（见 §2 `OpenAPISpec`）。
6. **生命周期可控**：`Start` / `Shutdown` 成对；HTTP Server 关闭后再 `Shutdown` Module 列表。

---

## 2. Module 接口（与一章 §11 对齐）

每个 Context 的壳（`module.go`）实现以下接口。**`RegisterPorts` / `ResolvePorts` 解决跨 Context 依赖；`RegisterDriving` 仅挂载本 Context 的 HTTP（或其它 driving）；`Start` 启动后台任务**。

```go
package module

import (
	"context"
	"encoding/json"

	"github.com/go-chi/chi/v5"

	"arenea/backend/internal/kernel/contracts"
)

// DrivingRegistry 由 app 实现：在 RegisterDriving 阶段注入，封装 /api/v1 挂载点与 OpenAPI 收集。
type DrivingRegistry interface {
	// WithAPIV1 在全局 /api/v1 分组上注册路由（fn 收到的 r 已带 /api/v1 前缀时可按需再 Route）
	WithAPIV1(fn func(r chi.Router))
	// RegisterOpenAPISpec 登记本 Context 的 OpenAPI JSON 片段（可多次调用，按 Name 去重）
	RegisterOpenAPISpec(name string, spec json.RawMessage)
}

// Module 与「一、主体架构设计」§11 一致。
type Module interface {
	Name() string   // 稳定标识：identity | catalog | capability | conversation | memory | operations
	Version() string

	// 阶段 1：向 kernel 注册本 Context 对外提供的端口实现（output ports）
	RegisterPorts(reg *contracts.Registry)
	// 阶段 2：从 kernel 解析本 Context 依赖的端口（input ports）；此时所有 RegisterPorts 已完成
	ResolvePorts(reg *contracts.Registry)

	// 阶段 3：挂载 HTTP 等 driving 适配器（仅副作用，不启动长任务）
	RegisterDriving(d DrivingRegistry)

	// 阶段 4：启动后台任务（cron、watcher、订阅 event bus）；HTTP Listen 可在 Start 前后由 launcher 决定
	Start(ctx context.Context) error
	Shutdown(ctx context.Context) error

	// OpenAPISpec 可选：若 OpenAPI 以 embed 文件维护，仍通过 RegisterDriving 登记；此方法供测试与静态检查直接读取
	OpenAPISpec() (json.RawMessage, error)
}
```

接口职责边界：

- `Name()` / `Version()`：健康检查、日志、OpenAPI `info` 扩展、迁移模块名（与三章 `schema_migrations.module` 建议一致，使用 Context 名）。
- `RegisterPorts`：例如 Capability 注册 `ToolResolver`、`ToolExecutor` 等实现到 `contracts.Registry`。
- `ResolvePorts`：例如 Conversation 从 `reg` 取出 `Catalog.AgentReader`、`Capability.ToolResolver`、`Memory.MemoryAssembler`。
- `RegisterDriving`：在 `WithAPIV1` 回调里挂载 `/tools`、`/sessions` 等路由；**不得**在此启动 goroutine 长任务（放到 `Start`）。
- `Start` / `Shutdown`：与 HTTP 解耦；`Shutdown` 须幂等、带超时由 launcher 控制。
- `OpenAPISpec()`：返回本 Context 的 OpenAPI 片段；聚合逻辑在 app 边缘层（见 §6）。

---

## 3. 推荐目录结构（按 Context · 全量展开）

下列结构是**目标态**：每个 Context 自含 4 子层（`domain` / `application` / `ports` / `adapters`），HTTP 仅在 `adapters/http`，SQL 仅在 `adapters/sqlite`，外部运行时（如 ADK）仅在专用 adapter 子包。**Capability** 因承载执行子系统，多出一章 §四 中的 `tooldef / toolctx / middleware / executor / registry / backends / adkbridge / schema / telemetry`；其它 Context 默认无该子系统。

```text
backend/
├── cmd/aranea/
│   ├── main.go                              # 信号 / Listen / Shutdown 编排
│   └── launcher/                            # 一章 §11 装配模式
│       ├── config.go                        # 加载 cfg + ENV + secret
│       ├── console/console.go               # CLI-only 装配
│       ├── web/web.go                       # HTTP + SSE
│       ├── full/full.go                     # HTTP + CLI + Cron
│       └── agent/agent.go                   # A2A / Webhook headless
└── internal/
    ├── kernel/                              # 一章 §5：跨 Context 共享内核（不依赖任何 Context）
    │   ├── ids/{ulid.go,slug.go}            # ID 生成 / 解析
    │   ├── clock/{clock.go,fake.go}         # 时间抽象 + 可注入 fake clock
    │   ├── errs/{codes.go,error.go}         # 错误码常量 + Domain Error + Wrap/Is
    │   ├── event/{envelope.go,bus.go,memory_bus.go}   # 事件信封 + Bus 接口 + 内存实现
    │   ├── runctx/runtime_context.go        # tenant/user/session/agent/trace/budget
    │   ├── contracts/                       # 跨 Context 端口集合（唯一允许的协作通道）
    │   │   ├── registry.go                  # Registry 类型 + 注册/解析方法
    │   │   ├── identity.go                  # IdentityReader / APIKeyVerifier
    │   │   ├── catalog.go                   # AgentReader
    │   │   ├── capability.go                # ToolResolver / ToolExecutor / SkillResolver / ProviderResolver
    │   │   ├── memory.go                    # MemoryAssembler / MemoryRecall / MemoryWriter
    │   │   ├── conversation.go              # ConversationRuntime / TurnRequest 等共享类型
    │   │   └── operations.go                # UsageReader / AuditWriter / JobScheduler
    │   ├── module/module.go                 # Module + DrivingRegistry 接口（本章 §2）
    │   ├── telemetry/{tracer.go,meter.go,names.go}    # OTel 包装 + instrumentation 名约定
    │   └── pkg/                             # 中性工具（无业务语义）
    │       ├── httpx/                       # 通用响应、错误码 → HTTP 映射
    │       ├── jsonutil/
    │       ├── schemagen/                   # typed Go struct → JSON Schema
    │       ├── validate/
    │       └── db/                          # 连接池打开 + 迁移执行器
    │
    ├── app/                                 # 装配层（不是业务）；本章 §5、§6、三章 §5
    │   ├── container.go                     # *sql.DB / Logger / Telemetry / Clock / EventBus
    │   ├── modules.go                       # InitModules — 显式 Context 列表（§5）
    │   ├── bootstrap.go                     # BootstrapPorts / StartModules / ShutdownModules
    │   ├── router.go                        # NewRouter + drivingRegistry（§6）
    │   ├── middleware/                      # 全局 driving 中间件
    │   │   ├── cors.go
    │   │   ├── recover.go
    │   │   └── auth_entry.go                # 认证入口（具体校验委托到 identity）
    │   ├── migrations.go                    # MigrationSources / RunMigrations（三章 §5）
    │   └── openapi.go                       # MergeOpenAPISpecs
    │
    ├── identity/                            # 用户 / 团队 / 工作区 / 角色 / API Key / 配额
    │   ├── module.go                        # 实现 module.Module 四阶段
    │   ├── domain/
    │   │   ├── user.go                      # User 聚合
    │   │   ├── team.go                      # Team 聚合
    │   │   ├── workspace.go                 # Workspace 聚合
    │   │   ├── role.go                      # Role / Permission 值对象
    │   │   ├── apikey.go                    # APIKey 聚合
    │   │   ├── errors.go                    # 本 Context sentinel error
    │   │   └── events.go                    # identity.user.created 等事件
    │   ├── application/                     # 用例（Command/Query handler）
    │   │   ├── user_service.go
    │   │   ├── team_service.go
    │   │   ├── workspace_service.go
    │   │   └── apikey_service.go
    │   ├── ports/
    │   │   ├── input.go                     # 暴露给 adapter 的 Service 接口
    │   │   └── output.go                    # 依赖项（Repo / AuditWriter / Clock）
    │   └── adapters/
    │       ├── http/
    │       │   ├── router.go                # MountRoutes(r chi.Router)
    │       │   ├── user_handler.go
    │       │   ├── team_handler.go
    │       │   ├── apikey_handler.go
    │       │   └── dto.go                   # 入/出参结构 + JSON tag
    │       └── sqlite/
    │           ├── repository_user.go
    │           ├── repository_team.go
    │           ├── repository_workspace.go
    │           ├── repository_apikey.go
    │           ├── model.go                 # SQL row 结构（不外泄）
    │           ├── seeds.go                 # 内置管理员 / 默认角色
    │           └── migrations/              # 表前缀 identity_*
    │               └── 000001_identity_init.up.sql
    │
    ├── catalog/                             # Agent 编目 / 版本 / 演化 / 头像 / 提示资产
    │   ├── module.go
    │   ├── domain/
    │   │   ├── agent.go                     # Agent 聚合
    │   │   ├── agent_version.go             # 版本快照
    │   │   ├── avatar.go
    │   │   ├── evolution.go                 # 演化提案
    │   │   ├── prompt_asset.go              # 提示文件 / 模板
    │   │   ├── errors.go
    │   │   └── events.go                    # catalog.agent.evolution.proposed 等
    │   ├── application/
    │   │   ├── agent_service.go
    │   │   ├── version_service.go
    │   │   ├── evolution_service.go
    │   │   ├── evolution_scanner.go         # 订阅 conversation.turn.completed
    │   │   └── avatar_service.go
    │   ├── ports/{input.go,output.go}       # 注册 AgentReader 到 kernel/contracts
    │   └── adapters/
    │       ├── http/{router.go,agent_handler.go,evolution_handler.go,avatar_handler.go,dto.go}
    │       └── sqlite/
    │           ├── repository_agent.go
    │           ├── repository_evolution.go
    │           ├── repository_avatar.go
    │           ├── model.go
    │           └── migrations/              # 表前缀 catalog_*
    │
    ├── capability/                          # Tool / Skill / MCP / Plugin / Provider — 详见一章 §四
    │   ├── module.go
    │   ├── domain/
    │   │   ├── tool.go
    │   │   ├── skill.go
    │   │   ├── mcp_server.go
    │   │   ├── plugin.go
    │   │   ├── provider.go
    │   │   ├── errors.go
    │   │   └── events.go                    # capability.tool.executed 等
    │   ├── application/
    │   │   ├── tool_service.go              # Tool CRUD + 启停
    │   │   ├── skill_service.go             # Skill 导入 / 激活
    │   │   ├── mcp_service.go
    │   │   ├── provider_service.go
    │   │   └── tool_engine.go               # registry + executor 的应用层门面
    │   ├── ports/{input.go,output.go}       # 注册 ToolResolver/ToolExecutor/SkillResolver/ProviderResolver
    │   ├── adapters/
    │   │   ├── http/{router.go,tool_handler.go,skill_handler.go,mcp_handler.go,provider_handler.go,dto.go}
    │   │   └── sqlite/
    │   │       ├── repository_tool.go
    │   │       ├── repository_skill.go
    │   │       ├── repository_mcp.go
    │   │       ├── repository_provider.go
    │   │       ├── model.go
    │   │       └── migrations/              # 表前缀 capability_*
    │   ├── tooldef/                         # §四 §1：能力领域契约
    │   │   ├── tool.go                      # Tool / StreamingTool / ApprovableTool
    │   │   └── skill.go
    │   ├── toolctx/toolctx.go               # §四 §2：执行上下文
    │   ├── middleware/                      # §四 §3：横切关注点
    │   │   ├── chain.go
    │   │   ├── validation.go
    │   │   ├── budget.go
    │   │   ├── policy.go                    # 权限 / 可见性
    │   │   ├── audit.go
    │   │   ├── tracing.go
    │   │   └── rate_limit.go
    │   ├── executor/executor.go             # §四 §4：唯一运行入口
    │   ├── registry/                        # §四 §5
    │   │   ├── registry.go
    │   │   └── builtin.go
    │   ├── backends/                        # §四 §6：一个文件 = 一类后端
    │   │   ├── base.go
    │   │   ├── filesystem.go
    │   │   ├── utility.go
    │   │   └── ...
    │   ├── schema/                          # §四 §7：typed input/output → JSON schema
    │   │   ├── schema.go
    │   │   ├── filesystem.go
    │   │   ├── utility.go
    │   │   └── weather.go
    │   ├── adkbridge/adapter.go             # §四 §8：tooldef → adk.Tool 的薄层（不含业务）
    │   └── telemetry/provider.go            # §四 §9：本 Context instrumentation name
    │
    ├── conversation/                        # 会话 / 消息 / 通道 / 多 Agent / 流式
    │   ├── module.go
    │   ├── domain/
    │   │   ├── session.go
    │   │   ├── message.go
    │   │   ├── channel.go
    │   │   ├── team_run.go
    │   │   ├── turn.go                      # 一次 RunTurn 的领域对象
    │   │   ├── errors.go
    │   │   └── events.go                    # conversation.turn.completed 等
    │   ├── application/
    │   │   ├── run_turn_handler.go          # 一章 §10.1 同步路径主用例
    │   │   ├── session_service.go
    │   │   ├── message_service.go
    │   │   ├── team_runtime_service.go
    │   │   └── channel_service.go
    │   ├── ports/
    │   │   ├── input.go
    │   │   └── output.go                    # ConversationRuntime 输出端口（被 adkruntime 实现）
    │   └── adapters/
    │       ├── http/{router.go,session_handler.go,message_handler.go,sse_handler.go,channel_handler.go,dto.go}
    │       ├── sqlite/
    │       │   ├── repository_session.go
    │       │   ├── repository_message.go
    │       │   ├── repository_channel.go
    │       │   ├── repository_team_run.go
    │       │   ├── model.go
    │       │   └── migrations/              # 表前缀 conversation_*
    │       └── adkruntime/                  # 一章 §6：唯一允许 import google.golang.org/adk
    │           ├── runner.go                # 实现 ports.ConversationRuntime
    │           ├── agent_builder.go         # Catalog/Capability/Memory 视图 → adk.Agent
    │           ├── tool_bridge.go           # ADK before_tool callback → capability.ExecuteTool
    │           ├── skill_bridge.go
    │           ├── memory_bridge.go         # 实现 adk.memory.Service，路由到 contracts.MemoryAssembler
    │           ├── model_bridge.go          # ProviderResolver → adk.model.Model
    │           └── plugins/                 # adk.Plugin：审计 / 脱敏 / 用量 / 超时 / tracing
    │               ├── audit.go
    │               ├── budget.go
    │               ├── pii.go
    │               └── tracing.go
    │
    ├── memory/                              # L0~L4 五层记忆体系
    │   ├── module.go
    │   ├── domain/
    │   │   ├── l0_snapshot.go               # 感知层
    │   │   ├── l1_slot.go                   # 工作记忆
    │   │   ├── l2_episode.go                # 情景记忆
    │   │   ├── l3_fact.go                   # 语义记忆
    │   │   ├── l4_node.go                   # 持久 / 图记忆
    │   │   ├── view.go                      # 跨层聚合 MemoryView
    │   │   ├── errors.go
    │   │   └── events.go
    │   ├── application/
    │   │   ├── l0_service.go
    │   │   ├── l1_service.go
    │   │   ├── l2_service.go
    │   │   ├── l3_service.go
    │   │   ├── l4_service.go
    │   │   ├── l4_extractor.go              # 抽取持久知识
    │   │   ├── pii_filter.go                # 写入前脱敏
    │   │   ├── assembler.go                 # 实现 contracts.MemoryAssembler
    │   │   ├── recall.go                    # 实现 contracts.MemoryRecall
    │   │   ├── consolidate.go               # L2→L3 / L3→L4
    │   │   ├── decay.go                     # 衰减策略
    │   │   └── event_handlers.go            # 订阅 conversation.turn.completed 写 L0/L2
    │   ├── ports/
    │   │   ├── input.go
    │   │   └── output.go                    # VectorStore / EmbedClient 等可插拔依赖
    │   └── adapters/
    │       ├── http/{router.go,l0_handler.go,l1_handler.go,l2_handler.go,l3_handler.go,l4_handler.go,dto.go}
    │       ├── sqlite/
    │       │   ├── repository_l0.go
    │       │   ├── repository_l1.go
    │       │   ├── repository_l2.go
    │       │   ├── repository_l3.go
    │       │   ├── repository_l4.go
    │       │   ├── model.go
    │       │   └── migrations/              # 表前缀 memory_*
    │       └── vectorstore/                 # 可插拔向量库
    │           ├── pgvector.go
    │           ├── memory.go                # 内存实现，开发用
    │           └── factory.go
    │
    └── operations/                          # 调度 / 监控 / 审计 / 用量 / 成本 / 生态
        ├── module.go
        ├── domain/
        │   ├── job.go
        │   ├── schedule.go
        │   ├── audit_log.go
        │   ├── usage_event.go
        │   ├── cost_ledger.go
        │   ├── monitor_event.go
        │   ├── errors.go
        │   └── events.go
        ├── application/
        │   ├── cron_service.go
        │   ├── audit_service.go             # 实现 contracts.AuditWriter
        │   ├── usage_service.go             # 实现 contracts.UsageReader
        │   ├── billing_service.go
        │   ├── monitor_service.go
        │   └── event_handlers.go            # 订阅所有 *.executed 事件做审计与计费
        ├── ports/{input.go,output.go}
        └── adapters/
            ├── http/{router.go,job_handler.go,audit_handler.go,usage_handler.go,monitor_handler.go,dto.go}
            ├── sqlite/
            │   ├── repository_job.go
            │   ├── repository_audit.go
            │   ├── repository_usage.go
            │   ├── repository_cost.go
            │   ├── model.go
            │   └── migrations/              # 表前缀 operations_*
            └── cron/                        # driving adapter：定时触发 cron_service
                ├── runner.go
                └── schedule.go
```

**通读这棵树时记住三条**：

- 每个 Context 自含 4 子层；上下文之间无文件级耦合（编译期约束见一章 §8）。新增文件**必须**落到目标 Context 的对应子层；旧路径（`internal/repository`、`internal/runtime`、`internal/transport`、`internal/service`、`internal/tools`）**禁止**新增代码。
- HTTP / SQL / ADK / Cron / VectorStore 都是 adapter；新增技术栈 = 新增 `adapters/<name>/` 子包，**不动** `domain` / `application` / `ports`。
- 旧实现到目标位置的逐文件映射见一章 §12.1「文件级迁移映射表」。

---

## 4. Context 实现示例（Capability）

以 **Capability** Context 为例：`RegisterPorts` 注册 `ToolResolver` 等；`ResolvePorts` 可解析 Identity 的审计端口等；`RegisterDriving` 挂载 `/tools`；`Start` 无长任务则返回 nil。

```go
package capability

import (
	"context"
	_ "embed"
	"encoding/json"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"arenea/backend/internal/kernel/contracts"
	"arenea/backend/internal/module"
)

//go:embed openapi.json
var openAPISpec []byte

type Module struct {
	reg    *contracts.Registry
	deps   Deps
	http   *HTTPAdapter
	engine *ToolEngine // application 层：registry + executor + backends
}

func NewModule(deps Deps) *Module {
	return &Module{deps: deps, http: NewHTTPAdapter(deps), engine: NewToolEngine(deps)}
}

func (m *Module) Name() string { return "capability" }
func (m *Module) Version() string { return "v1" }

func (m *Module) RegisterPorts(reg *contracts.Registry) {
	reg.RegisterToolResolver(m.engine)
	// reg.RegisterSkillResolver(...)
}

func (m *Module) ResolvePorts(reg *contracts.Registry) {
	m.reg = reg
	// m.audit = reg.AuditWriter()
}

func (m *Module) RegisterDriving(d module.DrivingRegistry) {
	d.RegisterOpenAPISpec("capability", openAPISpec)
	d.WithAPIV1(func(r chi.Router) {
		r.Use(chimw.SetHeader("X-Aranea-Context", m.Name()))
		r.Route("/tools", m.http.MountToolsRoutes)
	})
}

func (m *Module) Start(ctx context.Context) error { return nil }

func (m *Module) Shutdown(ctx context.Context) error {
	return m.engine.Shutdown(ctx)
}

func (m *Module) OpenAPISpec() (json.RawMessage, error) {
	return json.RawMessage(openAPISpec), nil
}
```

Context 对外只暴露 `NewModule` 与 `module.Module` 实现。launcher 不依赖 `HTTPAdapter`、`ToolEngine` 的具体类型。

---

## 5. 显式 Context 列表

所有 Context 在 `app/modules.go` 中显式聚合。**顺序建议**：`identity` → `catalog` → `capability` → `memory` → `conversation` → `operations`（Conversation 依赖多端口，放中后；Operations 常订阅事件，可置末）。

```go
package app

import (
	"arenea/backend/internal/catalog"
	"arenea/backend/internal/capability"
	"arenea/backend/internal/conversation"
	"arenea/backend/internal/identity"
	"arenea/backend/internal/memory"
	"arenea/backend/internal/module"
	"arenea/backend/internal/operations"
)

func InitModules(c *Container) []module.Module {
	return []module.Module{
		identity.NewModule(identity.Deps{DB: c.DB, Logger: c.Logger}),
		catalog.NewModule(catalog.Deps{DB: c.DB, Logger: c.Logger}),
		capability.NewModule(capability.Deps{DB: c.DB, Logger: c.Logger, ADK: c.ADKRuntime}),
		memory.NewModule(memory.Deps{DB: c.DB, Logger: c.Logger}),
		conversation.NewModule(conversation.Deps{DB: c.DB, Logger: c.Logger, ADK: c.ADKRuntime}),
		operations.NewModule(operations.Deps{DB: c.DB, Logger: c.Logger}),
	}
}
```

**端口阶段**须在挂载 HTTP 之前完成（见 §6）：

```go
package app

import (
	"arenea/backend/internal/kernel/contracts"
	"arenea/backend/internal/module"
)

func BootstrapPorts(mods []module.Module, reg *contracts.Registry) {
	for _, m := range mods {
		m.RegisterPorts(reg)
	}
	for _, m := range mods {
		m.ResolvePorts(reg)
	}
}
```

显式列表的收益：

- Context 顺序清晰，端口依赖与路由冲突更容易发现。
- 测试可直接替换模块列表，不依赖全局注册状态。
- 条件编译或不同发行版可通过不同文件维护不同 Context 集合。
- 初始化依赖一目了然，避免 `init()` 带来的隐式副作用和导入顺序问题。

条件编译示例：

```go
//go:build enterprise

package app

func InitEnterpriseModules(c *Container) []module.Module {
	return []module.Module{
		operations.NewBillingModule(operations.Deps{DB: c.DB, Logger: c.Logger}),
	}
}
```

---

## 6. 路由注册器与四阶段编排

全局 router 负责通用中间件、基础端点；**阶段 1~2** 在 `NewRouter` 之前由 `BootstrapPorts` 完成；**阶段 3** 在 `NewRouter` 内通过 `DrivingRegistry` 调用各 `RegisterDriving`；**阶段 4** `Start` 在 HTTP `Listen` 前后由 launcher 显式调用（见 §7）。

```go
package app

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"

	"arenea/backend/internal/kernel/contracts"
	"arenea/backend/internal/module"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type drivingRegistry struct {
	apiV1     chi.Router
	openapi   map[string]json.RawMessage
	openapiMu sync.Mutex
}

func newDrivingRegistry(apiV1 chi.Router) *drivingRegistry {
	return &drivingRegistry{apiV1: apiV1, openapi: map[string]json.RawMessage{}}
}

func (d *drivingRegistry) WithAPIV1(fn func(r chi.Router)) { fn(d.apiV1) }

func (d *drivingRegistry) RegisterOpenAPISpec(name string, spec json.RawMessage) {
	d.openapiMu.Lock()
	d.openapi[name] = spec
	d.openapiMu.Unlock()
}

func (d *drivingRegistry) mergedOpenAPI(mods []module.Module) (json.RawMessage, error) {
	return MergeOpenAPISpecs(d.openapi, mods)
}

// NewRouter 假设 BootstrapPorts 已执行完毕。
func NewRouter(modules []module.Module, reg *contracts.Registry) http.Handler {
	root := chi.NewRouter()
	root.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer)

	root.Get("/healthz", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	var dreg *drivingRegistry
	root.Route("/api/v1", func(api chi.Router) {
		dreg = newDrivingRegistry(api)
		for _, mod := range modules {
			mod.RegisterDriving(dreg)
		}
	})

	root.Get("/openapi.json", func(w http.ResponseWriter, req *http.Request) {
		spec, err := dreg.mergedOpenAPI(modules)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(spec)
	})

	return root
}

func StartModules(ctx context.Context, modules []module.Module) error {
	for _, m := range modules {
		if err := m.Start(ctx); err != nil {
			return err
		}
	}
	return nil
}

func ShutdownModules(ctx context.Context, modules []module.Module) error {
	for i := len(modules) - 1; i >= 0; i-- {
		if err := modules[i].Shutdown(ctx); err != nil {
			return err
		}
	}
	return nil
}
```

全局中间件只处理跨 Context 通用能力：request id、panic recovery、CORS、认证入口、日志与 tracing。业务中间件留在各 Context 的 `adapters/http` 或「四、模块功能设计」中的 executor middleware。

**典型 main 顺序**：

```text
cfg, container → reg := contracts.NewRegistry(container)
mods := InitModules(container)
BootstrapPorts(mods, reg)
router := NewRouter(mods, reg)
server.Listen… → StartModules(ctx, mods)
→ Shutdown server → ShutdownModules(ctx, mods)
```

### 6.1 OpenAPI 聚合契约

每个 Context 通过 `RegisterDriving → DrivingRegistry.RegisterOpenAPISpec(name, spec)` 提交**局部** spec；`app/openapi.go.MergeOpenAPISpecs` 按以下规则合并为顶层 `/openapi.json`。

```go
type OpenAPIFragment struct {
    Context string          // = Module.Name()
    Version string          // = Module.Version()
    Spec    json.RawMessage // OpenAPI 3.1 子集，仅含 paths / components / tags
}
```

**合并规则**：

| 字段                                                       | 处理策略                                                                  |
| ---------------------------------------------------------- | ------------------------------------------------------------------------ |
| `info`                                                     | 由 app 顶层注入；忽略 fragment 中的 `info`                                |
| `servers`                                                  | 同上                                                                     |
| `paths`                                                    | 直接合并；冲突（同 path + method）启动期 panic，禁止静默覆盖              |
| `components.schemas`                                       | 强制以 Context 名 PascalCase 为前缀（如 `CapabilityToolCreateRequest`）；冲突 panic |
| `components.parameters` / `responses` / `securitySchemes`  | 同 schemas，前缀去重                                                     |
| `tags`                                                     | 自动生成 `<Context>` tag；handler 路由声明必须带此 tag                    |
| `x-aranea-context`                                         | 顶层每个 path 标注归属 Context，便于网关分流与审计                        |

**校验**：

- 启动期校验失败必须 panic（与 `schema_migrations` 冲突一致），不允许静默降级。
- 测试矩阵必须覆盖：`TestOpenAPIMergeNoConflict`、`TestOpenAPIMergeRejectsDuplicatePath`、`TestOpenAPIMergeRejectsUnprefixedSchema`。
- 每个 Context 用 `//go:embed openapi.json` 内嵌 spec 文件，spec 文件由 typed Go struct 通过 `kernel/pkg/schemagen` 生成（一章 §7.2），**禁止**手写。
- CI 中加入 `openapi-cli validate openapi.json` 检查合并产物合法。

---

## 7. 主程序职责

主程序只保留固定启动流程（**四阶段**与 §6 一致）：

```go
package main

import (
	"context"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"arenea/backend/internal/app"
	"arenea/backend/internal/kernel/contracts"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg := app.LoadConfig()
	container := app.NewContainer(cfg)
	defer container.Close()

	reg := contracts.NewRegistry(container)
	modules := app.InitModules(container)

	app.BootstrapPorts(modules, reg)

	router := app.NewRouter(modules, reg)

	server := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      router,
		ReadTimeout:  cfg.HTTPReadTimeout,
		WriteTimeout: cfg.HTTPWriteTimeout,
	}

	go func() { _ = server.ListenAndServe() }()

	_ = app.StartModules(context.Background(), modules)

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_ = server.Shutdown(shutdownCtx)
	_ = app.ShutdownModules(shutdownCtx, modules)
}
```

主程序职责收敛为：

```text
加载配置 → 创建容器与 contracts.Registry → InitModules
→ BootstrapPorts（RegisterPorts → ResolvePorts）
→ NewRouter（RegisterDriving）
→ Listen → StartModules → 信号退出 → Shutdown
```

优雅退出只负责关闭 HTTP server、调用各 Context `Shutdown`，不承载业务清理细节。

---

## 8. 路由命名与版本约定

推荐统一使用 `/api/v1/{module}` 作为业务 API 前缀：

```text
GET    /healthz
GET    /openapi.json

GET    /api/v1/agents
POST   /api/v1/agents
GET    /api/v1/agents/{agentID}

GET    /api/v1/tools
POST   /api/v1/tools
POST   /api/v1/tools/{toolID}/invoke

GET    /api/v1/skills
POST   /api/v1/skills/import
POST   /api/v1/skills/{skillID}/activate

GET    /api/v1/sessions
POST   /api/v1/sessions
GET    /api/v1/sessions/{sessionID}/events
```

模块接口里的 `Version()` 表示模块实现版本，不直接等于 URL 版本。URL 版本由全局 router 控制，便于后续同时挂载 `/api/v1` 与 `/api/v2`。

---

## 9. 测试策略

路由层测试分两类：

1. **Context 级测试**：`BootstrapPorts` + 构造 `DrivingRegistry` fake，只测本 Context 的 `RegisterDriving` 挂载路径与 handler。
2. **应用级测试**：完整 `InitModules` + `BootstrapPorts` + `NewRouter`，验证 `/api/v1`、健康检查、OpenAPI 聚合、`Start`/`Shutdown` 顺序。

Context 级测试示例：

```go
func TestCapabilityHTTP(t *testing.T) {
	mod := capability.NewModule(fakeDeps())
	reg := contracts.NewRegistry(fakeContainer())
	mod.RegisterPorts(reg)
	mod.ResolvePorts(reg)

	r := chi.NewRouter()
	r.Route("/api/v1", func(api chi.Router) {
		dreg := newDrivingRegistry(api)
		mod.RegisterDriving(dreg)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tools", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
}
```

应用级测试示例：

```go
func TestAppRouterMountsContexts(t *testing.T) {
	reg := contracts.NewRegistry(fakeContainer())
	modules := []module.Module{
		fakecontext.New("alpha", "/alpha"),
		fakecontext.New("beta", "/beta"),
	}
	app.BootstrapPorts(modules, reg)
	router := app.NewRouter(modules, reg)

	assertRoute(t, router, http.MethodGet, "/api/v1/alpha", http.StatusOK)
	assertRoute(t, router, http.MethodGet, "/api/v1/beta", http.StatusOK)
}
```

---

## 10. 结论

Aranea 的路由层以 **`module.Module` 四阶段协议**为唯一跨 Context 装配契约：`RegisterPorts` / `ResolvePorts` 固化依赖方向，`RegisterDriving` 挂载 HTTP（及其它 driving），`Start` / `Shutdown` 管理生命周期。应用层显式列出 Context 顺序并完成端口引导，主进程保持稳定；增删能力等价于增删 Context 或扩展端口注册，无需隐式 `init()`。


# 三、数据库设计

Aranea 的数据库层采用「**全局连接池 + 按 Context 表前缀隔离 + Context 内 Repository**」的结构。核心目标是减少运维复杂度、与「一、主体架构设计」中的**限界上下文**一一对应，并为未来按 Context 拆库 / 拆服务预留边界。

---

## 1. 全局连接管理（单例复用）

整个程序只维护一个数据库连接池，由主 `Container` 持有并注入给各 Context。连接池在 `main` 启动阶段根据配置创建，后续所有 Context 共享同一个 `*sql.DB`。

```go
package app

import (
	"context"
	"database/sql"
	"time"
)

type Container struct {
	Config Config
	DB     *sql.DB
	Logger Logger
}

func NewContainer(cfg Config) *Container {
	db := MustOpenDB(cfg.Database)

	return &Container{
		Config: cfg,
		DB:     db,
		Logger: NewLogger(cfg),
	}
}

func MustOpenDB(cfg DatabaseConfig) *sql.DB {
	db, err := sql.Open(cfg.Driver, cfg.DSN)
	if err != nil {
		panic(err)
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	pingTimeout := cfg.PingTimeout
	if pingTimeout == 0 {
		pingTimeout = 5 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		panic(err)
	}

	return db
}
```

配置建议：

- SQLite 使用单文件数据库，适合本地开发、桌面部署和轻量单实例场景。
- PostgreSQL 使用多连接池，适合服务端部署、多实例和高并发场景。
- 单个连接池同时服务所有 Context，避免多数据库文件带来的备份、迁移和排障成本。
- 跨 Context 事务如果确实需要单数据库原子性，也只有在共享连接池前提下才有实现空间。

SQLite 推荐配置：

```text
driver: sqlite
dsn: file:aranea.db?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=on
max_open_conns: 1
max_idle_conns: 1
conn_max_lifetime: 0
conn_max_idle_time: 0
```

PostgreSQL 推荐配置：

```text
driver: postgres
dsn: postgres://aranea:password@localhost:5432/aranea?sslmode=disable
max_open_conns: 25
max_idle_conns: 5
conn_max_lifetime: 30m
conn_max_idle_time: 5m
```

---

## 2. 按 Context 的表前缀隔离

每个 **限界上下文** 内部自行定义表结构，**表名必须以 Context 前缀开头**，与「一、主体架构设计」§4 的划分一致。默认采用**表名前缀**（兼容 SQLite 与 PostgreSQL）：

```text
identity_*       # 用户、团队、工作区、角色、租户、API Key 等
catalog_*        # Agent 定义、版本、演化、头像、提示资产等
capability_*     # Tool、Skill、MCP、Plugin、Provider 配置等
conversation_*   # Session、Message、Channel、多 Agent 运行实例等
memory_*         # L0~L4 各层存储（快照、槽位、Episode、语义块、图节点等）
operations_*     # 调度、审计、用量、成本、监控事件等
```

**旧前缀映射（迁移时参考）**：`agents_*` → `catalog_*`；`tools_*` / `skills_*` → `capability_*`；`sessions_*` → `conversation_*`；`usage_*` 及计费类 → `operations_*`；记忆相关表 → `memory_*`。

示例：

```sql
CREATE TABLE capability_skill (
	id TEXT PRIMARY KEY,
	slug TEXT NOT NULL UNIQUE,
	name TEXT NOT NULL,
	status TEXT NOT NULL,
	current_version_id TEXT,
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP NOT NULL
);

CREATE TABLE capability_skill_version (
	id TEXT PRIMARY KEY,
	skill_id TEXT NOT NULL,
	version TEXT NOT NULL,
	manifest_json TEXT NOT NULL,
	created_at TIMESTAMP NOT NULL,
	FOREIGN KEY (skill_id) REFERENCES capability_skill(id)
);

CREATE TABLE capability_tool (
	id TEXT PRIMARY KEY,
	slug TEXT NOT NULL UNIQUE,
	name TEXT NOT NULL,
	enabled BOOLEAN NOT NULL,
	schema_json TEXT NOT NULL,
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP NOT NULL
);
```

约束：

- **Context 只能读写本前缀下的表**；Repository 实现不得拼接其它前缀的表名。
- 公共基础表放在明确的基础设施命名空间，例如 `schema_migrations`。
- 不允许某一 Context 的迁移脚本修改另一 Context 拥有的表。
- 数据库命名应稳定，避免频繁重命名表和字段。
- 若迁移期代码仍使用旧表名（如 `tool` 无前缀），须在专题设计文档中写明与目标 `capability_*` 的差异及迁移计划，避免长期分叉。

若未来 PostgreSQL 需要更强隔离，可切换为 **schema 与 Context 对齐**：

```text
identity.workspace
catalog.agent
capability.tool
conversation.session
memory.l3_chunk
operations.usage_event
```

默认仍以**单库 + 表名前缀**为主，降低 SQLite 与 PostgreSQL 的差异。

---

## 3. 数据访问层（Repository）抽象

每个 Context 在**自己的存储适配器**内定义 Repository，封装对本 Context 表前缀下所有 SQL。其它 Context 禁止直接访问这些表，只能通过 **`kernel/contracts`** 暴露的端口或应用服务调用。

```go
package capability

import (
	"context"
	"database/sql"
)

type SkillRepository interface {
	CreateSkill(ctx context.Context, skill *Skill) error
	GetSkillByID(ctx context.Context, id string) (*Skill, error)
	GetSkillBySlug(ctx context.Context, slug string) (*Skill, error)
	ListSkills(ctx context.Context, query ListSkillsQuery) ([]Skill, error)
	UpdateSkill(ctx context.Context, skill *Skill) error
	DeleteSkill(ctx context.Context, id string) error
	MarkSkillActive(ctx context.Context, id string) error
	RecordSkillActivation(ctx context.Context, id string) error
	WithTransaction(ctx context.Context, fn func(tx SkillRepository) error) error
}

type SQLSkillRepository struct {
	db DBTX
}

type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func NewSQLSkillRepository(db *sql.DB) *SQLSkillRepository {
	return &SQLSkillRepository{db: db}
}
```

Repository 设计要求：

- 对外暴露明确业务方法，例如 `CreateSkill`、`GetSkillByID`、`ListTools`。
- Context 外部不裸写 SQL，不拼接访问其它 Context 前缀表的查询。
- Repository 接口由 Context 自己定义，SQL 实现依赖 `*sql.DB` 或 `*sql.Tx`。
- 测试时可替换为 mock、fixture repository 或内存数据库实现。
- HTTP handler 不直接依赖 SQL：handler → application → repository。

Context 内部推荐分层（与一章 §5 对齐）：

```text
capability/
├── module.go
├── adapters/http/
├── application/
├── domain/
├── ports/
├── adapters/sqlite/
│   ├── repository.go      # Repository 接口（可上提至 ports）
│   ├── repository_sql.go
│   ├── model.go
│   └── migrations/
└── ...
```

---

## 4. Context 间数据关联通过接口而非直接 JOIN

当需要跨 Context 数据时，不编写**跨前缀 JOIN**，而是通过 **`kernel/contracts`** 中的 Query 端口调用对方 Context 暴露的 Reader / Service。

反例：

```sql
SELECT s.id, s.name, u.count
FROM capability_skill s
JOIN operations_usage_event u ON u.skill_id = s.id
WHERE s.id = ?;
```

推荐方式（Capability 组合 Operations 端口）：

```go
type SkillDetailService struct {
	skills SkillRepository
	usage  UsageReader // 由 operations Context 在 contracts.Registry 中注册
}

type UsageReader interface {
	GetSkillUsageSummary(ctx context.Context, skillID string) (*UsageSummary, error)
}

func (s *SkillDetailService) GetSkillDetail(ctx context.Context, skillID string) (*SkillDetail, error) {
	skill, err := s.skills.GetSkillByID(ctx, skillID)
	if err != nil {
		return nil, err
	}

	usage, err := s.usage.GetSkillUsageSummary(ctx, skillID)
	if err != nil {
		return nil, err
	}

	return &SkillDetail{
		Skill: skill,
		Usage: usage,
	}, nil
}
```

这样做的收益：

- Capability 不需要知道 `operations_*` 表结构。
- Operations 可独立优化聚合、缓存或统计表。
- 未来 Operations 拆成微服务时，将 `UsageReader` 实现替换为 RPC client 即可。
- Context 边界由 Go 端口表达，而不是由 SQL JOIN 隐式耦合。

允许例外：

- 只在只读报表、离线分析、迁移脚本中使用跨前缀 JOIN。
- 例外查询放在明确的 `reporting/` 或 `analytics/` 适配器层，不回流到业务 Repository。

---

## 5. 版本化迁移策略

每个 Context 维护自己的迁移脚本，放在 `adapters/sqlite/migrations/`（或等价路径）。启动时按**显式 Context 顺序**执行尚未应用的迁移。

推荐目录：

```text
internal/
├── capability/
│   └── adapters/sqlite/migrations/
│       ├── 000001_capability_skill.up.sql
│       └── ...
├── catalog/
│   └── adapters/sqlite/migrations/
│       └── ...
├── conversation/
│   └── adapters/sqlite/migrations/
│       └── ...
├── memory/
│   └── adapters/sqlite/migrations/
│       └── ...
├── operations/
│   └── adapters/sqlite/migrations/
│       └── ...
└── identity/
    └── adapters/sqlite/migrations/
        └── ...
```

迁移记录统一保存在 `schema_migrations` 表；**`module` 字段存 Context 名**（与二章 `Module.Name()` 一致）：

```sql
CREATE TABLE schema_migrations (
	module TEXT NOT NULL,
	version INTEGER NOT NULL,
	dirty BOOLEAN NOT NULL DEFAULT FALSE,
	applied_at TIMESTAMP NOT NULL,
	PRIMARY KEY (module, version)
);
```

统一迁移入口：

```go
package app

type MigrationSource struct {
	Context string // identity | catalog | capability | conversation | memory | operations
	Path    string
}

func MigrationSources() []MigrationSource {
	return []MigrationSource{
		{Context: "identity", Path: "internal/identity/adapters/sqlite/migrations"},
		{Context: "catalog", Path: "internal/catalog/adapters/sqlite/migrations"},
		{Context: "capability", Path: "internal/capability/adapters/sqlite/migrations"},
		{Context: "conversation", Path: "internal/conversation/adapters/sqlite/migrations"},
		{Context: "memory", Path: "internal/memory/adapters/sqlite/migrations"},
		{Context: "operations", Path: "internal/operations/adapters/sqlite/migrations"},
	}
}

func RunMigrations(db *sql.DB, sources []MigrationSource) error {
	for _, source := range sources {
		if err := migrateContext(db, source); err != nil {
			return fmt.Errorf("migrate %s: %w", source.Context, err)
		}
	}
	return nil
}
```

迁移原则：

- 新增 Context 只需增加自己的 `migrations/`，不修改全局迁移框架。
- 执行顺序与 `InitModules` / 端口依赖顺序协调一致（建议 identity → catalog → capability → memory → conversation → operations）。
- 迁移脚本只修改**本 Context 前缀**下的表；公共表由基础设施迁移负责。
- 生产环境只允许向前迁移，回滚脚本主要用于开发、测试和紧急预案。

---

## 6. 事务管理以 Context 为边界

Context 内多个写操作可使用数据库事务，由 Repository 提供 `WithTransaction`。**跨 Context 事务**不在数据库层实现，避免锁竞争、隐式耦合和长事务。

Repository 事务示例：

```go
func (r *SQLSkillRepository) WithTransaction(
	ctx context.Context,
	fn func(tx SkillRepository) error,
) error {
	db, ok := r.db.(*sql.DB)
	if !ok {
		return fn(r)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	txRepo := &SQLSkillRepository{db: tx}
	if err := fn(txRepo); err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}
```

模块内原子写入示例：

```go
func (s *SkillService) ActivateSkill(ctx context.Context, skillID string) error {
	return s.skills.WithTransaction(ctx, func(tx SkillRepository) error {
		if err := tx.MarkSkillActive(ctx, skillID); err != nil {
			return err
		}

		return tx.RecordSkillActivation(ctx, skillID)
	})
}
```

跨 Context 一致性建议：

- 不推荐在 Capability、Operations、Catalog 等多个 Context 之间开启同一数据库事务。
- 跨 Context 写入使用 Saga、Outbox Event 或业务补偿机制。
- 对于必须强一致的写入，应重新审视限界上下文划分，把同一原子性范围内的数据归入**同一 Context**。
- 例如「记录 Skill 激活与 Usage 事件」若必须原子化，可由 **Capability** 维护内部 `capability_skill_activation_event`，再异步同步到 **Operations**。

---

## 7. 查询优化与缓存预留

对于高频只读查询，Repository 接口设计时应预留缓存层注入点。缓存不写入 SQL 实现内部，而是通过装饰器包裹 Repository 或 Service。

高频查询示例：

- Skill 列表
- Skill 当前版本信息
- Tool schema
- Agent 配置摘要
- Session 最近事件游标

缓存装饰器示例：

```go
type CachedSkillRepository struct {
	next  SkillRepository
	cache Cache
}

func NewCachedSkillRepository(next SkillRepository, cache Cache) *CachedSkillRepository {
	return &CachedSkillRepository{
		next:  next,
		cache: cache,
	}
}

func (r *CachedSkillRepository) GetSkillByID(ctx context.Context, id string) (*Skill, error) {
	key := "capability:v1:skill:" + id

	var cached Skill
	if ok := r.cache.Get(ctx, key, &cached); ok {
		return &cached, nil
	}

	skill, err := r.next.GetSkillByID(ctx, id)
	if err != nil {
		return nil, err
	}

	_ = r.cache.Set(ctx, key, skill)
	return skill, nil
}
```

缓存策略：

- 缓存键基于 Context 名、Context 版本、查询参数和数据版本。
- 失效策略由 Context 自行决定，例如版本变更、写操作后主动删除、TTL 到期。
- 不要求所有 Context 默认开启缓存，先保留接口装饰点。
- 缓存只优化读取路径，不能成为业务正确性的唯一来源。

---

## 8. 结论

Aranea 的数据库设计以单连接池降低部署复杂度，以 **`identity_*` / `catalog_*` / `capability_*` / `conversation_*` / `memory_*` / `operations_*` 表前缀**与限界上下文一一对应，以 Repository 维护 Context 边界。Context 之间通过 **`kernel/contracts` 端口**协作而不是跨前缀 JOIN；迁移脚本按 Context 演进，`schema_migrations.module` 使用 Context 名；事务默认限制在 Context 内部。适合当前单体，也为按 Context 拆库、拆服务保留清晰接缝。


# 四、模块功能设计

## 定位：Capability Context 内的「执行子系统」

本章**不是**全库所有 Context 的通用目录规范——那一层由「一、主体架构设计」§5（`domain` / `application` / `ports` / `adapters`）与「二、路由设计」「三、数据库设计」约束。

本章专门细化 **Capability 限界上下文**（Tool、Skill、MCP、Plugin、Hook 等 **可被 Agent 调用** 的能力）在 **`application` 层之下**的**执行子系统**：如何把「领域能力契约 → 注册表 → 执行链 → 后端实现 → ADK 桥接」组织成可测试、可策略、可观测的一条路径。

**经验来源**：当前仓库中 `internal/tools`（`tooldef`、`toolctx`、`middleware`、`executor`、`registry`、`backends`、`adkbridge`、`schema`、`telemetry`、`storage`）是本章的**参考实现**；其中 `storage` 是迁移期对 `adapters/sqlite` 的等价落点，legacy `internal/repository` 只允许保留委托层；**目标落位**是 `internal/capability/application/*` 与同 Context 下的上述子包（与一章 §12.1 迁移一致）。

**与一、二、三章的关系**：

- **一章**：Capability 是六大 Context 之一；跨 Context 只能通过 `kernel/contracts`（如 `ToolResolver`、`ToolExecutor`）。
- **二章**：Capability 的 `module.go` 实现四阶段 `Module`；HTTP 挂在 `adapters/http`，OpenAPI 在 `RegisterDriving` 中登记。
- **三章**：Capability 的表前缀为 **`capability_*`**；Repository 只在 `adapters/sqlite`（或等价存储适配器）中访问这些表。

**目录结构的递进关系**（仅针对 **Capability**；其它 Context 无「可运行能力执行子系统」则可不建下列子包）：

1. **Context 壳 + CRUD**：`module.go`、`adapters/http`、`application`（编排）、`adapters/sqlite` + `migrations`（见一章 §5 + 三章）。
2. **执行子系统（本章）**：在 1 的 `application` 之下增加 `def`、`ctx`、`middleware`、`executor`、`registry`、`backends`、对 ADK 的 `adkbridge`（或 `adapters/adkruntime` 内的桥接包）、`schema`、`telemetry`。

**核心词汇**（后文统一含义）：

- **可运行能力**：Capability 内可被运行时单次调用的最小单元（一次 Tool 调用、一次 Skill 渲染、一次 MCP 调用等）。
- **管理面**：HTTP CRUD、配置、审计查询等经 `adapters/http → application → adapters/sqlite`。
- **运行面**：Conversation / `adkruntime` 经 **contracts 端口** 进入 Capability 的 **executor**（或经 ADK callback 回到同一入口）。
- **观测面**：事件、trace、metrics 及对外 SSE；由 middleware / `adk.plugin` 注入。
- **策略**：权限、可见性、预算等；优先 **middleware** 或 **application 内过滤**，见 §3。

### 三层协作总览（Capability）

```text
管理面：adapters/http -> application -> adapters/sqlite（capability_*）
运行面：adkruntime / contracts -> application（组装 registry 视图）
          -> executor -> middleware 链 -> backends
观测面：middleware（事件）/ telemetry -> operations、日志、前端、SSE
```

**说明**：`registry` 持有「本 Context 已解析的能力实例」；是否在注册前过滤（如仅 `enabled=1`）由 **application** 完成；执行路径上的再次拦截由 **middleware** 完成。

### 推荐目录结构（Capability + 执行子系统）

```text
internal/capability/
├── module.go                 # 实现 module.Module（二章 §2）
├── domain/
├── ports/                    # 本 Context 对外端口（含注册到 kernel 的接口）
├── application/
│   ├── skill_service.go      # 例：CRUD、激活
│   └── tool_engine.go        # 例：registry + executor 组装（执行子系统门面）
├── adapters/
│   ├── http/
│   └── sqlite/
│       ├── repository*.go
│       └── migrations/       # 三章 §5；表前缀 capability_*
├── tooldef/                  # 或 capabilitydef/；能力契约（Tool、StreamingTool…）
├── toolctx/
├── middleware/
├── executor/
├── registry/
├── backends/
├── adkbridge/                # 唯一将 tooldef 转为 adk.Tool 的薄层（业务仍不 import adk 于 domain）
├── schema/
└── telemetry/
```

旧路径 `internal/tools/**` 在迁移完成后应消失或变为对 `internal/capability/**` 的兼容 re-export（过渡期）。

## 1. 模块能力契约优先

不要在 `adapters/http` 或纯 CRUD `application` 里直接堆执行逻辑；先定义 **Capability 领域契约**。`tooldef.Tool`（目标包名可为 `capability/tooldef`）是示例：稳定标识、展示信息、输入输出 schema、校验与 `Execute` 合一。

通用要求：

- `<module>def` 只定义领域契约、事件结构、扩展接口，不依赖 HTTP、SQL 或具体第三方运行时。
- 能力接口要表达模块的最小可执行单元，例如 `Tool`、`Skill`、`MemoryProvider`、`AgentRuntime`。
- 可选能力通过小接口扩展，例如 `StreamingTool`、`ApprovableTool`，避免在主接口里堆积大量可空字段。
- 对外暴露稳定 key 或 slug，展示名、分类、描述、schema 等元信息由能力自己提供。
- 参数校验属于能力契约的一部分，不能只依赖前端或 API 层校验。

## 2. 运行上下文独立建模

每个会被运行时调用的模块，都应有自己的 `<module>ctx`。上下文用于承载一次调用的会话、用户、agent、trace、审批、状态存储等元数据，避免到处传递散乱参数。

设计要求：

- 上下文必须嵌入或包装 `context.Context`，确保超时、取消和 tracing 能沿调用链传递。
- 上下文字段只放调用级元数据，不放全局配置和大型服务对象。
- 提供 `New` 与 `Clone` 方法，middleware 可以安全替换底层 context 或补充 trace context。
- 对 agent/session/user/message 等常用身份信息保持显式字段，便于日志、事件、审计和策略判断。
- 对状态访问使用小接口，例如 `StateStore`，不要让业务实现直接依赖全局 store。
- **取消与超时**：adapter 与 backend 中需要发起 IO 时，必须使用 `<module>ctx` 内嵌的 `Context`（如 `http.NewRequestWithContext`），禁止在无理由的情况下用 `context.Background()` 覆盖上游已传入的 deadline/cancel。
- **禁止丢弃取消**：从外部 runtime 进入的 callback 构造新 ctx 时，应继承或包装父级 context，而不是静默丢弃取消信号。

## 3. Middleware 承载横切能力

模块中的校验、预算、追踪、事件、权限、限流、审计、重试、脱敏等横切逻辑应进入 `middleware/`，由 executor 统一串联。业务后端只关注自身能力，不直接散落这些通用逻辑。

**关于「策略 / policy」**：不在模块根下默认增加与 `middleware` 平级的 `policy/` 包。权限、可见性、预算等优先实现为 **middleware**；或在 **service** 内对「待注册/待执行」的能力列表做过滤，再交给 executor。仅当策略逻辑体量大、且需与 HTTP/executor 完全解耦时，才在包内增加小写 `policy.go` 或 `internal/policy` 子包，并由 middleware/service 调用——避免团队误以为存在「第四层运行时」。

推荐 middleware 串联顺序（可按模块调整）：

```text
（service 侧已对 registry 视图做过滤的前提下）
Validation -> Budget/RateLimit -> 权限或审批类 middleware -> Tracing -> Audit/Event -> Backend Execute
```

设计要求：

- middleware 接收模块上下文、能力实例、参数和 `next`，返回统一结果与 error。
- 每个 middleware 只处理一个横切关注点，保持可组合、可测试。
- 事件类 middleware 负责发出 `before`、`after`、`blocked`、`success`、`error` 等生命周期事件。
- 安全与预算类 middleware 可以返回结构化 blocked 结果，但必须记录阻断原因。
- tracing middleware 应使用模块自己的 `telemetry.Provider`，设置模块名、能力名、分类、session、agent 等属性。

**Middleware 函数签名**（Capability 内放在 `internal/capability/middleware/chain.go`）：

```go
package middleware

import (
	"context"

	"arenea/backend/internal/capability/tooldef"
	"arenea/backend/internal/capability/toolctx"
)

type Result = tooldef.ExecutionResult

type Next func(ctx context.Context, tctx *toolctx.Context, tool tooldef.Tool, params map[string]any) (Result, error)

type Middleware func(next Next) Next

func Compose(mws ...Middleware) Next {
	final := terminal()
	for i := len(mws) - 1; i >= 0; i-- {
		final = mws[i](final)
	}
	return final
}
```

**Policy middleware 示例**（旧 `internal/runtime/tool_policy.go` 的目标位置，落到 `internal/capability/middleware/policy.go`）：

```go
package middleware

import (
	"context"

	"arenea/backend/internal/capability/tooldef"
	"arenea/backend/internal/capability/toolctx"
	"arenea/backend/internal/kernel/errs"
	"arenea/backend/internal/kernel/runctx"
)

type PolicyChecker interface {
	AllowToolForAgent(ctx context.Context, tenantID, agentID, toolKey string) (allowed bool, reason string, err error)
}

func Policy(p PolicyChecker) Middleware {
	return func(next Next) Next {
		return func(ctx context.Context, tctx *toolctx.Context, tool tooldef.Tool, params map[string]any) (Result, error) {
			rc := runctx.From(ctx)
			ok, reason, err := p.AllowToolForAgent(ctx, rc.TenantID, rc.AgentID, tool.Name())
			if err != nil {
				return Result{}, errs.Wrap(err, errs.CapabilityToolPolicyFailure, "policy check failed")
			}
			if !ok {
				return Result{
					Status: tooldef.StatusBlocked,
					Reason: reason,
				}, errs.New(errs.CapabilityToolForbidden, reason)
			}
			return next(ctx, tctx, tool, params)
		}
	}
}
```

中间件**输入**只来自 `ctx`（含 `runctx.RuntimeContext`）、`toolctx.Context` 与显式参数；不允许从全局变量、环境变量、`init()` 注入业务状态。

## 4. Executor 是唯一运行入口

模块能力的执行必须经由 `executor`，而不是由 handler、service、adapter 或第三方 callback 直接调用 backend。这样才能保证校验、审计、预算和 tracing 总是生效。

执行链路推荐为：

```text
API / Runtime / Adapter
        |
        v
Service / Runtime Assembler（从 DB 读配置、合并内置能力、按策略过滤）
        |
        v
Registry 解析并得到 capability 实例
        |
        v
Executor.Run(ctx, capability, params)
        |
        v
Middleware Chain
        |
        v
Backend Execute
```

设计要求：

- executor 负责组装 middleware chain，并提供空 chain 的安全默认行为。
- 参数为空时统一规范化为空 map 或 typed input，避免后端重复处理 nil。
- service 与 adapter 只能调用 executor，不直接调用后端实现。
- executor 的单元测试应覆盖正常执行、校验失败、middleware 顺序、错误传播和事件输出。

## 5. Registry 统一能力发现

`registry` 是模块内部能力的汇合点，用于注册、查询、列表和排序。内置能力、数据库启用能力、插件能力、运行时动态能力都应先汇入 registry，再由 service 或 runtime 读取。

设计要求：

- registry 内部处理并发安全，外部不直接操作 map。
- 注册时以稳定 key/slug 为主键，空 key、nil 实例直接忽略或返回错误。
- 列表结果保持稳定排序，方便测试、前端展示和配置 diff。
- `Builtins()` 只负责内置能力集合，不处理数据库策略、用户权限或运行时过滤。
- registry 不直接访问 SQL；是否启用、是否对某 agent 可见等由 **service** 在组装 registry 视图时处理，**必要时**再由 middleware 在执行路径上二次拦截。

## 6. Backend 只实现具体能力

`backends/` 存放模块的具体能力实现。每个 backend 应当实现 `<module>def` 中的核心接口。可参考 `tools/backends` 用嵌入 **`Base`** 结构统一元数据与校验（非强制，但推荐保持一致风格）：

```go
// 示意：与 tools/backends/base.go 同思路
type Base struct {
	Key, Label, Desc, ToolCategory string
	InSchema, OutSchema            map[string]any
	Required                       []string
}
// Name/DisplayName/Description/Category/InputSchema/OutputSchema/Validate 由 Base 实现
// 具体工具类型嵌入 Base 并实现 Execute
```

设计要求：

- backend 不关心 HTTP 请求、前端 payload、数据库行结构和第三方运行时对象。
- backend 可以依赖标准库或必要基础设施，但必须遵守模块上下文的取消和超时。
- 文件、网络、命令、外部 API 等高风险能力必须在 backend 内做边界校验。
- 描述文案要包含使用边界，尤其是 agent 可调用能力必须明确“何时使用”和“何时不要使用”。
- 输出统一为结构化 map 或 typed output，避免返回纯文本导致上层难以审计和渲染。

## 7. Schema 与类型定义分离

模块输入输出结构放入 `schema/`，并通过统一工具生成 JSON Schema。这样同一份 typed struct 可以同时服务后端校验、ADK/function calling、OpenAPI、前端表单和测试 fixture。

设计要求：

- 每个能力都声明 input schema 与 output schema。
- schema 生成失败时回落到空 object，但应在测试中覆盖正常 schema。
- 对外 API、runtime adapter 和前端配置不要手写不一致的 schema。
- 如果模块支持用户自定义能力，数据库中保存的 schema 应与 runtime 使用的 schema 格式一致。

## 8. Adapter 隔离外部运行时

第三方运行时、SDK、协议和插件系统都应通过 `adapter/` 或更具体的 `adkbridge/` 接入。adapter 的职责是把内部契约转换成外部对象，把外部 callback 转回内部 executor。

设计要求：

- 内部核心接口不能直接依赖第三方运行时类型。
- adapter 中可以处理 schema 类型转换、callback 包装、审批标记、长任务标记等协议细节。
- 外部 callback 进入系统后必须重新构造模块上下文，并调用 executor。
- adapter 不实现业务逻辑，不绕过 registry 与 middleware（含策略类 middleware）。
- 当替换第三方运行时时，只应主要修改 adapter，而不是重写后端能力。

## 9. Telemetry 与事件是模块基础设施

可运行能力必须具备 tracing、metrics 或事件能力。`capability/telemetry`（或 `internal/capability/telemetry`）提供本 Context 的 instrumentation name，middleware 在执行时创建 span 并写入关键属性。

设计要求：

- instrumentation name 与 Go 包路径一致，例如 `arenea/backend/internal/capability`（迁移完成后；过渡期可与现有 `internal/tools` 并存）。
- span 名称应包含模块能力名，例如 `tool.read_file`、`skill.render`。
- span 属性至少包含能力 key、分类、session、agent、user、runtime status 等关键维度。
- 事件 payload 必须结构化，包含 phase、status、capability key、参数摘要、结果摘要、错误、耗时。
- **敏感字段默认处理**（事件与日志摘要建议一致）：
  - 字符串值长度超过 **1024** 字节（或 rune 合理阈值）时截断并加省略标记；
  - 参数键名匹配（大小写不敏感）`password`、`token`、`secret`、`api_key`、`authorization`、`cookie` 时，值替换为 `***` 或仅保留长度；
  - 大文件、全文内容不进事件体，只记录 path、hash、size 等元数据。

## 10. API、Storage 与 Runtime 分层协作

Capability Context 完整落地时通常包含三条路径：管理 API、持久化 SQLite、被 Conversation/adkruntime 驱动的执行路径。三者共享 **Capability 领域契约**，但职责不能混在一起。总览图见本章「三层协作总览（Capability）」。

设计要求：

- `adapters/http` 只做协议转换，不拼 SQL，不直接执行 backend。
- `application` 负责编排、权限与可见性策略（过滤 registry 视图）、调用 Repository 与 executor。
- `adapters/sqlite` 只暴露 Repository 与 SQL 实现，不泄露表细节给 HTTP。
- Conversation Runtime 只通过 **contracts** 消费 Capability 解析后的能力集合，不直接读 `capability_*` 表。
- 前端管理 API 与 runtime 使用的能力应共享 key、schema、status、visibility 等字段定义。

## 11. 错误模型与 Context 约定

- **错误包装**：模块内部返回错误时优先使用 `fmt.Errorf("...: %w", err)` 保留根因，便于日志与中间件判断。
- **对上层语义**：HTTP handler 将领域错误映射为稳定 HTTP 状态码与业务错误码（若项目已有统一 `ErrorCode` 类型则沿用）；避免将内部堆栈或 SQL 原文返回给客户端。
- **可判定错误**：需要分支处理的错误（如「未找到」「无权限」「预算耗尽」）优先使用 `errors.Is` / 自定义哨兵错误类型，而不是字符串包含判断。
- **与 Context**：凡可能阻塞的调用必须传入请求或会话级 context；测试可使用带超时的 context 验证取消行为。

## 12. 测试要求

采用该模式建设的模块至少包含以下测试：

- `<module>def` 和 backend 的参数校验测试。
- executor 的 middleware 顺序、错误传播、空参数处理测试。
- registry 的注册、查询、排序和并发安全测试。
- adapter 的 schema 转换、callback 执行和上下文传递测试。
- storage 的 CRUD、软删除、唯一约束、迁移兼容测试。
- **策略与可见性**：service 侧过滤结果与 middleware 侧拦截（允许、拒绝、`blocked` 事件）的测试。
- 高风险 backend 的沙箱边界、网络超时、输出截断测试。

## 13. 建设准则

**仅适用于 Capability Context**（其它 Context 见一章 §5，不必强行引入执行子系统）：

- 若 Capability 只提供能力的 **CRUD**（无运行时执行），保持 `adapters/http` + `application` + `adapters/sqlite` 即可，不强行引入 `executor`。
- 一旦能力会被 **agent / ADK / MCP** 调用，必须引入 `def + ctx + middleware + executor + registry + backends + adkbridge`（可按需裁剪子目录，但执行链路不得绕开 `executor`）。
- 暴露给 LLM / function calling 的能力必须提供 schema、策略（application 过滤与/或 middleware）、事件与 telemetry。
- 用户可配置、启停或授权的能力须在 **`capability_*` 表**中保存 status、visibility、runtime_status、config_schema、config、metadata。
- **禁止**在 `domain` 或 `application` 中直接 `import google.golang.org/adk`；ADK 类型仅出现在 `adkbridge` 或 `internal/.../adapters/adkruntime`。

## 14. 结论

「四、模块功能设计」是 **Capability Context** 的专题深化：在「一」的限界上下文与端口、「二」的四阶段 `Module` 与 HTTP、「三」的 `capability_*` 存储之上，规定**可运行能力**如何经 **registry + executor + middleware** 形成固定路径，并由 **adkbridge** 接入 go-adk。其它 Context（Identity、Catalog、Conversation、Memory、Operations）无此执行子系统时不必套用全部分层；`docs/23 tools struct design.md`、`docs/20 skill struct design.md` 等应标明归属 **Capability**，并与本章对齐。

---

# 附录 A：代码骨架模板

> 本附录提供 6 个最常用的样板。AI 在「P0 建壳」阶段与新增 Context / 文件时**直接复制粘贴**，禁止重新创作。占位符以 `<…>` 标注，例如 `<context>` 替换为 `capability`、`<Context>` 替换为 `Capability`。

## A.1 Context 壳（`internal/<context>/module.go`）

```go
package <context>

import (
	"context"
	_ "embed"
	"encoding/json"

	"github.com/go-chi/chi/v5"

	"arenea/backend/internal/kernel/contracts"
	"arenea/backend/internal/kernel/module"
)

//go:embed openapi.json
var openAPISpec []byte

type Deps struct {
	DB     DBTX        // 由 app/container 提供，三章 §3
	Logger Logger
	Clock  Clock
	Bus    EventBus
}

type Module struct {
	deps Deps
	reg  *contracts.Registry
}

func NewModule(deps Deps) *Module { return &Module{deps: deps} }

func (m *Module) Name() string    { return "<context>" }
func (m *Module) Version() string { return "v1" }

func (m *Module) RegisterPorts(reg *contracts.Registry) {
	m.reg = reg
	// reg.Register<X>Reader(m.<x>Service)
}

func (m *Module) ResolvePorts(reg *contracts.Registry) {
	// m.audit = reg.AuditWriter()
}

func (m *Module) RegisterDriving(d module.DrivingRegistry) {
	d.RegisterOpenAPISpec("<context>", openAPISpec)
	d.WithAPIV1(func(r chi.Router) {
		// r.Route("/<aggregates>", m.http.MountRoutes)
	})
}

func (m *Module) Start(ctx context.Context) error    { return nil }
func (m *Module) Shutdown(ctx context.Context) error { return nil }

func (m *Module) OpenAPISpec() (json.RawMessage, error) {
	return json.RawMessage(openAPISpec), nil
}
```

## A.2 Ports 输入 / 输出（`internal/<context>/ports/{input.go,output.go}`）

```go
// ports/input.go —— 暴露给本 Context 的 adapter 调用
package ports

import "context"

type <Aggregate>Service interface {
	Get<Aggregate>(ctx context.Context, id string) (*<Aggregate>, error)
	List<Aggregate>s(ctx context.Context, q List<Aggregate>Query) ([]<Aggregate>, error)
}
```

```go
// ports/output.go —— 本 Context 依赖的外部接口
package ports

import "context"

type <Aggregate>Repository interface {
	Get(ctx context.Context, id string) (*<Aggregate>, error)
	Save(ctx context.Context, x *<Aggregate>) error
	WithTransaction(ctx context.Context, fn func(tx <Aggregate>Repository) error) error
}
```

跨 Context 端口（被其它 Context 消费）**不**放这里，放 `internal/kernel/contracts/<context>.go`。

## A.3 占位 OpenAPI（`internal/<context>/openapi.json`）

```json
{
  "openapi": "3.1.0",
  "paths": {},
  "components": {
    "schemas": {}
  },
  "tags": [
    { "name": "<Context>", "description": "<Context> Context routes" }
  ]
}
```

启动期 `MergeOpenAPISpecs`（二章 §6.1）会自动注入 `info` / `servers`，禁止在此文件写。

## A.4 Deprecated alias（旧路径保留兼容）

```go
// Deprecated: moved to arenea/backend/internal/<context>/<sub>/<file>.go
// 该兼容文件将在 P8 Alias 清理阶段删除（见 aranea/docs/migration-status.md）。
package <oldpkg>

import (
	new<sub> "arenea/backend/internal/<context>/<sub>"
)

type (
	<TypeA> = new<sub>.<TypeA>
	<TypeB> = new<sub>.<TypeB>
)

var (
	<FuncA> = new<sub>.<FuncA>
	<FuncB> = new<sub>.<FuncB>
)
```

只允许 `type alias` 与 `var` 转发；**禁止**在兼容文件中放新逻辑、改签名、加包装层。

## A.5 占位迁移（`internal/<context>/adapters/sqlite/migrations/000001_<context>_init.up.sql`）

```sql
-- <Context> Context init schema
-- 表前缀强制 <context>_*；多租户 Day1 强制 tenant_id（一章 §12.3）

CREATE TABLE IF NOT EXISTS <context>_<aggregate> (
    id          TEXT     PRIMARY KEY,
    tenant_id   TEXT     NOT NULL,
    -- TODO: 业务字段
    created_at  TIMESTAMP NOT NULL,
    updated_at  TIMESTAMP NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_<context>_<aggregate>_tenant
    ON <context>_<aggregate>(tenant_id, id);
```

## A.6 Context HTTP 路由壳（`internal/<context>/adapters/http/router.go`）

```go
package http

import "github.com/go-chi/chi/v5"

type Handler struct {
	deps Deps
}

func NewHandler(deps Deps) *Handler { return &Handler{deps: deps} }

func (h *Handler) MountRoutes(r chi.Router) {
	// r.Get("/{id}", h.get)
	// r.Post("/",    h.create)
}
```

业务中间件挂在本 router 内；全局中间件（CORS / Recover / RequestID / 认证入口）在 `internal/app/middleware/` 与 `internal/app/router.go`，**不在此处重复**。

---

# 附录 B：CI 红线静态检查配置

> 把一章 §8 编译期红线、§12.3 多租户约束、§12.1.2 迁移红线落地为可执行的 lint。文件路径：`aranea/backend/.golangci.yml`。

```yaml
version: "2"

run:
  timeout: 5m
  modules-download-mode: readonly

linters:
  enable:
    - depguard          # 依赖方向红线（一章 §8）
    - forbidigo         # 字面量红线（context.Background 滥用 / fmt.Print 等）
    - revive
    - govet
    - staticcheck
    - errcheck
    - gosec
    - unused
    - misspell
    - gocritic
    - importas

linters-settings:
  depguard:
    rules:
      kernel-no-context:
        list-mode: lax
        files:
          - "**/internal/kernel/**/*.go"
        deny:
          - pkg: "arenea/backend/internal/identity"
            desc: "Kernel must not import any business context (一章 §5)"
          - pkg: "arenea/backend/internal/catalog"
            desc: "Kernel must not import any business context"
          - pkg: "arenea/backend/internal/capability"
            desc: "Kernel must not import any business context"
          - pkg: "arenea/backend/internal/conversation"
            desc: "Kernel must not import any business context"
          - pkg: "arenea/backend/internal/memory"
            desc: "Kernel must not import any business context"
          - pkg: "arenea/backend/internal/operations"
            desc: "Kernel must not import any business context"

      domain-pure:
        list-mode: lax
        files:
          - "**/internal/*/domain/**/*.go"
        deny:
          - pkg: "github.com/go-chi/chi"
            desc: "domain must not depend on HTTP (一章 §8)"
          - pkg: "database/sql"
            desc: "domain must not depend on SQL"
          - pkg: "google.golang.org/adk"
            desc: "domain must not depend on ADK"
          - pkg: "modernc.org/sqlite"
            desc: "domain must not depend on SQLite driver"

      application-no-cross-context:
        list-mode: lax
        files:
          - "**/internal/identity/application/**"
          - "**/internal/catalog/application/**"
          - "**/internal/capability/application/**"
          - "**/internal/conversation/application/**"
          - "**/internal/memory/application/**"
          - "**/internal/operations/application/**"
        deny:
          # 跨 Context 必须经 kernel/contracts —— 这里通过 forbidigo 二次校验
          - pkg: "google.golang.org/adk"
            desc: "application must not import ADK (一章 §6)"

      adk-isolation:
        list-mode: lax
        files:
          - "**/*.go"
          - "!**/internal/conversation/adapters/adkruntime/**"
        deny:
          - pkg: "google.golang.org/adk"
            desc: "google.golang.org/adk is only allowed in conversation/adapters/adkruntime/** (一章 §6)"

      sql-isolation:
        list-mode: lax
        files:
          - "**/*.go"
          - "!**/internal/*/adapters/sqlite/**"
          - "!**/internal/kernel/pkg/db/**"
        deny:
          - pkg: "database/sql"
            desc: "database/sql is only allowed in <context>/adapters/sqlite/** or kernel/pkg/db/** (一章 §8)"
          - pkg: "modernc.org/sqlite"
            desc: "modernc.org/sqlite is only allowed in <context>/adapters/sqlite/**"

      chi-isolation:
        list-mode: lax
        files:
          - "**/*.go"
          - "!**/internal/*/adapters/http/**"
          - "!**/internal/app/**"
          - "!**/cmd/**"
        deny:
          - pkg: "github.com/go-chi/chi"
            desc: "chi router is only allowed in <context>/adapters/http/** or app/** (一章 §8)"

  forbidigo:
    forbid:
      - p: '^context\.Background\(\)$'
        msg: "禁止用 context.Background() 覆盖上游 ctx；继承父 ctx 或在 driving adapter 入口构造（一章 §5.3）"
      - p: '^fmt\.Print(f|ln)?\(' 
        msg: "use structured logger; raw fmt.Print* is forbidden in production code"

issues:
  exclude-rules:
    # 测试代码可以使用 context.Background
    - path: '_test\.go'
      linters: [forbidigo]
    # 主入口可以使用 context.Background
    - path: 'cmd/aranea/main\.go'
      linters: [forbidigo]
```

**配套**（`aranea/backend/Makefile` 或 CI workflow）：

```makefile
.PHONY: lint
lint:
	golangci-lint run --timeout=5m

.PHONY: verify
verify: lint
	go vet ./...
	go build ./...
	go test ./... -count=1
```

GitHub Actions 最小片段（`.github/workflows/ci.yml`）：

```yaml
- uses: golangci/golangci-lint-action@v6
  with:
    working-directory: aranea/backend
    version: v1.61
    args: --timeout=5m
- run: |
    cd aranea/backend
    go vet ./...
    go build ./...
    go test ./... -count=1
```

---

# 附录 C：第一行 PR 端到端走读（row #1）

> 用 `aranea/docs/migration-status.md` 第 1 行（`runtime_context.go` → `kernel/runctx/`）作为最简单样板，AI 第一次执行 §12.1.3 Runbook 时**严格照抄**这一遍。

## C.1 任务定义

| 项 | 值 |
| --- | --- |
| 行号 | #1 |
| 源 | `aranea/backend/internal/runtime/runtime_context.go` |
| 目标 | `aranea/backend/internal/kernel/runctx/runtime_context.go` |
| 跨 Context 端口 | 无（runctx 是 Kernel 内部） |
| Deprecated alias | 是（旧路径有外部 import） |
| 阶段 | P1 Kernel 上提 |

## C.2 Pre-flight

```bash
git checkout main && git pull --ff-only
cat "aranea/docs/migration-status.md"             # 确认 #1 状态为 todo
cd aranea/backend
go build ./...                                     # 应绿
go test ./... -count=1                             # 应绿
go vet ./...                                       # 应绿
git checkout -b migrate/row-1-runctx-to-kernel
```

## C.3 Execute

```bash
# 1) 建壳：本 PR 第一个 commit 是 Kernel 目录占位
mkdir -p internal/kernel/{ids,clock,errs,event,runctx,contracts,module,telemetry,pkg}
# 各目录添加最小占位（避免空目录）：
printf 'package runctx\n' > internal/kernel/runctx/doc.go
printf 'package ids\n'    > internal/kernel/ids/doc.go
# ... 其它目录同上
git add internal/kernel
git commit -m "scaffold: kernel package skeleton (per docs/design/platform-architecture.md §5)"

# 2) git mv 主文件 + 测试（如有）
git mv internal/runtime/runtime_context.go internal/kernel/runctx/runtime_context.go
# 测试若存在：
[ -f internal/runtime/runtime_context_test.go ] && \
  git mv internal/runtime/runtime_context_test.go internal/kernel/runctx/runtime_context_test.go

# 3) 改包名：手动把新文件顶部 package runtime → package runctx
#    然后让 cursor 全仓搜索替换：
#       "arenea/backend/internal/runtime"   →  "arenea/backend/internal/kernel/runctx"
#       runtime.RuntimeContext              →  runctx.RuntimeContext
#       runtime.From / runtime.With         →  runctx.From / runctx.With
goimports -w internal/

# 4) 跨 Context 端口：runctx 暂无对外接口需提到 contracts，跳过。

# 5) Deprecated alias：旧路径保留 type alias（模板 A.4）
cat > internal/runtime/runtime_context.go <<'EOF'
// Deprecated: moved to arenea/backend/internal/kernel/runctx.
// 该兼容文件将在 P8 Alias 清理阶段删除（见 aranea/docs/migration-status.md）。
package runtime

import "arenea/backend/internal/kernel/runctx"

type RuntimeContext = runctx.RuntimeContext

var (
    From = runctx.From
    With = runctx.With
)
EOF
```

## C.4 Verify

```bash
go build ./...                          # 必须绿
go test ./... -count=1                  # 必须绿
go vet ./...                            # 必须绿
golangci-lint run --timeout=3m          # 必须绿（附录 B 红线全部通过）
```

任一项红 → `git reset --hard <pre-execute-sha>` 回滚到 Pre-flight 状态，重做。

## C.5 Commit & Track

```bash
git add -A
git commit -m "migrate(row-1): runtime/runtime_context.go → kernel/runctx/

Per docs/design/platform-architecture.md §12.1.1 row #1.
Source:      internal/runtime/runtime_context.go
Destination: internal/kernel/runctx/runtime_context.go
Cross-context port: none
Deprecated alias: yes (internal/runtime/runtime_context.go retained as type-alias forwarder)"

# 更新进度
# 编辑 aranea/docs/migration-status.md：
#   - #1 行 ☐ → ☑，写 PR 链接 + 日期
#   - 操作日志表 append 一行
git add "aranea/docs/migration-status.md"
git commit -m "track(row-1): mark done"

git push -u origin HEAD
gh pr create --fill
```

## C.6 PR 描述模板

```markdown
## Summary
- 按 `docs/design/platform-architecture.md` §12.1.1 row #1 迁移 `runtime_context.go` 到 `kernel/runctx/`。
- 旧路径保留 type-alias 兼容文件（P8 阶段统一清理）。
- 本 PR 不含业务逻辑改动；不涉及 ADK 或跨 Context 端口。

## Verification
- [x] `go build ./...`
- [x] `go test ./... -count=1`
- [x] `go vet ./...`
- [x] `golangci-lint run`

## Migration Status
- [x] `aranea/docs/migration-status.md` 已更新（row #1 标记 done）。
```

---
