// goldeval — G-13 金标快照驱动命令。
//
// 把入仓的 fixture 语料 + 金标评测集跑过确定性检索器（成链→切分→一跳扩展→
// 合并截断，全生产组件、无 PG/embedder 依赖），与入仓快照做 before/after diff。
//
// 用法（仓库根目录）：
//
//	go run ./internal/knowledge/eval/cmd/goldeval [flags]
//
// 注意：二进制经 internal/knowledge 传递依赖 pkg/auth，其 init 在非 dev/CI
// 环境缺 KRATOS_AUTH_SECRET 时 panic。本地运行先置 DEPLOY_ENV=dev（或 CI=true）；
// GitHub Actions 自带 CI=true 无需处理。本地刷新快照更顺手的等价入口是
// `go test ./internal/knowledge/eval/ -run TestGoldSnapshot -gold.update`
// （测试二进制走 isGoTestBinary 豁免，无需环境变量）。
//
// 退出码：0 无回归；1 运行错误（输入/环境/文件读写失败）；2 门禁违规——
// recall 回归、chunk MRR 回归、用例检索失败、集与快照漂移、快照过期
// （set name/topK 不一致）、入库基线本身降级。门禁判定单一事实源在
// eval 包 gate.go（CheckGoldGate），与 TestGoldSnapshotRegression 共用。
// CI（.github/workflows/ci.yml 的 Knowledge gold recall gate 步）以非零退出码卡 PR。
//
// 刷新快照：-update（评测集/语料变更或有意的指标改进，随代码同 PR 提交）。
// -update 会经过 eval.ValidateGoldBaseline——任何带错或非满分用例都拒绝写入，
// 垃圾/降级基线囤库只会让后续门禁对着弱化参照量假绿。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"aranea-agents/internal/knowledge/eval"
)

var (
	corpusPath   = flag.String("corpus", "internal/knowledge/eval/testdata/gold-corpus.json", "fixture 语料 JSON")
	setPath      = flag.String("set", "internal/knowledge/eval/testdata/gold-set.json", "金标评测集 JSON")
	baselinePath = flag.String("baseline", "internal/knowledge/eval/testdata/gold-snapshot.json", "基线快照 JSON")
	topK         = flag.Int("topk", 8, "检索窗口（必须与快照一致）")
	update       = flag.Bool("update", false, "用当前运行结果重写基线快照（带错/降级用例一律拒写）")
	outPath      = flag.String("out", "", "非空时把本次 RunReport JSON 写到该路径")
	mdPath       = flag.String("markdown", "", "非空时写入 Markdown 报告（全量报告 + diff）")
)

func main() {
	flag.Parse()
	os.Exit(run(context.Background()))
}

func run(ctx context.Context) int {
	corpus, err := eval.LoadFixtureCorpus(*corpusPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "goldeval:", err)
		return 1
	}
	set, err := eval.LoadEvalSet(*setPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "goldeval:", err)
		return 1
	}
	if err := eval.ValidateGoldExpectations(set); err != nil {
		fmt.Fprintln(os.Stderr, "goldeval:", err)
		return 1
	}
	searcher, err := eval.NewFixtureSearcher(corpus)
	if err != nil {
		fmt.Fprintln(os.Stderr, "goldeval:", err)
		return 1
	}
	report, err := eval.NewRunner(searcher).Run(ctx, set, *topK)
	if err != nil {
		fmt.Fprintln(os.Stderr, "goldeval:", err)
		return 1
	}

	if *outPath != "" {
		if err := eval.SaveRunReport(*outPath, report); err != nil {
			fmt.Fprintln(os.Stderr, "goldeval:", err)
			return 1
		}
	}

	if *update {
		if err := eval.ValidateGoldBaseline(report); err != nil {
			fmt.Fprintln(os.Stderr, "goldeval:", err)
			return 1
		}
		if err := eval.SaveRunReport(*baselinePath, report); err != nil {
			fmt.Fprintln(os.Stderr, "goldeval:", err)
			return 1
		}
		fmt.Printf("goldeval: baseline updated (%d cases, chunk recall %.3f, doc recall %.3f)\n",
			len(report.Cases), report.ChunkAgg.RecallAtK, report.DocAgg.RecallAtK)
		if *mdPath != "" {
			if err := os.WriteFile(*mdPath, []byte(eval.RenderMarkdown(report)), 0o644); err != nil {
				fmt.Fprintln(os.Stderr, "goldeval:", err)
				return 1
			}
		}
		return 0
	}

	base, err := eval.LoadRunReport(*baselinePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "goldeval:", err)
		return 1
	}

	// Markdown 无论最终结果如何都先落盘，供 CI job summary 汇总。
	diff := eval.DiffReports(base, report)
	if *mdPath != "" {
		md := eval.RenderMarkdown(report) + "\n" + eval.RenderDiffMarkdown(diff)
		if err := os.WriteFile(*mdPath, []byte(md), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "goldeval:", err)
			return 1
		}
	}

	violations := eval.CheckGoldGate(base, report)
	fmt.Printf("goldeval: cases=%d violations=%d regressions=%d chunk recall %.3f→%+.3f doc recall %.3f→%+.3f\n",
		len(report.Cases), len(violations), diff.RegressionCount,
		diff.Chunk.RecallAtK.Base, diff.Chunk.RecallAtK.Delta,
		diff.Doc.RecallAtK.Base, diff.Doc.RecallAtK.Delta)
	if len(violations) == 0 {
		return 0
	}
	for _, v := range violations {
		fmt.Println("goldeval:", v.String())
	}
	return 2
}
