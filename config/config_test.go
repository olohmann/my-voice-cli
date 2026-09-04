package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Model != "gpt-5.6-luna" {
		t.Fatalf("Model = %q, want gpt-5.6-luna", cfg.Model)
	}
	if cfg.ReasoningEffort != "low" {
		t.Fatalf("ReasoningEffort = %q, want low", cfg.ReasoningEffort)
	}
}

func TestLoadConfigFillsReasoningEffort(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("model = \"custom-model\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "custom-model" {
		t.Fatalf("Model = %q, want custom-model", cfg.Model)
	}
	if cfg.ReasoningEffort != "low" {
		t.Fatalf("ReasoningEffort = %q, want low", cfg.ReasoningEffort)
	}
}

func TestValidateReasoningEffort(t *testing.T) {
	value, err := ValidateReasoningEffort(" LOW ")
	if err != nil {
		t.Fatal(err)
	}
	if value != "low" {
		t.Fatalf("value = %q, want low", value)
	}
	if _, err := ValidateReasoningEffort("fast"); err == nil {
		t.Fatal("expected invalid reasoning effort error")
	}
}
