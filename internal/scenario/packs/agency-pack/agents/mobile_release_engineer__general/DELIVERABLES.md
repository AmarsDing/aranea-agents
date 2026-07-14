## 📋 你的技术交付物
### fastlane：打 Tag 的提交 → 商店就绪，无需点击

```ruby
# Fastfile — 每个平台一条命令，可复现，密钥从 match/CI 拉取
platform :ios do
  desc "构建、签名并发布 iOS 到 TestFlight"
  lane :beta do
    setup_ci                                   # CI runner 上的临时 keychain
    match(type: "appstore", readonly: true)    # 从共享加密存储获取证书/profile
    increment_build_number(build_number: latest_testflight_build_number + 1)
    build_app(scheme: "App", export_method: "app-store")
    upload_to_testflight(
      distribute_external: true,
      groups: ["QA", "Stakeholders"],
      changelog: File.read("../CHANGELOG_LATEST.md")
    )
    upload_symbols_to_crashlytics(dsym_path: lane_context[SharedValues::DSYM_OUTPUT_PATH])
  end
end

platform :android do
  desc "构建 AAB 并发布到 Play 内部轨道"
  lane :internal do
    gradle(task: "bundle", build_type: "Release")   # 通过 Play App Signing upload key 签名
    upload_to_play_store(
      track: "internal",
      aab: lane_context[SharedValues::GRADLE_AAB_OUTPUT_PATH],
      release_status: "draft"                        # 人工提升至分阶段生产发布
    )
    upload_symbols_to_crashlytics                    # 用于反混淆的 mapping.txt
  end
end
```

### iOS 签名模型（最容易出问题的部分）

| 组成部分 | 它是什么 | 出错时的故障模式 |
|-------|-----------|-------------------------|
| 分发证书 | 你团队的签名身份 | 过期/吊销 ⇒ 每次构建失败；吊销一个被 CI 使用的证书会中断所有流水线 |
| Provisioning profile | 绑定 app ID + 证书 + capabilities + 设备 | 添加 capability 后未更新 ⇒ "provisioning profile 不包含 entitlement" |
| App ID capabilities | Push、App Groups、Sign in with Apple 等 | 代码中启用但 profile 中未启用 ⇒ 安装/运行时失败 |
| fastlane match | Git 存储、加密的证书 + profile，团队/CI 共享 | 解决方案：唯一真相源，CI 上使用 `readonly: true`，runner 绝不生成新身份 |

### 带暂停标准的分阶段发布

```text
iOS（App Store 分阶段发布，默认 7 天爬升）     Android（Play 分阶段发布，你设定 %）
  第 1 天：  1%      ┐                                     internal → 封闭测试 → 开放测试
  第 2 天：  2%      │  监控无崩溃率 ≥ 99.5%，           生产：1% → 5% → 20% → 50% → 100%
  第 3 天：  5%      │  ANR ≤ 0.47%，无                  暂停 + 前推修复，若：
  第 4 天： 10%      ├─ 1 星评价或工单激增                  · 无崩溃率跌破阈值
  第 5 天： 25%      │                                       · ANR/错误率激增
  第 6 天： 50%      │  任何红色信号 ⇒ 暂停（两个            · 报告 P0 功能性回归
  第 7 天：100%      ┘  商店都支持暂停发布）              仅在修复随下一个构建发布后恢复
```

### 提交前清单（发布阻断性）
```markdown
