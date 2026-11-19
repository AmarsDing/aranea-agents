// ceoboard-natsprobe 一次性探测工具(整合 P2-10,自 ceo-dashboard/cmd/natsprobe
// 逐字移植):列出 NATS 实例的 JetStream streams/subjects/消费者,供适配器设计取证。
//
// 评审 2026-11-09 D6:任一目标探测失败(连接失败/JetStream 不可用)进程以
// 非零码退出——原实现打印「连接失败」后照常 exit 0,CI/脚本无法感知全灭探测;
// 摘要输出形态不变,仅返回码修正。全部目标失败也好、单个失败也好,只要有失败
// 即 exit 1(明细已逐目标打印到 stdout,足以定位)。
//
// 用法:
//
//	go run ./cmd/tools/ceoboard-natsprobe [name=url ...]
//	无参数时探测两套默认实例(HCF/NovaTrader,见 configs/workflows 的 endpoint 声明)。
package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
)

// probe 探测单个实例;失败时错误已写入 w 并作为返回值上抛(D6)。
func probe(name, url string, w io.Writer) error {
	fmt.Fprintf(w, "=== %s %s ===\n", name, url)
	nc, err := nats.Connect(url, nats.Timeout(3*time.Second), nats.Name("ceoboard-probe"))
	if err != nil {
		fmt.Fprintf(w, "连接失败: %v\n", err)
		return fmt.Errorf("%s: %w", name, err)
	}
	defer nc.Close()
	js, err := nc.JetStream()
	if err != nil {
		fmt.Fprintf(w, "JetStream 上下文失败: %v\n", err)
		return fmt.Errorf("%s: %w", name, err)
	}
	for stream := range js.StreamsInfo(nats.MaxWait(3 * time.Second)) {
		fmt.Fprintf(w, "stream=%s subjects=%v msgs=%d bytes=%d\n", stream.Config.Name, stream.Config.Subjects, stream.State.Msgs, stream.State.Bytes)
		for ci := range js.ConsumersInfo(stream.Config.Name, nats.MaxWait(3*time.Second)) {
			fmt.Fprintf(w, "  consumer=%s durable=%s filter=%s pending=%d\n", ci.Name, ci.Config.Durable, ci.Config.FilterSubject, ci.NumPending)
		}
	}
	return nil
}

// run 逐目标探测并汇总退出码:任一目标失败 → 1,全成功 → 0(纯函数,供测试)。
func run(targets [][2]string, w io.Writer) int {
	failed := false
	for _, t := range targets {
		if err := probe(t[0], t[1], w); err != nil {
			failed = true
		}
	}
	if failed {
		return 1
	}
	return 0
}

func main() {
	targets := [][2]string{
		{"HCF", "nats://192.168.4.44:14222"},
		{"NovaTrader", "nats://192.168.4.44:24222"},
	}
	if len(os.Args) > 1 {
		targets = targets[:0]
		for _, arg := range os.Args[1:] {
			name, url, ok := strings.Cut(arg, "=")
			if !ok || name == "" || url == "" {
				fmt.Fprintln(os.Stderr, "参数须为 name=url 形式,如 HCF=nats://192.168.4.44:14222")
				os.Exit(2)
			}
			targets = append(targets, [2]string{name, url})
		}
	}
	os.Exit(run(targets, os.Stdout))
}
