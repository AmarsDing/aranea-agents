package biz

type AgentAvatarSpec struct {
	AssetKey  string
	Name      string
	SortOrder int
}

func AgentAvatarSpecs() []AgentAvatarSpec {
	return []AgentAvatarSpec{
		{"avatar_career_01", "职场 1", 100},
		{"avatar_career_02", "职场 2", 110},
		{"avatar_career_03", "职场 3", 120},
		{"avatar_career_04", "职场 4", 130},
		{"avatar_career_05", "职场 5", 140},
		{"avatar_career_06", "职场 6", 150},
		{"avatar_career_07", "职场 7", 160},
		{"avatar_career_08", "职场 8", 170},
		{"avatar_career_09", "职场 9", 180},
		{"avatar_career_10", "职场 10", 190},
		{"avatar_career_11", "职场 11", 200},
		{"avatar_toon_01", "卡通 1", 300},
		{"avatar_toon_02", "卡通 2", 310},
		{"avatar_toon_03", "卡通 3", 320},
		{"avatar_toon_04", "卡通 4", 330},
		{"avatar_toon_05", "卡通 5", 340},
		{"avatar_vivid_01", "个性 1", 500},
		{"avatar_vivid_02", "个性 2", 510},
		{"avatar_vivid_03", "个性 3", 520},
		{"avatar_vivid_04", "个性 4", 530},
		{"avatar_vivid_05", "个性 5", 540},
	}
}
