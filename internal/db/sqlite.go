package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/farhank15/dbTui/internal/model"
)

// SQLiteConnector implements Connector for SQLite
type SQLiteConnector struct {
	db  *sql.DB
	cfg model.Connection
}

// NewSQLiteConnector creates a new SQLite connector
func NewSQLiteConnector() *SQLiteConnector {
	return &SQLiteConnector{}
}

func (s *SQLiteConnector) Connect(config model.Connection) error {
	dsn := config.DSN()
	if dsn == "" {
		return fmt.Errorf("sqlite file path is required")
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("failed to open sqlite connection: %w", err)
	}

	db.SetMaxOpenConns(1) // SQLite hanya support 1 writer
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	if err := db.Ping(); err != nil {
		db.Close()
		return fmt.Errorf("failed to ping sqlite: %w", err)
	}

	// Enable WAL mode for better concurrent performance
	db.Exec("PRAGMA journal_mode=WAL")
	db.Exec("PRAGMA foreign_keys=ON")

	s.db = db
	s.cfg = config
	return nil
}

func (s *SQLiteConnector) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

func (s *SQLiteConnector) Ping() error {
	if s.db == nil {
		return fmt.Errorf("not connected")
	}
	return s.db.Ping()
}

func (s *SQLiteConnector) IsConnected() bool {
	return s.db != nil && s.db.Ping() == nil
}

func (s *SQLiteConnector) GetDB() *sql.DB {
	return s.db
}

func (s *SQLiteConnector) GetDatabases() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// SQLite file = 1 database. Tapi kita bisa list attached databases
	rows, err := s.db.QueryContext(ctx, "PRAGMA database_list")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var databases []string
	for rows.Next() {
		var seq int
		var name, file string
		if err := rows.Scan(&seq, &name, &file); err != nil {
			return nil, err
		}
		databases = append(databases, name)
	}
	return databases, nil
}

func (s *SQLiteConnector) GetTables(dbName string) ([]model.TableInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var query string
	if dbName != "" && dbName != "main" {
		query = fmt.Sprintf("SELECT name FROM %s.sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%%' ORDER BY name", quoteIdent(dbName))
	} else {
		query = "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name"
	}

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []model.TableInfo
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, model.TableInfo{Name: name})
	}
	return tables, nil
}

func (s *SQLiteConnector) GetColumns(dbName, tableName string) ([]model.ColumnInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := fmt.Sprintf("PRAGMA table_info(%s)", quoteIdent(tableName))
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []model.ColumnInfo
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var defaultVal sql.NullString
		var pk int

		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultVal, &pk); err != nil {
			return nil, err
		}

		nullable := "YES"
		if notNull == 1 {
			nullable = "NO"
		}

		key := ""
		if pk == 1 {
			key = "PRI"
		}

		def := ""
		if defaultVal.Valid {
			def = defaultVal.String
		}

		extra := ""
		// Check autoincrement
		autoIncCtx, autoIncCancel := context.WithTimeout(context.Background(), 5*time.Second)
		var sqliteSchema string
		err := s.db.QueryRowContext(autoIncCtx, fmt.Sprintf("SELECT sql FROM sqlite_master WHERE type='table' AND name='%s'", strings.ReplaceAll(tableName, "'", "''"))).Scan(&sqliteSchema)
		autoIncCancel()
		if err == nil && strings.Contains(strings.ToUpper(sqliteSchema), "AUTOINCREMENT") && pk == 1 {
			extra = "auto_increment"
		}

		columns = append(columns, model.ColumnInfo{
			Name:     name,
			Type:     colType,
			Nullable: nullable,
			Key:      key,
			Default:  def,
			Extra:    extra,
		})
	}
	return columns, nil
}

