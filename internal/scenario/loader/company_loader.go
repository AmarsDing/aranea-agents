package loader

import (
	"fmt"
	"os"
	"path/filepath"
)

// LoadCompanySpec 解析 {scenarioDir}/{companyKey}/agents.yaml 并填充默认值。
// 这是行业 / 场景数据的唯一入口：调用方（data.SeedPackIndustry）拿到
// CompanySpec 后再由 data.ConvertCompanySpecToPack 统一转换为 Pack 模型，
// 走 pack.Importer 导入。所有"loader → biz 直接写库"的旧路径已废弃并删除。
func LoadCompanySpec(scenarioDir, companyKey string) (*CompanySpec, error) {
	path := filepath.Join(scenarioDir, companyKey, "agents.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var spec CompanySpec
	if err := yamlUnmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	spec.CompanyKey = companyKey
	fillDefaults(&spec)
	return &spec, nil
}
