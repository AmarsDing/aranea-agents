package conf

import (
	"os"
	"testing"
)

func TestPGODefaultFilesV2_Default(t *testing.T) {
	os.Unsetenv("PGO_DEFAULT_FILES_V2")
	if !PGODefaultFilesV2() {
		t.Fatal("should default to true")
	}
}

func TestPGODefaultFilesV2_Enabled(t *testing.T) {
	os.Setenv("PGO_DEFAULT_FILES_V2", "1")
	defer os.Unsetenv("PGO_DEFAULT_FILES_V2")
	if !PGODefaultFilesV2() {
		t.Fatal("should be true when set to 1")
	}
}

func TestPGODefaultFilesV2_True(t *testing.T) {
	os.Setenv("PGO_DEFAULT_FILES_V2", "true")
	defer os.Unsetenv("PGO_DEFAULT_FILES_V2")
	if !PGODefaultFilesV2() {
		t.Fatal("should be true when set to true")
	}
}

func TestPGODefaultFilesV2_Disabled(t *testing.T) {
	os.Setenv("PGO_DEFAULT_FILES_V2", "0")
	defer os.Unsetenv("PGO_DEFAULT_FILES_V2")
	if PGODefaultFilesV2() {
		t.Fatal("should be false when set to 0")
	}
}

func TestPGOCategoryResponsibilityInject_Default(t *testing.T) {
	os.Unsetenv("PGO_CATEGORY_RESPONSIBILITY_INJECT")
	if PGOCategoryResponsibilityInject() {
		t.Fatal("should default to false")
	}
}

func TestPGOCategoryResponsibilityInject_Enabled(t *testing.T) {
	os.Setenv("PGO_CATEGORY_RESPONSIBILITY_INJECT", "yes")
	defer os.Unsetenv("PGO_CATEGORY_RESPONSIBILITY_INJECT")
	if !PGOCategoryResponsibilityInject() {
		t.Fatal("should be true when set to yes")
	}
}

func TestPGOAIRefineV2_Default(t *testing.T) {
	os.Unsetenv("PGO_AI_REFINE_V2")
	if PGOAIRefineV2() {
		t.Fatal("should default to false")
	}
}

func TestPGOAIRefineV2_Enabled(t *testing.T) {
	os.Setenv("PGO_AI_REFINE_V2", "1")
	defer os.Unsetenv("PGO_AI_REFINE_V2")
	if !PGOAIRefineV2() {
		t.Fatal("should be true when set to 1")
	}
}

func TestPGOCLIImportEnabled_Default(t *testing.T) {
	os.Unsetenv("PGO_CLI_IMPORT_ENABLED")
	if !PGOCLIImportEnabled() {
		t.Fatal("should default to true")
	}
}

func TestPGOCLIImportEnabled_Disabled(t *testing.T) {
	os.Setenv("PGO_CLI_IMPORT_ENABLED", "0")
	defer os.Unsetenv("PGO_CLI_IMPORT_ENABLED")
	if PGOCLIImportEnabled() {
		t.Fatal("should be false when set to 0")
	}
}

func TestPGOCLIImportEnabled_False(t *testing.T) {
	os.Setenv("PGO_CLI_IMPORT_ENABLED", "false")
	defer os.Unsetenv("PGO_CLI_IMPORT_ENABLED")
	if PGOCLIImportEnabled() {
		t.Fatal("should be false when set to false")
	}
}

func TestBLOUnifiedJobEnabled_Default(t *testing.T) {
	os.Unsetenv("BLO_UNIFIED_JOB_ENABLED")
	if BLOUnifiedJobEnabled() {
		t.Fatal("should default to false")
	}
}

func TestBLOUnifiedJobEnabled_Enabled(t *testing.T) {
	os.Setenv("BLO_UNIFIED_JOB_ENABLED", "true")
	defer os.Unsetenv("BLO_UNIFIED_JOB_ENABLED")
	if !BLOUnifiedJobEnabled() {
		t.Fatal("should be true when set to true")
	}
}

func TestBLOPendingTaskV2_Default(t *testing.T) {
	os.Unsetenv("BLO_PENDING_TASK_V2")
	if BLOPendingTaskV2() {
		t.Fatal("should default to false")
	}
}

func TestBLOEscalationV2_Default(t *testing.T) {
	os.Unsetenv("BLO_ESCALATION_V2")
	if BLOEscalationV2() {
		t.Fatal("should default to false")
	}
}

func TestBLOIntentClassifier_Default(t *testing.T) {
	os.Unsetenv("BLO_INTENT_CLASSIFIER")
	if BLOIntentClassifier() {
		t.Fatal("should default to false")
	}
}

func TestBLOTriggerRules_Default(t *testing.T) {
	os.Unsetenv("BLO_TRIGGER_RULES")
	if BLOTriggerRules() {
		t.Fatal("should default to false")
	}
}

func TestParseBoolFlag_CaseInsensitive(t *testing.T) {
	os.Setenv("BLO_UNIFIED_JOB_ENABLED", "TRUE")
	defer os.Unsetenv("BLO_UNIFIED_JOB_ENABLED")
	if !BLOUnifiedJobEnabled() {
		t.Fatal("should be case-insensitive")
	}
}

func TestParseBoolFlag_InvalidValue(t *testing.T) {
	os.Setenv("BLO_UNIFIED_JOB_ENABLED", "maybe")
	defer os.Unsetenv("BLO_UNIFIED_JOB_ENABLED")
	if BLOUnifiedJobEnabled() {
		t.Fatal("invalid value should be false")
	}
}

func TestServer_ProcessLogEnabled_NilServer(t *testing.T) {
	var s *Server
	if !s.ProcessLogEnabled() {
		t.Fatal("nil server should default to enabled")
	}
}
