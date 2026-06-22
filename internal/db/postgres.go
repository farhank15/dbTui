package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/farhank15/dbTui/internal/model"
)

// PostgresConnector implements Connector for PostgreSQL
type PostgresConnector struct {
	db  *sql.DB
	cfg model.Connection
}

// NewPostgresConnector creates a new PostgreSQL connector
func NewPostgresConnector() *PostgresConnector {
	return &PostgresConnector{}
}

func (p *PostgresConnector) Connect(config model.Connection) error {
	// Pre-check: database name is required
	if config.Database == "" {
		// Default to 'postgres' like psql does
		config.Database = "postgres"
	}

	// Pre-check: host is required
	if config.Host == "" {
		config.Host = "localhost"
	}

	// Pre-check: port should be valid
	if config.Port <= 0 {
		config.Port = 5432
	}

	dsn := config.DSN()

	// Use a shorter timeout for initial connection attempt
	connCtx, connCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer connCancel()

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("failed to open postgres connection: %w", err)
	}

	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.PingContext(connCtx); err != nil {
		db.Close()
		return fmt.Errorf("failed to connect to PostgreSQL at %s:%d as '%s': %w",
			config.Host, config.Port, config.User, err)
	}

	p.db = db
	p.cfg = config
	return nil
}

func (p *PostgresConnector) Close() error {
	if p.db != nil {
		return p.db.Close()
	}
	return nil
}

func (p *PostgresConnector) Ping() error {
	if p.db == nil {
		return fmt.Errorf("not connected")
	}
	return p.db.Ping()
}

func (p *PostgresConnector) IsConnected() bool {
	return p.db != nil && p.db.Ping() == nil
}

func (p *PostgresConnector) GetDB() *sql.DB {
	return p.db
}

func (p *PostgresConnector) GetDatabases() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := p.db.QueryContext(ctx, "SELECT datname FROM pg_database WHERE datistemplate = false ORDER BY datname")
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
		databases = append(databases, name)
	}
	return databases, nil
}

func (p *PostgresConnector) useDB(dbName string) error {
	if p.cfg.Database == dbName {
		return nil
	}

	// Close old connection
	if p.db != nil {
		p.db.Close()
	}

	// Update config database name
	p.cfg.Database = dbName

	// Re-establish connection to the new database
	dsn := p.cfg.DSN()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}

	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)

	p.db = db
	return nil
}

func (p *PostgresConnector) GetTables(dbName string) ([]model.TableInfo, error) {
	if err := p.useDB(dbName); err != nil {
		// If we can't switch database, try querying with schema
		return p.getTablesForSchema("public")
	}
	return p.getTablesForSchema("public")
}

