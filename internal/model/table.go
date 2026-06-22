package model

// ColumnInfo represents a database column
type ColumnInfo struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Nullable   string `json:"nullable"`
	Key        string `json:"key"`
	Default    string `json:"default"`
	Extra      string `json:"extra"`
}

// TableInfo represents a database table with its columns
type TableInfo struct {
	Name       string       `json:"name"`
	Columns    []ColumnInfo `json:"columns"`
	RowCount   int64        `json:"row_count"`
}

// IndexInfo represents a database index
type IndexInfo struct {
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
	Unique  bool     `json:"unique"`
	Primary bool     `json:"primary"`
}

// ForeignKeyInfo represents a foreign key constraint
type ForeignKeyInfo struct {
	Name           string `json:"name"`
	Column         string `json:"column"`
	RefTable       string `json:"ref_table"`
	RefColumn      string `json:"ref_column"`
	OnDelete       string `json:"on_delete"`
	OnUpdate       string `json:"on_update"`
}

// TableDetail contains full table information
type TableDetail struct {
	Table       TableInfo        `json:"table"`
	Indexes     []IndexInfo      `json:"indexes"`
	ForeignKeys []ForeignKeyInfo `json:"foreign_keys"`
}

// ColumnDef is used when creating a table
type ColumnDef struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Length     int    `json:"length"`
	Nullable   bool   `json:"nullable"`
	PrimaryKey bool   `json:"primary_key"`
	AutoInc    bool   `json:"auto_inc"`
	Default    string `json:"default"`
	Unique     bool   `json:"unique"`
}
