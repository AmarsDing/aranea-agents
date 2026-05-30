package biz

import (
	"context"
)

type AgentTemplate struct {
	Key         string
	Label       string
	Icon        string
	Description string
	DisplayName string
	Provider    string
	Model       string
}

type AgentTemplateRepo interface {
	ListAgentTemplates(ctx context.Context) ([]AgentTemplate, error)
}

type AgentTemplateUsecase struct {
	repo AgentTemplateRepo
}

func NewAgentTemplateUsecase(repo AgentTemplateRepo) *AgentTemplateUsecase {
	return &AgentTemplateUsecase{repo: repo}
}

func (u *AgentTemplateUsecase) List(ctx context.Context) ([]AgentTemplate, error) {
	items, err := u.repo.ListAgentTemplates(ctx)
	if err != nil {
		return nil, err
	}
	if len(items) > 0 {
		return items, nil
	}
	return ListAgentTemplates(), nil
}

func ListAgentTemplates() []AgentTemplate {
	return []AgentTemplate{
		{Key: "fox", Label: "小狐", Icon: "pets", DisplayName: "小狐助手", Provider: "openrouter", Model: "gpt-4.1-mini", Description: "温柔、敏捷，擅长把复杂问题拆成清晰步骤。"},
		{Key: "programmer", Label: "程序员", Icon: "code", DisplayName: "研发助手", Provider: "openrouter", Model: "gpt-4.1-mini", Description: "资深研发工程师，关注架构、代码质量、测试与可维护性。"},
		{Key: "support", Label: "客服", Icon: "support_agent", DisplayName: "客服助手", Provider: "openrouter", Model: "gpt-4.1-mini", Description: "耐心、清晰，优先解决用户问题并记录上下文。"},
		{Key: "writer", Label: "写手", Icon: "edit_note", DisplayName: "写作助手", Provider: "openrouter", Model: "gpt-4.1-mini", Description: "擅长品牌文案、结构化写作、润色与多语种表达。"},
		{Key: "translator", Label: "翻译", Icon: "translate", DisplayName: "翻译助手", Provider: "openrouter", Model: "gpt-4.1-mini", Description: "忠实、准确地进行中英互译，并保留术语一致性。"},
		{Key: "luo", Label: "小罗", Icon: "bolt", DisplayName: "执行助手", Provider: "openrouter", Model: "gpt-4.1-mini", Description: "执行力强，适合任务推进、复盘和状态同步。"},
		{Key: "mimi", Label: "米米", Icon: "auto_awesome", DisplayName: "创意助手", Provider: "openrouter", Model: "gpt-4.1-mini", Description: "轻松友好，擅长创意发散、陪伴式讨论和灵感整理。"},
	}
}
