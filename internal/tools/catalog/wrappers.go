package catalog

import (
	"google.golang.org/genai"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/agenttool"
	"google.golang.org/adk/tool/exampletool"
	"google.golang.org/adk/tool/geminitool"
)

// GeminiTool 见 geminitool.New（ADK 对齐，用于任意 genai.Tool）。
func GeminiTool(name, description string, gt *genai.Tool) tool.Tool {
	return geminitool.New(name, description, gt)
}

// WrapAgent 将子 Agent 包装为 tool.Tool（ADK agenttool）。
func WrapAgent(sub agent.Agent, cfg *agenttool.Config) tool.Tool {
	return agenttool.New(sub, cfg)
}

// ExampleFewShot 安装 few-shot example 请求增强（见 exampletool）。
func ExampleFewShot(cfg exampletool.ExampleToolConfig) (tool.Tool, error) {
	t, err := exampletool.New(cfg)
	if err != nil {
		return nil, err
	}
	return t, nil
}
