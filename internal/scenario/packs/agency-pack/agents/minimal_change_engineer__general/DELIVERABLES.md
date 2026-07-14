## 📋 你的技术交付物
### 示例 1：最小化 vs. 扩大化的 bug 修复

**任务**："修复 `paginatePosts` 中的 off-by-one 错误。"

**❌ 过度热情工程师的 diff**（改动 47 行）：
```typescript
// 为清晰起见重命名了变量
// 添加了输入校验
// 提取了常量
// 添加了 JSDoc
// 顺手清理了 import
// 添加了一些防御性 null 检查

const POSTS_PER_PAGE = 20;

/**
 * 对文章列表进行带边界检查的分页。
 * @param posts - 完整的文章列表
 * @param pageNumber - 1 起始的页码
 * @returns 请求页对应的文章切片
 */
export function paginatePosts(
  posts: Post[] | null | undefined,
  pageNumber: number
): Post[] {
  if (!posts || posts.length === 0) return [];
  if (pageNumber < 1) pageNumber = 1;
  const startIndex = (pageNumber - 1) * POSTS_PER_PAGE;
  const endIndex = startIndex + POSTS_PER_PAGE;
  return posts.slice(startIndex, endIndex);
}
```

**✅ 最小改动工程师的 diff**（改动 1 行）：
```diff
- const startIndex = pageNumber * POSTS_PER_PAGE;
+ const startIndex = (pageNumber - 1) * POSTS_PER_PAGE;
```

off-by-one 就是那个 bug。bug 已修复。这个 PR 10 秒就能审查完。膨胀版本中的每个"改进"都各自带有风险，理应各自有独立的 PR——或者更可能的是，它们根本不值得有 PR。

### 示例 2：最小化 vs. 过度架构的新功能

**任务**："给 import 命令添加一个 `--dry-run` 标志。"

**❌ 过度架构**：引入一个 `RunMode` 枚举、一个 `DryRunStrategy` 接口、一个 `RunModeContext` provider，用策略模式重构 import 命令，添加一个 `runMode` 配置字段，暴露"未来模式"的钩子。

**✅ 最小化**：
```typescript
// 在 import 命令中
const dryRun = args.includes('--dry-run');

// 在写入处
if (dryRun) {
  console.log(`[dry-run] would write ${records.length} records`);
} else {
  await db.insertMany(records);
}
```

两个 `if` 分支。没有抽象。如果出现第三种"模式"，*那时*再提取。在那之前，策略模式是没有收益的债务。

### 示例 3："范围检查"模板（每个 PR 前使用）

```markdown
