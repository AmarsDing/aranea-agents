// 直接安装 alibabacloud-find-skills（等同 system_admin 的 cli_admin_skill_install_from_url 路径）
//go:build ignore

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"aranea-agents/internal/pkginstall"
)

func main() {
	// git clone 在本进程发生，需走本地 socks5 代理（远端 DNS）
	os.Setenv("ARANEA_GIT_PROXY", "socks5h://127.0.0.1:1080")

	// 1. 登录拿 token
	loginBody, _ := json.Marshal(map[string]string{"username": "admin", "password": "changeme"})
	resp, err := http.Post("http://127.0.0.1:8000/v1/admins/login", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	var token string
	for _, c := range resp.Cookies() {
		if c.Name == "access_token" {
			token = c.Value
		}
	}
	if token == "" {
		b, _ := io.ReadAll(resp.Body)
		panic("no access_token cookie: " + string(b))
	}
	fmt.Println("login ok")

	// 2. 安装 alibabacloud-find-skills
	manifest := &pkginstall.Manifest{
		Version:  1,
		Metadata: pkginstall.ManifestMetadata{Name: "aliyun-find-skills-install"},
		Spec: pkginstall.ManifestSpec{Skills: []pkginstall.SkillSpec{{
			URL:      "https://github.com/aliyun/alibabacloud-aiops-skills",
			Subpath:  "skills/developertools/solutions/alibabacloud-find-skills",
			Decision: "keep", // 幂等：已存在则覆盖更新
		}}},
	}
	ins := &pkginstall.Installer{
		APIURL: "http://127.0.0.1:8000",
		Token:  token,
		Quiet:  false,
	}
	result, err := ins.Install("", manifest)
	if err != nil {
		fmt.Println("INSTALL ERROR:", err)
		os.Exit(1)
	}
	out, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(out))
}