func (p *PostgresConnector) getTablesForSchema(schema string) ([]model.TableInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := `SELECT table_name FROM information_schema.tables 
		WHERE table_schema = $1 AND table_type = 'BASE TABLE' 
		ORDER BY table_name`

	rows, err := p.db.QueryContext(ctx, query, schema)
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

func (p *PostgresConnector) GetColumns(dbName, tableName string) ([]model.ColumnInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := `SELECT 
		c.column_name, 
		c.data_type, 
		c.is_nullable,
		COALESCE(c.character_maximum_length::text, '') as char_max,
		COALESCE(c.column_default, '') as column_default,
		CASE WHEN pk.column_name IS NOT NULL THEN 'PRI' ELSE '' END as key_type,
		COALESCE((
			SELECT 'auto_increment' 
			FROM information_schema.columns c2 
			WHERE c2.table_schema = c.table_schema 
			AND c2.table_name = c.table_name 
			AND c2.column_name = c.column_name 
			AND c2.column_default LIKE 'nextval%'
		), '') as extra
	FROM information_schema.columns c
	LEFT JOIN (
		SELECT ku.column_name, tc.table_schema, tc.table_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage ku ON tc.constraint_name = ku.constraint_name
		WHERE tc.constraint_type = 'PRIMARY KEY'
	) pk ON c.table_schema = pk.table_schema AND c.table_name = pk.table_name AND c.column_name = pk.column_name
	WHERE c.table_schema = 'public' AND c.table_name = $1
	ORDER BY c.ordinal_position`

	rows, err := p.db.QueryContext(ctx, query, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []model.ColumnInfo
	for rows.Next() {
		var col model.ColumnInfo
		var charMax, defaultVal, keyType, extra string
		if err := rows.Scan(&col.Name, &col.Type, &col.Nullable, &charMax, &defaultVal, &keyType, &extra); err != nil {
			return nil, err
		}

		col.Default = defaultVal
		col.Key = keyType
		col.Extra = extra

		if charMax != "" {
			col.Type = fmt.Sprintf("%s(%s)", col.Type, charMax)
		}
		if col.Nullable == "YES" {
			col.Nullable = "YES"
		} else {
			col.Nullable = "NO"
		}

		columns = append(columns, col)
	}
	return columns, nil
}

func (p *PostgresConnector) GetIndexes(dbName, tableName string) ([]model.IndexInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := `SELECT
		i.relname as index_name,
		a.attname as column_name,
		ix.indisunique as is_unique,
		ix.indisprimary as is_primary
	FROM pg_class t
	JOIN pg_index ix ON t.oid = ix.indrelid
	JOIN pg_class i ON i.oid = ix.indexrelid
	JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = ANY(ix.indkey)
	WHERE t.relkind = 'r' AND t.relname = $1
	ORDER BY i.relname, a.attnum`

	rows, err := p.db.QueryContext(ctx, query, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	indexMap := make(map[string]*model.IndexInfo)
	var order []string

	for rows.Next() {
		var idxName, colName string
		var isUnique, isPrimary bool
		if err := rows.Scan(&idxName, &colName, &isUnique, &isPrimary); err != nil {
			return nil, err
		}

		if _, ok := indexMap[idxName]; !ok {
			indexMap[idxName] = &model.IndexInfo{
				Name:    idxName,
				Columns: make([]string, 0),
				Unique:  isUnique,
				Primary: isPrimary,
			}
			order = append(order, idxName)
		}
		indexMap[idxName].Columns = append(indexMap[idxName].Columns, colName)
	}

	var indexes []model.IndexInfo
	for _, name := range order {
		indexes = append(indexes, *indexMap[name])
	}
	return indexes, nil
}

func (p *PostgresConnector) GetForeignKeys(dbName, tableName string) ([]model.ForeignKeyInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := `SELECT
		tc.constraint_name,
		kcu.column_name,
		ccu.table_name AS ref_table,
		ccu.column_name AS ref_column,
		rc.delete_rule,
		rc.update_rule
	FROM information_schema.table_constraints tc
	JOIN information_schema.key_column_usage kcu ON tc.constraint_name = kcu.constraint_name
	JOIN information_schema.constraint_column_usage ccu ON tc.constraint_name = ccu.constraint_name
	JOIN information_schema.referential_constraints rc ON tc.constraint_name = rc.constraint_name
	WHERE tc.constraint_type = 'FOREIGN KEY' AND tc.table_name = $1`

	rows, err := p.db.QueryContext(ctx, query, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var fks []model.ForeignKeyInfo
	for rows.Next() {
		var fk model.ForeignKeyInfo
		if err := rows.Scan(&fk.Name, &fk.Column, &fk.RefTable, &fk.RefColumn, &fk.OnDelete, &fk.OnUpdate); err != nil {
			return nil, err
		}
		fks = append(fks, fk)
	}
	return fks, nil
}

func (p *PostgresConnector) GetTableDetail(dbName, tableName string) (*model.TableDetail, error) {
	columns, err := p.GetColumns(dbName, tableName)
	if err != nil {
		return nil, err
	}

	indexes, err := p.GetIndexes(dbName, tableName)
	if err != nil {
		return nil, err
	}

	fks, err := p.GetForeignKeys(dbName, tableName)
	if err != nil {
		return nil, err
	}

	rowCount, err := p.GetRowCount(dbName, tableName)
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

func (p *PostgresConnector) ExecuteQuery(query string) (*model.QueryResult, error) {
	start := time.Now()
	trimmed := strings.TrimSpace(strings.ToUpper(query))
	isSelect := strings.HasPrefix(trimmed, "SELECT") ||
		strings.HasPrefix(trimmed, "WITH") ||
		strings.HasPrefix(trimmed, "SHOW") ||
		strings.HasPrefix(trimmed, "DESCRIBE") ||
		strings.HasPrefix(trimmed, "EXPLAIN")

	if isSelect {
		return p.executeSelect(query, start)
	}
	return p.executeExec(query, start)
}

func (p *PostgresConnector) executeSelect(query string, start time.Time) (*model.QueryResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := p.db.QueryContext(ctx, query)
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
				row[i] = fmt.Sprintf("%v", val)
			}
		}
		result.AddRow(row)
		rowCount++
	}

	result.Duration = time.Since(start).String()
	return result, nil
}

func (p *PostgresConnector) executeExec(query string, start time.Time) (*model.QueryResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := p.db.ExecContext(ctx, query)
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

func (p *PostgresConnector) ExecuteQueryWithDB(dbName, query string) (*model.QueryResult, error) {
	if err := p.useDB(dbName); err != nil {
		// Fallback: prefix query with SET search_path
		query = fmt.Sprintf("SET search_path TO %s; %s", pqQuoteIdent(dbName), query)
	}
	return p.ExecuteQuery(query)
}

func (p *PostgresConnector) CreateDatabase(name string) error {
	_, err := p.db.ExecContext(context.Background(), fmt.Sprintf("CREATE DATABASE %s", pqQuoteIdent(name)))
	return err
}

func (p *PostgresConnector) DropDatabase(name string) error {
	_, err := p.db.ExecContext(context.Background(), fmt.Sprintf("DROP DATABASE IF EXISTS %s", pqQuoteIdent(name)))
	return err
}

func (p *PostgresConnector) CreateTable(dbName, tableName string, columns []model.ColumnDef) error {
	if err := p.useDB(dbName); err != nil {
		return err
	}

	var parts []string
	for _, col := range columns {
		colType := col.Type
		if col.Length > 0 {
			colType = fmt.Sprintf("%s(%d)", col.Type, col.Length)
		}

		part := fmt.Sprintf("%s %s", pqQuoteIdent(col.Name), colType)

		if !col.Nullable {
			part += " NOT NULL"
		}
		if col.Default != "" {
			part += fmt.Sprintf(" DEFAULT %s", col.Default)
		}
		if col.AutoInc {
			part += " GENERATED ALWAYS AS IDENTITY"
		}
		if col.Unique {
			part += " UNIQUE"
		}
		if col.PrimaryKey {
			part += " PRIMARY KEY"
		}

		parts = append(parts, part)
	}

	query := fmt.Sprintf("CREATE TABLE %s (\n  %s\n)", pqQuoteIdent(tableName), strings.Join(parts, ",\n  "))
	_, err := p.db.ExecContext(context.Background(), query)
	return err
}

func (p *PostgresConnector) DropTable(dbName, tableName string) error {
	if err := p.useDB(dbName); err != nil {
		return err
	}
	_, err := p.db.ExecContext(context.Background(), fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE", pqQuoteIdent(tableName)))
	return err
}

func (p *PostgresConnector) TruncateTable(dbName, tableName string) error {
	if err := p.useDB(dbName); err != nil {
		return err
	}
	_, err := p.db.ExecContext(context.Background(), fmt.Sprintf("TRUNCATE TABLE %s", pqQuoteIdent(tableName)))
	return err
}

func (p *PostgresConnector) RenameTable(dbName, oldName, newName string) error {
	if err := p.useDB(dbName); err != nil {
		return err
	}
	_, err := p.db.ExecContext(context.Background(), fmt.Sprintf("ALTER TABLE %s RENAME TO %s", pqQuoteIdent(oldName), pqQuoteIdent(newName)))
	return err
}

func (p *PostgresConnector) AddColumn(dbName, tableName string, col model.ColumnDef) error {
	if err := p.useDB(dbName); err != nil {
		return err
	}

	colType := col.Type
	if col.Length > 0 {
		colType = fmt.Sprintf("%s(%d)", col.Type, col.Length)
	}

	part := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", pqQuoteIdent(tableName), pqQuoteIdent(col.Name), colType)
	if !col.Nullable {
		part += " NOT NULL"
	}
	if col.Default != "" {
		part += fmt.Sprintf(" DEFAULT %s", col.Default)
	}

	_, err := p.db.ExecContext(context.Background(), part)
	return err
}

func (p *PostgresConnector) DropColumn(dbName, tableName, colName string) error {
	if err := p.useDB(dbName); err != nil {
		return err
	}
	_, err := p.db.ExecContext(context.Background(), fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", pqQuoteIdent(tableName), pqQuoteIdent(colName)))
	return err
}

func (p *PostgresConnector) ModifyColumn(dbName, tableName string, oldName string, col model.ColumnDef) error {
	// PostgreSQL doesn't support MODIFY COLUMN, we use ALTER TABLE + ALTER COLUMN
	if err := p.useDB(dbName); err != nil {
		return err
	}

	// Rename column if name changed
	if oldName != col.Name {
		if _, err := p.db.ExecContext(context.Background(), fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s",
			pqQuoteIdent(tableName), pqQuoteIdent(oldName), pqQuoteIdent(col.Name))); err != nil {
			return err
		}
	}

	// Type change
	colType := col.Type
	if col.Length > 0 {
		colType = fmt.Sprintf("%s(%d)", col.Type, col.Length)
	}
	if _, err := p.db.ExecContext(context.Background(), fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s",
		pqQuoteIdent(tableName), pqQuoteIdent(col.Name), colType)); err != nil {
		return err
	}

	// Nullable
	if col.Nullable {
		_, _ = p.db.ExecContext(context.Background(), fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP NOT NULL",
			pqQuoteIdent(tableName), pqQuoteIdent(col.Name)))
	} else {
		_, _ = p.db.ExecContext(context.Background(), fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET NOT NULL",
			pqQuoteIdent(tableName), pqQuoteIdent(col.Name)))
	}

	// Default
	if col.Default != "" {
		_, _ = p.db.ExecContext(context.Background(), fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET DEFAULT %s",
			pqQuoteIdent(tableName), pqQuoteIdent(col.Name), col.Default))
	}

	return nil
}

func (p *PostgresConnector) GetRowCount(dbName, tableName string) (int64, error) {
	if err := p.useDB(dbName); err != nil {
		return 0, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var count int64
	err := p.db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", pqQuoteIdent(tableName))).Scan(&count)
	return count, err
}

func pqQuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
