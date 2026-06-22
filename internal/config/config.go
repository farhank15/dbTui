package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/farhank15/dbTui/internal/model"
)

// Config manages application configuration
type Config struct {
	Connections []model.Connection `json:"connections"`
	Theme       string             `json:"theme"`
}

// ConfigManager handles reading/writing config
type ConfigManager struct {
	configPath string
	config     *Config
}

// NewConfigManager creates a new config manager
func NewConfigManager() (*ConfigManager, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cannot find home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, ".dbTui")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		// Fallback to temp directory
		configDir = os.TempDir()
	}

	configPath := filepath.Join(configDir, "config.json")
	cm := &ConfigManager{
		configPath: configPath,
		config: &Config{
			Connections: make([]model.Connection, 0),
			Theme:       "default",
		},
	}

	if err := cm.load(); err != nil {
		if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Warning: Could not load config: %v\n", err)
		}
	}

	return cm, nil
}

// GetConnections returns all saved connections
func (cm *ConfigManager) GetConnections() []model.Connection {
	return cm.config.Connections
}

// GenerateUniqueID returns a unique connection ID that doesn't collide with existing connections
func (cm *ConfigManager) GenerateUniqueID() string {
	maxVal := 0
	for _, conn := range cm.config.Connections {
		var val int
		if _, err := fmt.Sscanf(conn.ID, "conn_%d", &val); err == nil {
			if val > maxVal {
				maxVal = val
			}
		}
	}
	return fmt.Sprintf("conn_%d", maxVal+1)
}

// AddConnection adds a connection
func (cm *ConfigManager) AddConnection(conn model.Connection) error {
	cm.config.Connections = append(cm.config.Connections, conn)
	return cm.save()
}

// UpdateConnection updates a connection by ID
func (cm *ConfigManager) UpdateConnection(id string, conn model.Connection) error {
	for i, c := range cm.config.Connections {
		if c.ID == id {
			cm.config.Connections[i] = conn
			return cm.save()
		}
	}
	return fmt.Errorf("connection not found: %s", id)
}

// DeleteConnection deletes a connection by ID
func (cm *ConfigManager) DeleteConnection(id string) error {
	for i, c := range cm.config.Connections {
		if c.ID == id {
			cm.config.Connections = append(cm.config.Connections[:i], cm.config.Connections[i+1:]...)
			return cm.save()
		}
	}
	return fmt.Errorf("connection not found: %s", id)
}

// GetConnectionByID returns a connection by ID
func (cm *ConfigManager) GetConnectionByID(id string) *model.Connection {
	for _, c := range cm.config.Connections {
		if c.ID == id {
			return &c
		}
	}
	return nil
}

func (cm *ConfigManager) load() error {
	data, err := os.ReadFile(cm.configPath)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, cm.config); err != nil {
		return err
	}

	// Self-healing: identify and fix duplicate or empty IDs
	seen := make(map[string]bool)
	changed := false
	for i := range cm.config.Connections {
		conn := &cm.config.Connections[i]
		if conn.ID == "" || seen[conn.ID] {
			// Find a unique ID starting from 1
			idNum := 1
			var newID string
			for {
				candidate := fmt.Sprintf("conn_%d", idNum)
				exists := seen[candidate]
				if !exists {
					for _, c := range cm.config.Connections {
						if c.ID == candidate {
							exists = true
							break
						}
					}
				}
				if !exists {
					newID = candidate
					break
				}
				idNum++
			}
			conn.ID = newID
			changed = true
		}
		seen[conn.ID] = true
	}

	if changed {
		if err := cm.save(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to save repaired config: %v\n", err)
		}
	}

	return nil
}

func (cm *ConfigManager) save() error {
	data, err := json.MarshalIndent(cm.config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cm.configPath, data, 0644)
}