func (s *SQLiteConnector) GetIndexes(dbName, tableName string) ([]model.IndexInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf("PRAGMA index_list(%s)", quoteIdent(tableName)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var indexes []model.IndexInfo
	for rows.Next() {
		var seq int
		var name, uniqueStr string
		var origin, partial int

		if err := rows.Scan(&seq, &name, &uniqueStr, &origin, &partial); err != nil {
			return nil, err
		}

		isUnique := uniqueStr == "1"
		isPrimary := strings.HasPrefix(name, "sqlite_autoindex_") && isUnique

		// Get columns for this index
		idxInfoCtx, idxInfoCancel := context.WithTimeout(context.Background(), 5*time.Second)
		colRows, err := s.db.QueryContext(idxInfoCtx, fmt.Sprintf("PRAGMA index_info(%s)", quoteIdent(name)))
		if err != nil {
			idxInfoCancel()
			continue
		}

		var columns []string
		for colRows.Next() {
			var cid, seqno int
			var colName string
			if err := colRows.Scan(&cid, &seqno, &colName); err != nil {
				continue
			}
			columns = append(columns, colName)
		}
		colRows.Close()
		idxInfoCancel()

		indexes = append(indexes, model.IndexInfo{
			Name:    name,
			Columns: columns,
			Unique:  isUnique,
			Primary: isPrimary,
		})
	}
	return indexes, nil
}

func (s *SQLiteConnector) GetForeignKeys(dbName, tableName string) ([]model.ForeignKeyInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf("PRAGMA foreign_key_list(%s)", quoteIdent(tableName)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var fks []model.ForeignKeyInfo
	for rows.Next() {
		var id, seq int
		var table, from, to string
		var onUpdate, onDelete string
		var match sql.NullString

		if err := rows.Scan(&id, &seq, &table, &from, &to, &onUpdate, &onDelete, &match); err != nil {
			return nil, err
		}

		fks = append(fks, model.ForeignKeyInfo{
			Name:      fmt.Sprintf("fk_%s_%s", tableName, from),
			Column:    from,
			RefTable:  table,
			RefColumn: to,
			OnDelete:  onDelete,
			OnUpdate:  onUpdate,
		})
	}
	return fks, nil
}

func (s *SQLiteConnector) GetTableDetail(dbName, tableName string) (*model.TableDetail, error) {
	columns, err := s.GetColumns(dbName, tableName)
	if err != nil {
		return nil, err
	}

	indexes, err := s.GetIndexes(dbName, tableName)
	if err != nil {
		return nil, err
	}

	fks, err := s.GetForeignKeys(dbName, tableName)
	if err != nil {
		return nil, err
	}

	rowCount, err := s.GetRowCount(dbName, tableName)
	if err != nil {
		rowCount = 0
	}

	return &model.TableDetail{
		Table: model.TableInfo{
			Name:     tableName,
			Columns:  columns,
			RowCount: rowCount,
		},
		Indexes:     indexes,
		ForeignKeys: fks,
	}, nil
}

func (s *SQLiteConnector) ExecuteQuery(query string) (*model.QueryResult, error) {
	start := time.Now()
	trimmed := strings.TrimSpace(strings.ToUpper(query))
	isSelect := strings.HasPrefix(trimmed, "SELECT") ||
		strings.HasPrefix(trimmed, "WITH") ||
		strings.HasPrefix(trimmed, "PRAGMA") ||
		strings.HasPrefix(trimmed, "EXPLAIN")

	if isSelect {
		return s.executeSelect(query, start)
	}
	return s.executeExec(query, start)
}

func (s *SQLiteConnector) executeSelect(query string, start time.Time) (*model.QueryResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return &model.QueryResult{
			Error:    err.Error(),
			Columns:  []string{"ERROR"},
			Rows:     [][]string{{err.Error()}},
			Duration: time.Since(start).String(),
		}, nil
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	result := model.NewQueryResult()
	result.Columns = columns
	result.IsSelect = true

	rowCount := 0
	for rows.Next() {
		if rowCount >= model.MaxDisplayRows {
			result.Message = fmt.Sprintf("Results truncated at %d rows. Add LIMIT/OFFSET to see more.", model.MaxDisplayRows)
			break
		}

		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range columns {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, err
		}

		row := make([]string, len(columns))
		for i, val := range values {
			if val == nil {
				row[i] = "NULL"
			} else {
				switch v := val.(type) {
				case []byte:
					row[i] = string(v)
				default:
					row[i] = fmt.Sprintf("%v", v)
				}
			}
		}
		result.AddRow(row)
		rowCount++
	}

	result.Duration = time.Since(start).String()
	return result, nil
}

func (s *SQLiteConnector) executeExec(query string, start time.Time) (*model.QueryResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := s.db.ExecContext(ctx, query)
	if err != nil {
		return &model.QueryResult{
			Error:    err.Error(),
			Columns:  []string{"ERROR"},
			Rows:     [][]string{{err.Error()}},
			Duration: time.Since(start).String(),
		}, nil
	}

	affected, _ := result.RowsAffected()
	msg := fmt.Sprintf("Query OK, %d rows affected", affected)

	return &model.QueryResult{
		Columns:      []string{"Message"},
		Rows:         [][]string{{msg}},
		RowsAffected: affected,
		Duration:     time.Since(start).String(),
		IsSelect:     false,
		Message:      msg,
	}, nil
}

func (s *SQLiteConnector) ExecuteQueryWithDB(dbName, query string) (*model.QueryResult, error) {
	// SQLite single database, just execute
	return s.ExecuteQuery(query)
}

func (s *SQLiteConnector) CreateDatabase(name string) error {
	// For SQLite, creating a database means creating a new file
	// We'll let the user do this via file selection
	return fmt.Errorf("SQLite: use 'ATTACH DATABASE' to create a new database file")
}

func (s *SQLiteConnector) DropDatabase(name string) error {
	return fmt.Errorf("SQLite: cannot drop a database, delete the file manually")
}

func (s *SQLiteConnector) CreateTable(dbName, tableName string, columns []model.ColumnDef) error {
	var parts []string
	var primaryKeys []string

	for _, col := range columns {
		colType := col.Type
		if col.Length > 0 {
			colType = fmt.Sprintf("%s(%d)", col.Type, col.Length)
		}

		part := fmt.Sprintf("%s %s", quoteIdent(col.Name), colType)

		if !col.Nullable {
			part += " NOT NULL"
		}
		if col.PrimaryKey {
			primaryKeys = append(primaryKeys, quoteIdent(col.Name))
		}
		if col.AutoInc {
			part += " PRIMARY KEY AUTOINCREMENT"
		}
		if col.Default != "" {
			part += fmt.Sprintf(" DEFAULT %s", col.Default)
		}
		if col.Unique {
			part += " UNIQUE"
		}

		parts = append(parts, part)
	}

	if len(primaryKeys) > 0 && !columns[0].AutoInc {
		parts = append(parts, fmt.Sprintf("PRIMARY KEY (%s)", strings.Join(primaryKeys, ", ")))
	}

	query := fmt.Sprintf("CREATE TABLE %s (\n  %s\n)", quoteIdent(tableName), strings.Join(parts, ",\n  "))
	_, err := s.db.ExecContext(context.Background(), query)
	return err
}

func (s *SQLiteConnector) DropTable(dbName, tableName string) error {
	_, err := s.db.ExecContext(context.Background(), fmt.Sprintf("DROP TABLE IF EXISTS %s", quoteIdent(tableName)))
	return err
}

func (s *SQLiteConnector) TruncateTable(dbName, tableName string) error {
	_, err := s.db.ExecContext(context.Background(), fmt.Sprintf("DELETE FROM %s", quoteIdent(tableName)))
	return err
}

func (s *SQLiteConnector) RenameTable(dbName, oldName, newName string) error {
	_, err := s.db.ExecContext(context.Background(), fmt.Sprintf("ALTER TABLE %s RENAME TO %s", quoteIdent(oldName), quoteIdent(newName)))
	return err
}

func (s *SQLiteConnector) AddColumn(dbName, tableName string, col model.ColumnDef) error {
	colType := col.Type
	if col.Length > 0 {
		colType = fmt.Sprintf("%s(%d)", col.Type, col.Length)
	}

	part := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", quoteIdent(tableName), quoteIdent(col.Name), colType)
	if !col.Nullable {
		part += " NOT NULL"
	}
	if col.Default != "" {
		part += fmt.Sprintf(" DEFAULT %s", col.Default)
	}

	_, err := s.db.ExecContext(context.Background(), part)
	return err
}

func (s *SQLiteConnector) DropColumn(dbName, tableName, colName string) error {
	// SQLite 3.35+ supports DROP COLUMN
	_, err := s.db.ExecContext(context.Background(), fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", quoteIdent(tableName), quoteIdent(colName)))
	return err
}

func (s *SQLiteConnector) ModifyColumn(dbName, tableName string, oldName string, col model.ColumnDef) error {
	// SQLite has limited ALTER TABLE support, we recreate the table
	return fmt.Errorf("SQLite has limited ALTER TABLE support. Use a CREATE TABLE statement instead")
}

func (s *SQLiteConnector) GetRowCount(dbName, tableName string) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var count int64
	err := s.db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", quoteIdent(tableName))).Scan(&count)
	return count, err
}
