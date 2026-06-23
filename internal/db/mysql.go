package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/farhank15/dbTui/internal/model"
)

// MySQLConnector implements Connector for MySQL
type MySQLConnector struct {
	mu  sync.Mutex
	db  *sql.DB
	cfg model.Connection
}

// NewMySQLConnector creates a new MySQL connector
func NewMySQLConnector() *MySQLConnector {
	return &MySQLConnector{}
}

func (m *MySQLConnector) Connect(config model.Connection) error {
	dsn := config.DSN()
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("failed to open mysql connection: %w", err)
	}

	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		db.Close()
		return fmt.Errorf("failed to ping mysql: %w", err)
	}

	m.db = db
	m.cfg = config
	return nil
}

func (m *MySQLConnector) useDB(dbName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if dbName == "" {
		return nil
	}
	if m.cfg.Database == dbName {
		return nil
	}

	// Close old connection
	if m.db != nil {
		m.db.Close()
	}

	// Update config database name
	m.cfg.Database = dbName

	// Re-establish connection to the new database
	dsn := m.cfg.DSN()
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}

	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)

	m.db = db
	return nil
}

func (m *MySQLConnector) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db != nil {
		return m.db.Close()
	}
	return nil
}

func (m *MySQLConnector) Ping() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil {
		return fmt.Errorf("not connected")
	}
	return m.db.Ping()
}

func (m *MySQLConnector) IsConnected() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.db != nil && m.db.Ping() == nil
}

func (m *MySQLConnector) GetDB() *sql.DB {
	return m.db
}

func (m *MySQLConnector) GetDatabases() ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := m.db.QueryContext(ctx, "SHOW DATABASES")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var databases []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		// Skip system databases
		if name != "information_schema" && name != "mysql" && name != "performance_schema" && name != "sys" {
			databases = append(databases, name)
		}
	}
	return databases, nil
}

