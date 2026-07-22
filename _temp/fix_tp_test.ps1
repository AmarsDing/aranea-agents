# Repair mojibake in task_planner_impl_test.go by line-number replacement.
$path = "f:\aranea-agents\internal\agent\task_planner_impl_test.go"
$T = "`t"
$lines = [System.IO.File]::ReadAllLines($path, [System.Text.Encoding]::UTF8)

$repl = @{
  36  = $T+$T+$T + 'UserMessage:    strings.Repeat("请帮我分析这个中等长度的任务，包含一些上下文信息。", 12),'
  45  = $T+$T+$T+$T + 'strings.Repeat("这是一个复杂的研究任务，需要深入分析。", 30),'
  131 = $T+$T + '{"组建团队 keyword", "请组建团队完成这个项目", true},'
  133 = $T+$T + '{"团队a团队b keyword", "团队A写诗，团队B写代码", true},'
  134 = $T+$T + '{"多个团队 keyword", "需要多个团队协作完成", true},'
  136 = $T+$T + '{"real team prompt", "请组建两个团队并行工作：团队A写一首关于春天的五言绝句，团队B写一首关于秋天的五言绝句。两首诗完成后请汇总对比。", true},'
  139 = $T+$T + '{"simple question", "今天天气怎么样", false},'
  167 = '//   - dag: N teams, each team has >= 1 members collaborating'
  244 = '//   - team formation keywords ("分派N个团队", "组建团队", "团队协作") -> dag:'
  245 = '//     user expects one or more multi-member teams (>= 1 members each).'
  246 = '//   - parallel keywords ("并行", "同时执行") -> parallel: independent concurrent'
  256 = '// "分派N个团队"/"组建团队" implies multi-member teams -> dag'
  257 = $T+$T + '{"two teams Chinese", "分派两个团队分别负责代码分析和数据分析", "dag"},'
  258 = $T+$T + '{"two teams English mix", "分派两个team进行，一个负责代码分析，一个负责模拟数据分析", "dag"},'
  259 = $T+$T + '{"multiple teams", "需要多个团队协作完成", "dag"},'
  261 = $T+$T + '{"team formation Chinese", "请组建团队完成这个项目", "dag"},'
  269 = $T+$T + '{"parallel processing keyword", "并行处理多个子任务", "parallel"},'
  273 = $T+$T + '{"single task", "写一首关于春天的诗", ""},'
  274 = $T+$T + '{"question", "什么是五言绝句？", ""},'
  290 = '// 2026-07-04 问题 2 修复：detectTeamCount 用于在 decomposeTask 中约束 LLM'
  291 = @('// 生成恰好 N 个 subtask，避免 orchestrateDAG 多创建 team。', 'func TestDetectTeamCount(t *testing.T) {')
  297 = $T+$T + '// 阿拉伯数字 + 量词 + team/团队'
  298 = $T+$T + '{"2个团队", "分派2个团队分别负责代码分析和数据分析", 2},'
  299 = $T+$T + '{"3个团队", "组建3个团队协作完成", 3},'
  300 = $T+$T + '{"2支团队", "分派2支团队", 2},'
  303 = $T+$T + '{"digit without 量词", "我需要3团队", 3},'
  309 = $T+$T + '{"十个团队", "需要十个团队", 10},'
  321 = $T+$T + '{"unrelated number", "我需要5分钟完成", 0},'
  417 = @($T + '// 上游：LLM 提供的 deliverables 原样保留（名称不被兜底覆盖）。', $T + 'if len(subTasks[0].Deliverables) != 1 {')
  424 = @($T + '// 下游：LLM 提供的 input_contract 原样保留，且不再追加兜底派生项。', $T + 'if len(subTasks[1].InputContract) != 1 {')
  425 = $T+$T + 't.Fatalf("InputContract = %d, want 1 (LLM 已提供时不追加兜底)", len(subTasks[1].InputContract))'
  453 = @($T + '// 生产者兜底：有下游依赖但 LLM 未输出 deliverables -> 派生 {step_id}_output。', $T + 'if len(research.Deliverables) != 1 {')
  462 = $T + '// 消费者兜底：有 depends_on 且 LLM 未输出 input_contract -> 每个上游派生一项，'
  463 = @($T + '// 名称与上游兜底 deliverable 一致（构造即匹配）。', $T + 'if len(write.InputContract) != 1 {')
  471 = @($T + '// 无依赖边：solo 既不是生产者也不是消费者，不派生任何契约。', $T + 'if len(solo.Deliverables) != 0 || len(solo.InputContract) != 0 {')
  474 = @($T + '// 消费者自身若无下游依赖，不派生 deliverables。', $T + 'if len(write.Deliverables) != 0 {')
  510 = $T+$T+$T+$T + '{Name: "spec", Type: "document", Format: "markdown", Description: "设计文档"},'
  564 = $T + '// nil bus -> no panic.'
  568 = $T + '// empty session -> skipped.'
}

$out = New-Object System.Collections.Generic.List[string]
for ($i = 0; $i -lt $lines.Count; $i++) {
  $n = $i + 1
  if ($repl.ContainsKey($n)) {
    foreach ($x in $repl[$n]) { $out.Add([string]$x) }
  } else {
    $out.Add($lines[$i])
  }
}
$utf8 = New-Object System.Text.UTF8Encoding($false)
[System.IO.File]::WriteAllLines($path, $out, $utf8)
Write-Output "done: $($lines.Count) -> $($out.Count) lines"
