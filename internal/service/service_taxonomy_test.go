package service_test

import (
	"testing"

	taxv1 "aranea-agents/api/kratos/taxonomy/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/service"
)

func TestToProtoTaxonomy(t *testing.T) {
	c := biz.OrganizationNode{
		ID:           "cat-1",
		Key:          "general",
		Name:         "通用",
		Description:  "通用分类",
		Status:       "active",
		Enabled:      true,
		SortOrder:    1,
		ParentID:     "",
		Level:        "1",
		WorkspaceID:  "ws-1",
		OwnerUserID:  "user-1",
		IsSystem:     true,
		ConfigJSON:   `{"color":"blue"}`,
		MetadataJSON: `{"order":1}`,
		CreatedAt:    "2024-01-01",
		UpdatedAt:    "2024-06-01",
		DeletedAt:    "",
	}
	pb := service.ToProtoTaxonomy(c)
	if pb.GetId() != "cat-1" || pb.GetKey() != "general" {
		t.Fatalf("id/key mismatch: id=%q key=%q", pb.GetId(), pb.GetKey())
	}
	if pb.GetName() != "通用" || pb.GetDescription() != "通用分类" {
		t.Fatalf("name/desc mismatch: name=%q desc=%q", pb.GetName(), pb.GetDescription())
	}
	if pb.GetStatus() != "active" || !pb.GetEnabled() {
		t.Fatalf("status/enabled mismatch: status=%q enabled=%v", pb.GetStatus(), pb.GetEnabled())
	}
	if pb.GetSortOrder() != 1 || pb.GetParentId() != "" {
		t.Fatalf("sort/parent mismatch: sort=%d parent=%q", pb.GetSortOrder(), pb.GetParentId())
	}
	if pb.GetLevel() != "1" || pb.GetWorkspaceId() != "ws-1" {
		t.Fatalf("level/workspace mismatch: level=%q ws=%q", pb.GetLevel(), pb.GetWorkspaceId())
	}
	if pb.GetOwnerUserId() != "user-1" || !pb.GetIsSystem() {
		t.Fatalf("owner/system mismatch: owner=%q system=%v", pb.GetOwnerUserId(), pb.GetIsSystem())
	}
	if pb.GetConfigJson() != `{"color":"blue"}` || pb.GetMetadataJson() != `{"order":1}` {
		t.Fatalf("config/metadata mismatch: config=%q meta=%q", pb.GetConfigJson(), pb.GetMetadataJson())
	}
	if pb.GetCreatedAt() != "2024-01-01" || pb.GetUpdatedAt() != "2024-06-01" {
		t.Fatalf("timestamps mismatch: created=%q updated=%q", pb.GetCreatedAt(), pb.GetUpdatedAt())
	}
}

func TestFromProtoTaxonomy_Nil(t *testing.T) {
	got := service.FromProtoTaxonomy(nil)
	if got.ID != "" || got.Key != "" {
		t.Fatalf("expected zero-value TaxonomyNode, got %+v", got)
	}
}

func TestFromProtoTaxonomy_ToProtoTaxonomy_RoundTrip(t *testing.T) {
	pb := &taxv1.TaxonomyNode{
		Id:           "cat-2",
		Key:          "dev",
		Name:         "开发",
		Description:  "开发分类",
		Status:       "active",
		Enabled:      true,
		SortOrder:    2,
		ParentId:     "cat-1",
		Level:        "2",
		WorkspaceId:  "ws-2",
		OwnerUserId:  "user-2",
		IsSystem:     false,
		ConfigJson:   `{"color":"red"}`,
		MetadataJson: `{"order":2}`,
		CreatedAt:    "2024-02-01",
		UpdatedAt:    "2024-07-01",
		DeletedAt:    "",
	}
	bizCat := service.FromProtoTaxonomy(pb)
	if bizCat.ID != "cat-2" || bizCat.Key != "dev" {
		t.Fatalf("id/key mismatch: id=%q key=%q", bizCat.ID, bizCat.Key)
	}
	if bizCat.Name != "开发" || bizCat.Description != "开发分类" {
		t.Fatalf("name/desc mismatch: name=%q desc=%q", bizCat.Name, bizCat.Description)
	}
	if bizCat.Status != "active" || !bizCat.Enabled {
		t.Fatalf("status/enabled mismatch: status=%q enabled=%v", bizCat.Status, bizCat.Enabled)
	}
	if bizCat.SortOrder != 2 || bizCat.ParentID != "cat-1" {
		t.Fatalf("sort/parent mismatch: sort=%d parent=%q", bizCat.SortOrder, bizCat.ParentID)
	}
	if bizCat.Level != "2" || bizCat.WorkspaceID != "ws-2" {
		t.Fatalf("level/workspace mismatch: level=%q ws=%q", bizCat.Level, bizCat.WorkspaceID)
	}
	if bizCat.OwnerUserID != "user-2" || bizCat.IsSystem {
		t.Fatalf("owner/system mismatch: owner=%q system=%v", bizCat.OwnerUserID, bizCat.IsSystem)
	}
	if bizCat.ConfigJSON != `{"color":"red"}` || bizCat.MetadataJSON != `{"order":2}` {
		t.Fatalf("config/metadata mismatch: config=%q meta=%q", bizCat.ConfigJSON, bizCat.MetadataJSON)
	}

	pb2 := service.ToProtoTaxonomy(bizCat)
	if pb2.GetId() != pb.GetId() || pb2.GetKey() != pb.GetKey() {
		t.Fatalf("round-trip id/key mismatch")
	}
	if pb2.GetName() != pb.GetName() || pb2.GetDescription() != pb.GetDescription() {
		t.Fatalf("round-trip name/desc mismatch")
	}
	if pb2.GetSortOrder() != pb.GetSortOrder() || pb2.GetParentId() != pb.GetParentId() {
		t.Fatalf("round-trip sort/parent mismatch")
	}
	if pb2.GetIsSystem() == pb.GetIsSystem() && pb.GetIsSystem() == false {
	} else if pb2.GetIsSystem() != pb.GetIsSystem() {
		t.Fatalf("round-trip is_system mismatch: %v vs %v", pb2.GetIsSystem(), pb.GetIsSystem())
	}
}

