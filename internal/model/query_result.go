package model

import "fmt"

// MaxDisplayRows is the maximum number of rows to display in the result table
const MaxDisplayRows = 1000

// QueryResult represents the result of a SQL query
type QueryResult struct {
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`
	RowsAffected int64 `json:"rows_affected"`
	Error    string   `json:"error,omitempty"`
	Duration string   `json:"duration"`
	IsSelect bool     `json:"is_select"`
	Message  string   `json:"message,omitempty"`
	ConnID   string   `json:"conn_id,omitempty"`
	Database string   `json:"database,omitempty"`
	Table    string   `json:"table,omitempty"`
}

// NewQueryResult creates a new query result
func NewQueryResult() *QueryResult {
	return &QueryResult{
		Columns: make([]string, 0),
		Rows:    make([][]string, 0),
	}
}

// AddRow adds a row to the result
func (qr *QueryResult) AddRow(row []string) {
	qr.Rows = append(qr.Rows, row)
}

// RowCount returns the number of rows
func (qr *QueryResult) RowCount() int {
	return len(qr.Rows)
}

// ColCount returns the number of columns
func (qr *QueryResult) ColCount() int {
	return len(qr.Columns)
}

// CSV returns the result as a CSV string
func (qr *QueryResult) CSV() string {
	var out string
	for i, col := range qr.Columns {
		if i > 0 {
			out += ","
		}
		out += escapeCSV(col)
	}
	out += "\n"
	for _, row := range qr.Rows {
		for i, val := range row {
			if i > 0 {
				out += ","
			}
			out += escapeCSV(val)
		}
		out += "\n"
	}
	return out
}

func escapeCSV(s string) string {
	needsQuoting := false
	for _, c := range s {
		if c == ',' || c == '"' || c == '\n' || c == '\r' {
			needsQuoting = true
			break
		}
	}
	if !needsQuoting {
		return s
	}
	escaped := ""
	for _, c := range s {
		if c == '"' {
			escaped += "\""
		}
		escaped += string(c)
	}
	return fmt.Sprintf("\"%s\"", escaped)
}
