// ceoboard-natsprobe 一次性探测工具(整合 P2-10,自 ceo-dashboard/cmd/natsprobe
// 逐字移植):列出 NATS 实例的 JetStream streams/subjects/消费者,供适配器设计取证。
//
// 用法:
//
//	go run ./cmd/tools/ceoboard-natsprobe [name=url ...]
//	无参数时探测两套默认实例(HCF/NovaTrader,见 configs/workflows 的 endpoint 声明)。
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
)

func probe(name, url string) {
	fmt.Printf("=== %s %s ===\n", name, url)
	nc, err := nats.Connect(url, nats.Timeout(3*time.Second), nats.Name("ceoboard-probe"))
	if err != nil {
		fmt.Printf("连接失败: %v\n", err)
		return
	}
	defer nc.Close()
	js, err := nc.JetStream()
	if err != nil {
		fmt.Printf("JetStream 上下文失败: %v\n", err)
		return
	}
	for stream := range js.StreamsInfo(nats.MaxWait(3 * time.Second)) {
		fmt.Printf("stream=%s subjects=%v msgs=%d bytes=%d\n", stream.Config.Name, stream.Config.Subjects, stream.State.Msgs, stream.State.Bytes)
		for ci := range js.ConsumersInfo(stream.Config.Name, nats.MaxWait(3*time.Second)) {
			fmt.Printf("  consumer=%s durable=%s filter=%s pending=%d\n", ci.Name, ci.Config.Durable, ci.Config.FilterSubject, ci.NumPending)
		}
	}
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
	for _, t := range targets {
		probe(t[0], t[1])
	}
}
