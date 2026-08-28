package biz

import "testing"

func TestShouldDropLifestyleMemories(t *testing.T) {
	t.Parallel()
	if !ShouldDropLifestyleMemories("核对生产环境并生成报告") {
		t.Fatal("task query must drop lifestyle memories")
	}
	if ShouldDropLifestyleMemories("周末去吃日料") {
		t.Fatal("lifestyle query must keep lifestyle memories")
	}
	if ShouldDropLifestyleMemories("今天天气怎么样") {
		t.Fatal("non-task query must not drop lifestyle memories")
	}
}

func TestLifestyleMemoryText(t *testing.T) {
	t.Parallel()
	if !LifestyleMemoryText("用户喜欢吃日料和寿司") {
		t.Fatal("want lifestyle")
	}
	if LifestyleMemoryText("Prefers Go") {
		t.Fatal("work fact must not match")
	}
}
