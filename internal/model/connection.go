package model

import (
	"fmt"
	"strings"
)

// ConnectionType represents the database type
type ConnectionType string

const (
	TypePostgres ConnectionType = "postgres"
	TypeMySQL    ConnectionType = "mysql"
	TypeSQLite   ConnectionType = "sqlite"
)

// Connection represents a saved database connection
type Connection struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Type     ConnectionType `json:"type"`
	Host     string         `json:"host"`
	Port     int            `json:"port"`
	User     string         `json:"user"`
	Password string         `json:"password"`
	Database string         `json:"database"`
	SSLMode  string         `json:"ssl_mode"`
	File     string         `json:"file"` // for SQLite
}

// pgxQuoteValue wraps a value in single quotes for pgx/libpq keyword/value format.
// This prevents special characters (spaces, =, $, etc.) from breaking the DSN parser.
func pgxQuoteValue(v string) string {
	// Escape single quotes by doubling them, then wrap in single quotes
	escaped := strings.ReplaceAll(v, "'", "''")
	return "'" + escaped + "'"
}

// DSN returns the connection string
func (c *Connection) DSN() string {
	switch c.Type {
	case TypePostgres:
		ssl := c.SSLMode
		if ssl == "" {
			ssl = "disable"
		}
		// Quote each value to handle special characters (spaces, =, $, etc.)
		return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
			pgxQuoteValue(c.Host), c.Port, pgxQuoteValue(c.User),
			pgxQuoteValue(c.Password), pgxQuoteValue(c.Database), pgxQuoteValue(ssl))
	case TypeMySQL:
		return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True",
			c.User, c.Password, c.Host, c.Port, c.Database)
	case TypeSQLite:
		return c.File
	}
	return ""
}

// DisplayDSN returns a masked DSN for display
func (c *Connection) DisplayDSN() string {
	switch c.Type {
	case TypePostgres:
		return fmt.Sprintf("postgres://%s@%s:%d/%s", c.User, c.Host, c.Port, c.Database)
	case TypeMySQL:
		return fmt.Sprintf("mysql://%s@%s:%d/%s", c.User, c.Host, c.Port, c.Database)
	case TypeSQLite:
		return fmt.Sprintf("sqlite://%s", c.File)
	}
	return ""
}
