package graphrag

import (
	"reflect"
	"testing"
)

func TestDetectCommunities_TwoCliques(t *testing.T) {
	edges := []LinkEdge{
		{SrcDocID: "a1", DstDocID: "a2", Weight: 1},
		{SrcDocID: "a1", DstDocID: "a3", Weight: 1},
		{SrcDocID: "a2", DstDocID: "a3", Weight: 1},
		{SrcDocID: "b1", DstDocID: "b2", Weight: 1},
		{SrcDocID: "b1", DstDocID: "b3", Weight: 1},
		{SrcDocID: "b2", DstDocID: "b3", Weight: 1},
	}
	got := DetectCommunities(edges, 2)
	if len(got) != 6 {
		t.Fatalf("节点数 = %d，期望 6", len(got))
	}
	if !(got["a1"] == got["a2"] && got["a2"] == got["a3"]) {
		t.Fatalf("a 团未收敛到同一社区: %v", got)
	}
	if !(got["b1"] == got["b2"] && got["b2"] == got["b3"]) {
		t.Fatalf("b 团未收敛到同一社区: %v", got)
	}
	if got["a1"] == got["b1"] {
		t.Fatalf("两团被并入同一社区: %v", got)
	}
	// 确定性：同一输入重跑结果逐键一致。
	if again := DetectCommunities(edges, 2); !reflect.DeepEqual(got, again) {
		t.Fatalf("重跑不一致: %v vs %v", got, again)
	}
}

func TestDetectCommunities_SyncSnapshotExact(t *testing.T) {
	edges := []LinkEdge{{SrcDocID: "a", DstDocID: "b", Weight: 1}}
	// 同步更新：1 轮互换标签，2 轮回到初始分配。
	if got := DetectCommunities(edges, 1); !reflect.DeepEqual(got, map[string]int{"a": 1, "b": 0}) {
		t.Fatalf("1 轮结果 = %v", got)
	}
	if got := DetectCommunities(edges, 2); !reflect.DeepEqual(got, map[string]int{"a": 0, "b": 1}) {
		t.Fatalf("2 轮结果 = %v", got)
	}
	// iterations<=0 直接返回初始分配。
	if got := DetectCommunities(edges, 0); !reflect.DeepEqual(got, map[string]int{"a": 0, "b": 1}) {
		t.Fatalf("0 轮结果 = %v", got)
	}
}

func TestDetectCommunities_WeightedVote(t *testing.T) {
	// a-x 重边压制 x-b 轻边：1 轮后 x 投 a 的社区。
	edges := []LinkEdge{
		{SrcDocID: "a", DstDocID: "x", Weight: 5},
		{SrcDocID: "x", DstDocID: "b", Weight: 1},
	}
	want := map[string]int{"a": 2, "b": 2, "x": 0}
	if got := DetectCommunities(edges, 1); !reflect.DeepEqual(got, want) {
		t.Fatalf("加权票数结果 = %v，期望 %v", got, want)
	}
}

func TestDetectCommunities_DuplicateEdgesAggregate(t *testing.T) {
	// 同一无序对的重复边聚合（1+2=3 > 1.5），且方向无关。
	edges := []LinkEdge{
		{SrcDocID: "a", DstDocID: "b", Weight: 1},
		{SrcDocID: "b", DstDocID: "a", Weight: 2},
		{SrcDocID: "a", DstDocID: "c", Weight: 1.5},
	}
	want := map[string]int{"a": 1, "b": 0, "c": 0}
	if got := DetectCommunities(edges, 1); !reflect.DeepEqual(got, want) {
		t.Fatalf("聚合结果 = %v，期望 %v", got, want)
	}
}

func TestDetectCommunities_EmptyAndSelfLoop(t *testing.T) {
	if got := DetectCommunities(nil, 2); len(got) != 0 {
		t.Fatalf("空边结果 = %v", got)
	}
	// 自环忽略：只含自环时无有效节点。
	if got := DetectCommunities([]LinkEdge{{SrcDocID: "a", DstDocID: "a", Weight: 1}}, 2); len(got) != 0 {
		t.Fatalf("自环结果 = %v", got)
	}
}
