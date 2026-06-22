package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/farhank15/dbTui/internal/model"
)

func TestConfigLoadSelfHealing(t *testing.T) {
	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "dbtui_config_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.json")

	// Set up duplicate & empty IDs
	rawConfig := Config{
		Theme: "default",
		Connections: []model.Connection{
			{ID: "conn_2", Name: "Postgres 1", Type: model.TypePostgres},
			{ID: "conn_2", Name: "Postgres 2", Type: model.TypePostgres},
			{ID: "", Name: "MySQL 1", Type: model.TypeMySQL},
			{ID: "conn_1", Name: "SQLite 1", Type: model.TypeSQLite},
		},
	}

	data, err := json.Marshal(rawConfig)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Initialize manager pointing to this temp config path
	cm := &ConfigManager{
		configPath: configPath,
		config: &Config{
			Connections: make([]model.Connection, 0),
			Theme:       "default",
		},
	}

	if err := cm.load(); err != nil {
		t.Fatalf("load failed: %v", err)
	}

	// Assert duplicate/empty IDs are fixed
	seenIDs := make(map[string]bool)
	for i, conn := range cm.config.Connections {
		if conn.ID == "" {
			t.Errorf("Connection at index %d has empty ID", i)
		}
		if seenIDs[conn.ID] {
			t.Errorf("Duplicate ID found: %s", conn.ID)
		}
		seenIDs[conn.ID] = true
	}

	// Re-load to make sure it was successfully saved and is now clean
	cm2 := &ConfigManager{
		configPath: configPath,
		config: &Config{
			Connections: make([]model.Connection, 0),
			Theme:       "default",
		},
	}
	if err := cm2.load(); err != nil {
		t.Fatalf("second load failed: %v", err)
	}

	seenIDs2 := make(map[string]bool)
	for i, conn := range cm2.config.Connections {
		if conn.ID == "" {
			t.Errorf("Connection at index %d has empty ID after reload", i)
		}
		if seenIDs2[conn.ID] {
			t.Errorf("Duplicate ID found after reload: %s", conn.ID)
		}
		seenIDs2[conn.ID] = true
	}
}
