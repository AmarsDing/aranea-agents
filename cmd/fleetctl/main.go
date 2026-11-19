// fleetctl 是运行台编排里 agent 用的命令行：协调者用它建任务、派 worker、读收件箱，
// worker 用它汇报结果、提问。它只在运行台起的终端里可用——终端环境里注入了
// ARANEA_FLEET_URL（回连地址）与 ARANEA_FLEET_TOKEN（该终端独有的令牌）。
//
// 用法：fleetctl <命令> [--键 值 ...]   例：fleetctl report --outcome succeeded --body "done"
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const usage = `fleetctl <命令> [--键 值 ...]

协调者：task-create  worker-start  status  check  worker-read  send  answer  worker-stop  done
worker：report  ask  status

示例：
  fleetctl task-create --title "补单测" --spec "给 office 模块补单元测试，覆盖率 ≥80%"
  fleetctl worker-start --task task_xxx --agent codex
  fleetctl report --outcome succeeded --body "已补 12 个用例" --files "a_test.go,b_test.go"`

func main() {
	if len(os.Args) < 2 || os.Args[1] == "-h" || os.Args[1] == "--help" || os.Args[1] == "help" {
		fmt.Println(usage)
		return
	}
	url, token := os.Getenv("ARANEA_FLEET_URL"), os.Getenv("ARANEA_FLEET_TOKEN")
	if url == "" || token == "" {
		fail("fleetctl 只能在运行台起的终端里使用（缺少 ARANEA_FLEET_URL / ARANEA_FLEET_TOKEN）")
	}
	args, err := parseArgs(os.Args[2:])
	if err != nil {
		fail(err.Error())
	}
	body, _ := json.Marshal(map[string]any{"cmd": os.Args[1], "args": args})
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		fail(err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		fail("连接运行台失败：" + err.Error())
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out struct {
		OK     bool   `json:"ok"`
		Output string `json:"output"`
		Error  string `json:"error"`
	}
	if json.Unmarshal(raw, &out) != nil {
		fail(fmt.Sprintf("运行台返回了无法解析的结果（HTTP %d）：%s", resp.StatusCode, strings.TrimSpace(string(raw))))
	}
	if !out.OK {
		fail(out.Error)
	}
	fmt.Println(out.Output)
}

// parseArgs 支持 --k v 与 --k=v；值缺省视为 "true"。
func parseArgs(in []string) (map[string]string, error) {
	out := map[string]string{}
	for i := 0; i < len(in); i++ {
		a := in[i]
		if !strings.HasPrefix(a, "--") {
			return nil, fmt.Errorf("参数要写成 --键 值，无法识别：%s", a)
		}
		k := strings.TrimPrefix(a, "--")
		if j := strings.Index(k, "="); j >= 0 {
			out[k[:j]] = k[j+1:]
			continue
		}
		if i+1 < len(in) && !strings.HasPrefix(in[i+1], "--") {
			out[k] = in[i+1]
			i++
		} else {
			out[k] = "true"
		}
	}
	return out, nil
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "fleetctl: "+msg)
	os.Exit(1)
}
