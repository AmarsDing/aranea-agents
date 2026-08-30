package biz

import "testing"

func TestLooksLikeKnowledgeQuery(t *testing.T) {
	t.Parallel()
	positive := []string{
		// S03 原语料（session-eval-20260829-r2）
		"AWOS 的 LT31 无前向散射告警但 MOR 骤降，可能是什么原因？结合知识库回答。",
		"这个原因在知识库里有对应的处置 SOP 吗？",
		// 变体
		"知识库中有没有关于容灾的文档",
		"查一下我们的文档库",
		"资料库里有吗",
		"is this in our knowledge base?",
		"check the knowledge_base first",
		"运维手册里怎么规定的",
		"有没有对应的处置预案",
		// 任务动作不 veto：加能力不免义务
		"核对知识库数据并生成巡检报告",
	}
	for _, s := range positive {
		if !LooksLikeKnowledgeQuery(s) {
			t.Errorf("expected knowledge query: %q", s)
		}
	}
	negative := []string{
		"",
		"   ",
		"今天天气怎么样",
		"推荐三本书",
		"帮我写一份手册",   // 裸「手册」不收——写手册不是检索诉求
		"写一份 SOP 文档",  // 裸「SOP」不收（S03-t2 由「知识库」命中）
		"如何做红烧排骨",
		"为什么天空是蓝色的",
		"你好",
	}
	for _, s := range negative {
		if LooksLikeKnowledgeQuery(s) {
			t.Errorf("unexpected knowledge query: %q", s)
		}
	}
}
