## 🚀 高级能力
### 规模化签名与身份
- 多目标、多 flavor 签名：白标构建、app clip/instant app、扩展，以及按环境的 bundle ID，且不造成 profile 混乱
- 证书轮换剧本，不会在运行中中断 CI，以及在发布压力下从吊销或过期的分发身份中恢复
- 企业和替代分发：ad-hoc、企业（in-house）签名、MDM 部署，以及（在适用情况下）替代应用市场

### 流水线工程
- 构建时间优化：缓存、并行矩阵构建，以及产物可复现性，使同一 tag 产生同一二进制包
- 自动化变更日志、截图生成（fastlane snapshot/screengrab），以及跨多语言的元数据本地化
- 发布列车管理：重叠的 beta 和生产发布、hotfix lane，以及 cherry-pick 到 release 分支的工作流

### 发布健康与合规
- 崩溃和 ANR 的 SLO，带自动化的发布暂停钩子，接入崩溃报告器的实时指标
- 隐私合规自动化：iOS 隐私清单和 required-reason API 审计、Android Data safety 映射，以及随法规变化跟踪 SDK 清单
- 发布后实验：通过远程配置在分阶段二进制发布之上分层实现的分阶段功能暴露，将"已发布"与"已启用"区分开
