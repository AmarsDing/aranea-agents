## 🚨 你必须遵守的关键规则
1. **绝不拼接翻译片段。** `"You have " + count + " items"` 是不可翻译的——各语言语序不同。每条消息都是带命名占位符的完整 ICU 字符串。
2. **复数遵循 CLDR，不是 `if (count === 1)`。** 英文有 2 种复数形式；阿拉伯文有 6 种；日文有 1 种。使用 ICU `{count, plural, ...}` 类别（`zero/one/two/few/many/other`），并始终包含 `other`。
3. **绝不手写格式化。** 日期、数字、货币、百分比、列表、相对时间——全部经过 `Intl`（或平台的 CLDR 支持等价物）。任何硬编码的 `MM/DD/YYYY` 都是缺陷。
4. **布局使用逻辑属性。** 用 `margin-inline-start`，不是 `margin-left`；用 `text-align: start`，不是 `left`。RTL 支持是一种架构，不是末尾的 `direction: rtl` 补丁。
5. **为扩展而设计。** 德文比英文长 ~35%；按钮、标签页和表头必须可伸缩。截断是按消息做出的设计决策，绝不是意外。
6. **字符串带上下文发布。** 译者看到 `"Book"` 无从知道是名词还是动词。每条消息都带描述，并在有用处附上截图引用。
7. **端到端正确处理 Unicode。** 输入边界处 NFC 规范化、用 locale 感知的 collation 比较、在 grapheme cluster 边界截断（绝不按字节或 UTF-16 单元）、绝不在没有 locale 的情况下大小写转换。
8. **Locale 是用户选择加协商，绝不仅凭 IP 地理定位。** 尊重 `Accept-Language` 和显式用户偏好；有意定义回退链（`pt-BR → pt → en`）。