func TestToTaxonomyTreeNode_Nil(t *testing.T) {
	if got := service.ToTaxonomyTreeNode(nil); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestToTaxonomyTreeNode_Single(t *testing.T) {
	n := &biz.OrganizationTreeNode{
		Category: biz.OrganizationNode{
			ID:   "cat-1",
			Key:  "root",
			Name: "Root",
		},
		Children: nil,
	}
	pb := service.ToTaxonomyTreeNode(n)
	if pb.GetNode() == nil || pb.GetNode().GetId() != "cat-1" {
		t.Fatalf("node mismatch: %+v", pb.GetNode())
	}
	if len(pb.GetChildren()) != 0 {
		t.Fatalf("expected no children, got %d", len(pb.GetChildren()))
	}
}

func TestToTaxonomyTreeNode_WithChildren(t *testing.T) {
	n := &biz.OrganizationTreeNode{
		Category: biz.OrganizationNode{
			ID:   "parent",
			Key:  "parent-key",
			Name: "Parent",
		},
		Children: []biz.OrganizationTreeNode{
			{
				Category: biz.OrganizationNode{
					ID:   "child-1",
					Key:  "child-key-1",
					Name: "Child 1",
				},
			},
			{
				Category: biz.OrganizationNode{
					ID:   "child-2",
					Key:  "child-key-2",
					Name: "Child 2",
				},
			},
		},
	}
	pb := service.ToTaxonomyTreeNode(n)
	if pb.GetNode().GetId() != "parent" {
		t.Fatalf("parent id mismatch: %q", pb.GetNode().GetId())
	}
	if len(pb.GetChildren()) != 2 {
		t.Fatalf("children count mismatch: %d", len(pb.GetChildren()))
	}
	if pb.GetChildren()[0].GetNode().GetId() != "child-1" {
		t.Fatalf("child-1 id mismatch: %q", pb.GetChildren()[0].GetNode().GetId())
	}
	if pb.GetChildren()[1].GetNode().GetId() != "child-2" {
		t.Fatalf("child-2 id mismatch: %q", pb.GetChildren()[1].GetNode().GetId())
	}
}

func TestToTaxonomyTreeNode_DeepNesting(t *testing.T) {
	n := &biz.OrganizationTreeNode{
		Category: biz.OrganizationNode{ID: "root", Key: "root", Name: "Root"},
		Children: []biz.OrganizationTreeNode{
			{
				Category: biz.OrganizationNode{ID: "level1", Key: "l1", Name: "L1"},
				Children: []biz.OrganizationTreeNode{
					{
						Category: biz.OrganizationNode{ID: "level2", Key: "l2", Name: "L2"},
					},
				},
			},
		},
	}
	pb := service.ToTaxonomyTreeNode(n)
	l1 := pb.GetChildren()[0]
	l2 := l1.GetChildren()[0]
	if l2.GetNode().GetId() != "level2" {
		t.Fatalf("deep nesting id mismatch: %q", l2.GetNode().GetId())
	}
}

func TestToTaxonomyTree_Empty(t *testing.T) {
	pb := service.ToTaxonomyTree(nil)
	if pb == nil {
		t.Fatal("expected non-nil slice")
	}
	if len(pb) != 0 {
		t.Fatalf("expected empty slice, got %d", len(pb))
	}
}

func TestToTaxonomyTree_MultipleRoots(t *testing.T) {
	nodes := []biz.OrganizationTreeNode{
		{Category: biz.OrganizationNode{ID: "r1", Key: "root1", Name: "Root 1"}},
		{Category: biz.OrganizationNode{ID: "r2", Key: "root2", Name: "Root 2"}},
	}
	pb := service.ToTaxonomyTree(nodes)
	if len(pb) != 2 {
		t.Fatalf("expected 2 roots, got %d", len(pb))
	}
	if pb[0].GetNode().GetId() != "r1" || pb[1].GetNode().GetId() != "r2" {
		t.Fatalf("root ids mismatch: r0=%q r1=%q", pb[0].GetNode().GetId(), pb[1].GetNode().GetId())
	}
}

func TestFromProtoTaxonomy_ZeroValues(t *testing.T) {
	pb := &taxv1.TaxonomyNode{
		Id:      "cat-0",
		Key:     "zero",
		Enabled: false,
	}
	bizCat := service.FromProtoTaxonomy(pb)
	if bizCat.ID != "cat-0" || bizCat.Key != "zero" {
		t.Fatalf("id/key mismatch")
	}
	if bizCat.Enabled {
		t.Fatal("expected enabled=false")
	}
	if bizCat.Name != "" || bizCat.Description != "" {
		t.Fatalf("expected empty name/desc: name=%q desc=%q", bizCat.Name, bizCat.Description)
	}
	if bizCat.SortOrder != 0 {
		t.Fatalf("expected sort_order=0, got %d", bizCat.SortOrder)
	}
}
