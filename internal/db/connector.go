package db

import (
	"database/sql"

	"github.com/farhank15/dbTui/internal/model"
)

// Connector defines the interface for database operations
type Connector interface {
	// Connect establishes a connection
	Connect(config model.Connection) error
	// Close closes the connection
	Close() error
	// Ping checks if the connection is alive
	Ping() error
	// IsConnected returns true if connected
	IsConnected() bool

	// GetDatabases returns list of databases
	GetDatabases() ([]string, error)
	// GetTables returns tables in a database
	GetTables(dbName string) ([]model.TableInfo, error)
	// GetColumns returns columns of a table
	GetColumns(dbName, tableName string) ([]model.ColumnInfo, error)
	// GetIndexes returns indexes of a table
	GetIndexes(dbName, tableName string) ([]model.IndexInfo, error)
	// GetForeignKeys returns foreign keys of a table
	GetForeignKeys(dbName, tableName string) ([]model.ForeignKeyInfo, error)
	// GetTableDetail returns full table details
	GetTableDetail(dbName, tableName string) (*model.TableDetail, error)

	// ExecuteQuery executes a SQL query and returns results
	ExecuteQuery(query string) (*model.QueryResult, error)
	// ExecuteQueryWithDB executes a query on a specific database
	ExecuteQueryWithDB(dbName, query string) (*model.QueryResult, error)

	// CreateDatabase creates a new database
	CreateDatabase(name string) error
	// DropDatabase drops a database
	DropDatabase(name string) error
	// CreateTable creates a table with given columns
	CreateTable(dbName string, tableName string, columns []model.ColumnDef) error
	// DropTable drops a table
	DropTable(dbName, tableName string) error
	// TruncateTable truncates a table
	TruncateTable(dbName, tableName string) error
	// RenameTable renames a table
	RenameTable(dbName, oldName, newName string) error

	// AddColumn adds a column to a table
	AddColumn(dbName, tableName string, col model.ColumnDef) error
	// DropColumn drops a column from a table
	DropColumn(dbName, tableName, colName string) error
	// ModifyColumn modifies a column
	ModifyColumn(dbName, tableName string, oldName string, col model.ColumnDef) error

	// GetRowCount returns the number of rows in a table
	GetRowCount(dbName, tableName string) (int64, error)

	// GetDB returns the underlying *sql.DB for custom operations
	GetDB() *sql.DB
}
