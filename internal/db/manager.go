package db

import (
	"fmt"
	"sync"

	"github.com/farhank15/dbTui/internal/model"
)

// ConnectionState represents an active connection
type ConnectionState struct {
	Connection model.Connection
	Connector  Connector
	Connected  bool
	Databases  []string
}

// Manager manages multiple database connections
type Manager struct {
	mu          sync.RWMutex
	connections map[string]*ConnectionState
}

// NewManager creates a new connection manager
func NewManager() *Manager {
	return &Manager{
		connections: make(map[string]*ConnectionState),
	}
}

// Connect establishes a new connection
func (m *Manager) Connect(config model.Connection) error {
	var conn Connector

	switch config.Type {
	case model.TypePostgres:
		conn = NewPostgresConnector()
	case model.TypeMySQL:
		conn = NewMySQLConnector()
	case model.TypeSQLite:
		conn = NewSQLiteConnector()
	default:
		return fmt.Errorf("unsupported database type: %s", config.Type)
	}

	if err := conn.Connect(config); err != nil {
		return fmt.Errorf("failed to connect to %s: %w", config.Type, err)
	}

	state := &ConnectionState{
		Connection: config,
		Connector:  conn,
		Connected:  true,
	}

	m.mu.Lock()
	m.connections[config.ID] = state
	m.mu.Unlock()

	return nil
}

// Disconnect closes a connection
func (m *Manager) Disconnect(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.connections[id]
	if !ok {
		return fmt.Errorf("connection not found: %s", id)
	}

	if err := state.Connector.Close(); err != nil {
		return err
	}

	state.Connected = false
	delete(m.connections, id)
	return nil
}

// GetConnector returns the connector for a connection
func (m *Manager) GetConnector(id string) (Connector, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, ok := m.connections[id]
	if !ok {
		return nil, fmt.Errorf("connection not found: %s", id)
	}
	if !state.Connected {
		return nil, fmt.Errorf("connection is not active: %s", id)
	}

	return state.Connector, nil
}

// GetConnectionState returns the connection state
func (m *Manager) GetConnectionState(id string) *ConnectionState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connections[id]
}

// GetActiveConnections returns all active connections
func (m *Manager) GetActiveConnections() []*ConnectionState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*ConnectionState
	for _, state := range m.connections {
		result = append(result, state)
	}
	return result
}

// IsConnected checks if a connection is active
func (m *Manager) IsConnected(id string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, ok := m.connections[id]
	return ok && state.Connected
}

// RefreshDatabases refreshes the database list for a connection
func (m *Manager) RefreshDatabases(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.connections[id]
	if !ok {
		return fmt.Errorf("connection not found: %s", id)
	}

	databases, err := state.Connector.GetDatabases()
	if err != nil {
		return err
	}

	state.Databases = databases
	return nil
}