func (m *MySQLConnector) GetTables(dbName string) ([]model.TableInfo, error) {
	if err := m.useDB(dbName); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rows, err := m.db.QueryContext(ctx, "SELECT TABLE_NAME FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_TYPE = 'BASE TABLE' ORDER BY TABLE_NAME", dbName)
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

func (m *MySQLConnector) GetColumns(dbName, tableName string) ([]model.ColumnInfo, error) {
	if err := m.useDB(dbName); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := `SELECT 
		COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, 
		COALESCE(COLUMN_KEY, ''), COALESCE(COLUMN_DEFAULT, ''), 
		COALESCE(EXTRA, '')
	FROM INFORMATION_SCHEMA.COLUMNS 
	WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
	ORDER BY ORDINAL_POSITION`

	rows, err := m.db.QueryContext(ctx, query, dbName, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []model.ColumnInfo
	for rows.Next() {
		var col model.ColumnInfo
		if err := rows.Scan(&col.Name, &col.Type, &col.Nullable, &col.Key, &col.Default, &col.Extra); err != nil {
			return nil, err
		}
		columns = append(columns, col)
	}
	return columns, nil
}

func (m *MySQLConnector) GetIndexes(dbName, tableName string) ([]model.IndexInfo, error) {
	if err := m.useDB(dbName); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := m.db.QueryContext(ctx, fmt.Sprintf("SHOW INDEX FROM %s.%s", quoteIdent(dbName), quoteIdent(tableName)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	indexMap := make(map[string]*model.IndexInfo)
	var order []string

	for rows.Next() {
		var table, nonUnique, keyName, seqInIndex, columnName string
		var collation, subPart, packed, indexType, comment, indexComment, visible, expression sql.NullString
		var cardinality int64

		var nullableCol sql.NullString
		if err := rows.Scan(&table, &nonUnique, &keyName, &seqInIndex, &columnName,
			&collation, &cardinality, &subPart, &packed, &nullableCol, &indexType, &comment, &indexComment, &visible, &expression); err != nil {
			// Try scanning fewer columns for older MySQL
			return m.getIndexesSimple(dbName, tableName)
		}

		if _, ok := indexMap[keyName]; !ok {
			isUnique := nonUnique == "0"
			isPrimary := keyName == "PRIMARY"
			indexMap[keyName] = &model.IndexInfo{
				Name:    keyName,
				Columns: make([]string, 0),
				Unique:  isUnique,
				Primary: isPrimary,
			}
			order = append(order, keyName)
		}
		indexMap[keyName].Columns = append(indexMap[keyName].Columns, columnName)
	}

	var indexes []model.IndexInfo
	for _, name := range order {
		indexes = append(indexes, *indexMap[name])
	}
	return indexes, nil
}

func (m *MySQLConnector) getIndexesSimple(dbName, tableName string) ([]model.IndexInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rows, err := m.db.QueryContext(ctx, fmt.Sprintf("SHOW INDEX FROM %s.%s", quoteIdent(dbName), quoteIdent(tableName)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	indexMap := make(map[string]*model.IndexInfo)
	var order []string

	for rows.Next() {
		var table, nonUnique, keyName, seqInIndex, columnName string
		var collation, cardinality, subPart, packed, indexType, comment string

		var nullableCol sql.NullString
		if err := rows.Scan(&table, &nonUnique, &keyName, &seqInIndex, &columnName,
			&collation, &cardinality, &subPart, &packed, &nullableCol, &indexType, &comment); err != nil {
			return nil, err
		}

		if _, ok := indexMap[keyName]; !ok {
			isUnique := nonUnique == "0"
			isPrimary := keyName == "PRIMARY"
			indexMap[keyName] = &model.IndexInfo{
				Name:    keyName,
				Columns: make([]string, 0),
				Unique:  isUnique,
				Primary: isPrimary,
			}
			order = append(order, keyName)
		}
		indexMap[keyName].Columns = append(indexMap[keyName].Columns, columnName)
	}

	var indexes []model.IndexInfo
	for _, name := range order {
		indexes = append(indexes, *indexMap[name])
	}
	return indexes, nil
}



func (m *MySQLConnector) GetForeignKeys(dbName, tableName string) ([]model.ForeignKeyInfo, error) {
	if err := m.useDB(dbName); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := `SELECT 
		CONSTRAINT_NAME, COLUMN_NAME, REFERENCED_TABLE_NAME, REFERENCED_COLUMN_NAME
		FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE 
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? AND REFERENCED_TABLE_NAME IS NOT NULL`

	rows, err := m.db.QueryContext(ctx, query, dbName, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var fks []model.ForeignKeyInfo
	for rows.Next() {
		var fk model.ForeignKeyInfo
		if err := rows.Scan(&fk.Name, &fk.Column, &fk.RefTable, &fk.RefColumn); err != nil {
			return nil, err
		}
		fks = append(fks, fk)
	}
	return fks, nil
}

func (m *MySQLConnector) GetTableDetail(dbName, tableName string) (*model.TableDetail, error) {
	columns, err := m.GetColumns(dbName, tableName)
	if err != nil {
		return nil, err
	}

	indexes, err := m.GetIndexes(dbName, tableName)
	if err != nil {
		return nil, err
	}

	fks, err := m.GetForeignKeys(dbName, tableName)
	if err != nil {
		return nil, err
	}

	rowCount, err := m.GetRowCount(dbName, tableName)
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

func (m *MySQLConnector) ExecuteQuery(query string) (*model.QueryResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	start := time.Now()
	trimmed := strings.TrimSpace(strings.ToUpper(query))
	isSelect := strings.HasPrefix(trimmed, "SELECT") ||
		strings.HasPrefix(trimmed, "WITH") ||
		strings.HasPrefix(trimmed, "SHOW") ||
		strings.HasPrefix(trimmed, "DESCRIBE") ||
		strings.HasPrefix(trimmed, "EXPLAIN") ||
		strings.HasPrefix(trimmed, "CALL")

	if isSelect {
		return m.executeSelect(query, start)
	}
	return m.executeExec(query, start)
}

func (m *MySQLConnector) executeSelect(query string, start time.Time) (*model.QueryResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := m.db.QueryContext(ctx, query)
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

func (m *MySQLConnector) executeExec(query string, start time.Time) (*model.QueryResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := m.db.ExecContext(ctx, query)
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

func (m *MySQLConnector) ExecuteQueryWithDB(dbName, query string) (*model.QueryResult, error) {
	if err := m.useDB(dbName); err != nil {
		return nil, err
	}
	return m.ExecuteQuery(query)
}

func (m *MySQLConnector) CreateDatabase(name string) error {
	_, err := m.db.ExecContext(context.Background(), fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci", quoteIdent(name)))
	return err
}

func (m *MySQLConnector) DropDatabase(name string) error {
	_, err := m.db.ExecContext(context.Background(), fmt.Sprintf("DROP DATABASE IF EXISTS %s", quoteIdent(name)))
	return err
}

func (m *MySQLConnector) CreateTable(dbName, tableName string, columns []model.ColumnDef) error {
	if _, err := m.db.ExecContext(context.Background(), fmt.Sprintf("USE %s", quoteIdent(dbName))); err != nil {
		return err
	}

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
		if col.AutoInc {
			part += " AUTO_INCREMENT"
		}
		if col.Default != "" {
			part += fmt.Sprintf(" DEFAULT %s", col.Default)
		}
		if col.Unique {
			part += " UNIQUE"
		}
		if col.PrimaryKey {
			primaryKeys = append(primaryKeys, quoteIdent(col.Name))
		}

		parts = append(parts, part)
	}

	if len(primaryKeys) > 0 {
		parts = append(parts, fmt.Sprintf("PRIMARY KEY (%s)", strings.Join(primaryKeys, ", ")))
	}

	query := fmt.Sprintf("CREATE TABLE %s (\n  %s\n) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4",
		quoteIdent(tableName), strings.Join(parts, ",\n  "))
	_, err := m.db.ExecContext(context.Background(), query)
	return err
}

func (m *MySQLConnector) DropTable(dbName, tableName string) error {
	if _, err := m.db.ExecContext(context.Background(), fmt.Sprintf("USE %s", quoteIdent(dbName))); err != nil {
		return err
	}
	_, err := m.db.ExecContext(context.Background(), fmt.Sprintf("DROP TABLE IF EXISTS %s", quoteIdent(tableName)))
	return err
}

func (m *MySQLConnector) TruncateTable(dbName, tableName string) error {
	if _, err := m.db.ExecContext(context.Background(), fmt.Sprintf("USE %s", quoteIdent(dbName))); err != nil {
		return err
	}
	_, err := m.db.ExecContext(context.Background(), fmt.Sprintf("TRUNCATE TABLE %s", quoteIdent(tableName)))
	return err
}

func (m *MySQLConnector) RenameTable(dbName, oldName, newName string) error {
	if _, err := m.db.ExecContext(context.Background(), fmt.Sprintf("USE %s", quoteIdent(dbName))); err != nil {
		return err
	}
	_, err := m.db.ExecContext(context.Background(), fmt.Sprintf("RENAME TABLE %s TO %s", quoteIdent(oldName), quoteIdent(newName)))
	return err
}

func (m *MySQLConnector) AddColumn(dbName, tableName string, col model.ColumnDef) error {
	if _, err := m.db.ExecContext(context.Background(), fmt.Sprintf("USE %s", quoteIdent(dbName))); err != nil {
		return err
	}

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

	_, err := m.db.ExecContext(context.Background(), part)
	return err
}

func (m *MySQLConnector) DropColumn(dbName, tableName, colName string) error {
	if _, err := m.db.ExecContext(context.Background(), fmt.Sprintf("USE %s", quoteIdent(dbName))); err != nil {
		return err
	}
	_, err := m.db.ExecContext(context.Background(), fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", quoteIdent(tableName), quoteIdent(colName)))
	return err
}

func (m *MySQLConnector) ModifyColumn(dbName, tableName string, oldName string, col model.ColumnDef) error {
	if _, err := m.db.ExecContext(context.Background(), fmt.Sprintf("USE %s", quoteIdent(dbName))); err != nil {
		return err
	}

	colType := col.Type
	if col.Length > 0 {
		colType = fmt.Sprintf("%s(%d)", col.Type, col.Length)
	}

	part := fmt.Sprintf("ALTER TABLE %s MODIFY COLUMN %s %s", quoteIdent(tableName), quoteIdent(oldName), colType)
	if !col.Nullable {
		part += " NOT NULL"
	}
	if col.Default != "" {
		part += fmt.Sprintf(" DEFAULT %s", col.Default)
	}

	_, err := m.db.ExecContext(context.Background(), part)
	return err
}

func (m *MySQLConnector) GetRowCount(dbName, tableName string) (int64, error) {
	if err := m.useDB(dbName); err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var count int64
	err := m.db.QueryRowContext(ctx,
		fmt.Sprintf("SELECT COUNT(*) FROM %s.%s", quoteIdent(dbName), quoteIdent(tableName))).Scan(&count)
	return count, err
}

func quoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}


