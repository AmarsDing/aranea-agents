package service_test

import (
	"testing"

	avatarv1 "aranea-agents/api/kratos/avatar/v1"
	ecosystemv1 "aranea-agents/api/kratos/ecosystem/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/service"
)

func TestToProtoAvatar(t *testing.T) {
	tests := []struct {
		name  string
		input *biz.AvatarAsset
		want  *avatarv1.AvatarAsset
	}{
		{
			name: "full_asset",
			input: &biz.AvatarAsset{
				ID:            "av-1",
				Key:           "avatar_career_01",
				Name:          "职场 1",
				Description:   "Built-in agent avatar",
				MimeType:      "image/png",
				WorkspaceID:   "ws-1",
				OwnerUserID:   "user-1",
				Source:        "system",
				Category:      "agent",
				IsSystem:      true,
				FileSizeBytes: 4096,
				WidthPx:       256,
				HeightPx:      256,
				SortOrder:     100,
				CreatedAt:     "2025-01-01T00:00:00Z",
			},
			want: &avatarv1.AvatarAsset{
				Id:            "av-1",
				Key:           "avatar_career_01",
				Name:          "职场 1",
				Description:   "Built-in agent avatar",
				MimeType:      "image/png",
				WorkspaceId:   "ws-1",
				OwnerUserId:   "user-1",
				Source:        "system",
				Category:      "agent",
				IsSystem:      true,
				FileSizeBytes: 4096,
				WidthPx:       256,
				HeightPx:      256,
				SortOrder:     100,
				CreatedAt:     "2025-01-01T00:00:00Z",
			},
		},
		{
			name:  "nil_input",
			input: nil,
			want:  nil,
		},
		{
			name: "minimal_asset",
			input: &biz.AvatarAsset{
				ID:   "av-2",
				Key:  "test",
				Name: "Test",
			},
			want: &avatarv1.AvatarAsset{
				Id:   "av-2",
				Key:  "test",
				Name: "Test",
			},
		},
		{
			name: "channel_platform_asset",
			input: &biz.AvatarAsset{
				ID:            "channel_feishu",
				Key:           "channel_feishu",
				Name:          "飞书",
				Category:      "channel",
				IsSystem:      true,
				FileSizeBytes: 2048,
				WidthPx:       128,
				HeightPx:      128,
				SortOrder:     10,
			},
			want: &avatarv1.AvatarAsset{
				Id:            "channel_feishu",
				Key:           "channel_feishu",
				Name:          "飞书",
				Category:      "channel",
				IsSystem:      true,
				FileSizeBytes: 2048,
				WidthPx:       128,
				HeightPx:      128,
				SortOrder:     10,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.ToProtoAvatar(tt.input)
			if tt.input == nil {
				if got != nil {
					t.Errorf("got = %v, want nil", got)
				}
				return
			}
			if got.Id != tt.want.Id {
				t.Errorf("Id = %q, want %q", got.Id, tt.want.Id)
			}
			if got.Key != tt.want.Key {
				t.Errorf("Key = %q, want %q", got.Key, tt.want.Key)
			}
			if got.Name != tt.want.Name {
				t.Errorf("Name = %q, want %q", got.Name, tt.want.Name)
			}
			if got.Description != tt.want.Description {
				t.Errorf("Description = %q, want %q", got.Description, tt.want.Description)
			}
			if got.MimeType != tt.want.MimeType {
				t.Errorf("MimeType = %q, want %q", got.MimeType, tt.want.MimeType)
			}
			if got.WorkspaceId != tt.want.WorkspaceId {
				t.Errorf("WorkspaceId = %q, want %q", got.WorkspaceId, tt.want.WorkspaceId)
			}
			if got.OwnerUserId != tt.want.OwnerUserId {
				t.Errorf("OwnerUserId = %q, want %q", got.OwnerUserId, tt.want.OwnerUserId)
			}
			if got.Source != tt.want.Source {
				t.Errorf("Source = %q, want %q", got.Source, tt.want.Source)
			}
			if got.Category != tt.want.Category {
				t.Errorf("Category = %q, want %q", got.Category, tt.want.Category)
			}
			if got.IsSystem != tt.want.IsSystem {
				t.Errorf("IsSystem = %v, want %v", got.IsSystem, tt.want.IsSystem)
			}
			if got.FileSizeBytes != tt.want.FileSizeBytes {
				t.Errorf("FileSizeBytes = %d, want %d", got.FileSizeBytes, tt.want.FileSizeBytes)
			}
			if got.WidthPx != tt.want.WidthPx {
				t.Errorf("WidthPx = %d, want %d", got.WidthPx, tt.want.WidthPx)
			}
			if got.HeightPx != tt.want.HeightPx {
				t.Errorf("HeightPx = %d, want %d", got.HeightPx, tt.want.HeightPx)
			}
			if got.SortOrder != tt.want.SortOrder {
				t.Errorf("SortOrder = %d, want %d", got.SortOrder, tt.want.SortOrder)
			}
			if got.CreatedAt != tt.want.CreatedAt {
				t.Errorf("CreatedAt = %q, want %q", got.CreatedAt, tt.want.CreatedAt)
			}
		})
	}
}

