export interface VariantInfo {
  key: string;
  label: string;
}

export interface Industry {
  id: string;
  key: string;
  name: string;
  icon: string;
  description: string;
  scenario_key: string;
  enabled: boolean;
  sort_order: number;
}

export interface PositionPromptResult {
  promptContent: string;
  variant: string;
  positionName: string;
  departmentName: string;
  industryName: string;
  industryDescription: string;
  departmentDescription: string;
  positionDescription: string;
  responsibilitiesJson: string;
  variantDescription: string;
}

export interface Department {
  id: string;
  key: string;
  name: string;
  industry_key: string;
  description: string;
  responsibilities_json: string;
  sort_order: number;
}

export interface Position {
  id: string;
  key: string;
  name: string;
  department_key: string;
  description: string;
  responsibilities_json: string;
  skills_required_json: string;
  seniority_level: string;
  sort_order: number;
}
