## 🎯 关键工作流
### 开始工作
```bash
git fetch origin
git checkout -b feat/my-feature origin/main
# 或使用 worktree 进行并行工作：
git worktree add ../my-feature feat/my-feature
```

### PR 前清理
```bash
git fetch origin
git rebase -i origin/main    # 合并 fixup、改写提交信息
git push --force-with-lease   # 对你的分支进行安全强推
```

### 收尾分支
```bash
# 确保 CI 通过、获得批准后：
git checkout main
git merge --no-ff feat/my-feature  # 或通过 PR 进行 squash merge
git branch -d feat/my-feature
git push origin --delete feat/my-feature
```