func TestProductToProto(t *testing.T) {
	tests := []struct {
		name string
		p    biz.EcosystemProduct
		want *ecosystemv1.Product
	}{
		{
			name: "full_product",
			p: biz.EcosystemProduct{
				ID:           "eco-1",
				Name:         "skill-pack-1",
				DisplayName:  "Skill Pack 1",
				Description:  "A useful skill pack",
				Type:         "skill_pack",
				AuthorID:     "author-1",
				Version:      "1.0.0",
				PriceModel:   "free",
				PriceCents:   0,
				Rating:       4.5,
				InstallCount: 100,
				ConfigJSON:   `{"tools":["search"]}`,
				Status:       "published",
				CreatedAt:    "2025-01-01T00:00:00Z",
				UpdatedAt:    "2025-01-02T00:00:00Z",
				Installed:    true,
			},
			want: &ecosystemv1.Product{
				Id:           "eco-1",
				Name:         "skill-pack-1",
				DisplayName:  "Skill Pack 1",
				Description:  "A useful skill pack",
				Type:         "skill_pack",
				AuthorId:     "author-1",
				Version:      "1.0.0",
				PriceModel:   "free",
				PriceCents:   0,
				Rating:       4.5,
				InstallCount: 100,
				ConfigJson:   `{"tools":["search"]}`,
				Status:       "published",
				CreatedAt:    "2025-01-01T00:00:00Z",
				UpdatedAt:    "2025-01-02T00:00:00Z",
				Installed:    true,
			},
		},
		{
			name: "minimal_product",
			p: biz.EcosystemProduct{
				ID:   "eco-2",
				Name: "minimal",
			},
			want: &ecosystemv1.Product{
				Id:   "eco-2",
				Name: "minimal",
			},
		},
		{
			name: "paid_product",
			p: biz.EcosystemProduct{
				ID:           "eco-3",
				Name:         "pro-pack",
				DisplayName:  "Pro Pack",
				PriceModel:   "paid",
				PriceCents:   999,
				Rating:       3.8,
				InstallCount: 42,
				Installed:    false,
			},
			want: &ecosystemv1.Product{
				Id:           "eco-3",
				Name:         "pro-pack",
				DisplayName:  "Pro Pack",
				PriceModel:   "paid",
				PriceCents:   999,
				Rating:       3.8,
				InstallCount: 42,
				Installed:    false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.ProductToProto(tt.p)
			if got.Id != tt.want.Id {
				t.Errorf("Id = %q, want %q", got.Id, tt.want.Id)
			}
			if got.Name != tt.want.Name {
				t.Errorf("Name = %q, want %q", got.Name, tt.want.Name)
			}
			if got.DisplayName != tt.want.DisplayName {
				t.Errorf("DisplayName = %q, want %q", got.DisplayName, tt.want.DisplayName)
			}
			if got.Description != tt.want.Description {
				t.Errorf("Description = %q, want %q", got.Description, tt.want.Description)
			}
			if got.Type != tt.want.Type {
				t.Errorf("Type = %q, want %q", got.Type, tt.want.Type)
			}
			if got.AuthorId != tt.want.AuthorId {
				t.Errorf("AuthorId = %q, want %q", got.AuthorId, tt.want.AuthorId)
			}
			if got.Version != tt.want.Version {
				t.Errorf("Version = %q, want %q", got.Version, tt.want.Version)
			}
			if got.PriceModel != tt.want.PriceModel {
				t.Errorf("PriceModel = %q, want %q", got.PriceModel, tt.want.PriceModel)
			}
			if got.PriceCents != tt.want.PriceCents {
				t.Errorf("PriceCents = %d, want %d", got.PriceCents, tt.want.PriceCents)
			}
			if got.Rating != tt.want.Rating {
				t.Errorf("Rating = %f, want %f", got.Rating, tt.want.Rating)
			}
			if got.InstallCount != tt.want.InstallCount {
				t.Errorf("InstallCount = %d, want %d", got.InstallCount, tt.want.InstallCount)
			}
			if got.ConfigJson != tt.want.ConfigJson {
				t.Errorf("ConfigJson = %q, want %q", got.ConfigJson, tt.want.ConfigJson)
			}
			if got.Status != tt.want.Status {
				t.Errorf("Status = %q, want %q", got.Status, tt.want.Status)
			}
			if got.CreatedAt != tt.want.CreatedAt {
				t.Errorf("CreatedAt = %q, want %q", got.CreatedAt, tt.want.CreatedAt)
			}
			if got.UpdatedAt != tt.want.UpdatedAt {
				t.Errorf("UpdatedAt = %q, want %q", got.UpdatedAt, tt.want.UpdatedAt)
			}
			if got.Installed != tt.want.Installed {
				t.Errorf("Installed = %v, want %v", got.Installed, tt.want.Installed)
			}
		})
	}
}
