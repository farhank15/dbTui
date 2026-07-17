package agent

import (
	"fmt"
	"strings"
	"time"
)

func (s *Server) handleQuery(req Request) Response {
	params, err := parseParams[QueryParams](req.Params)
	if err != nil {
		return s.makeError(req.ID, ErrInvalidParams, "invalid params: "+err.Error(), nil)
	}

	if params.Name == "" {
		return s.makeError(req.ID, ErrInvalidParams, "name is required", nil)
	}
	if params.SQL == "" {
		return s.makeError(req.ID, ErrInvalidParams, "sql is required", nil)
	}

	if !s.manager.IsConnected(params.Name) {
		return s.makeError(req.ID, ErrNotConnected, "not connected: "+params.Name, nil)
	}

	isSelect := isReadOnly(params.SQL)
	strict := true
	if params.Strict != nil {
		strict = *params.Strict
	}
	if strict && !isSelect {
		return s.makeError(req.ID, ErrWriteQueryBlocked, "write queries disabled in strict mode", params.SQL)
	}

	maxRows := params.MaxRows
	if maxRows <= 0 {
		maxRows = s.defaultMaxRows()
	}

	conn, err := s.manager.GetConnector(params.Name)
	if err != nil {
		return s.makeError(req.ID, ErrNotConnected, err.Error(), nil)
	}

	done := make(chan struct{})
	var result *QueryResult
	var queryErr error

	go func() {
		defer close(done)
		defer func() {
			if r := recover(); r != nil {
				queryErr = errFromRecover(r)
			}
		}()

		start := time.Now()
		dbResult, err := conn.ExecuteQuery(params.SQL)
		elapsed := time.Since(start)

		if err != nil {
			queryErr = err
			return
		}

		count := len(dbResult.Rows)
		truncated := false
		rows := dbResult.Rows
		if maxRows > 0 && count > maxRows {
			rows = rows[:maxRows]
			truncated = true
			count = maxRows
		}

		// Convert rows to [][]string (nil for NULL)
		strRows := make([][]string, len(rows))
		for i, row := range rows {
			strRow := make([]string, len(row))
			copy(strRow, row)
			strRows[i] = strRow
		}

		result = &QueryResult{
			Columns:   dbResult.Columns,
			Rows:      strRows,
			Duration:  elapsed.Round(time.Millisecond).String(),
			RowCount:  count,
			IsSelect:  dbResult.IsSelect,
			Truncated: truncated,
		}
	}()

	select {
	case <-done:
		if queryErr != nil {
			return s.makeError(req.ID, ErrInternal, queryErr.Error(), nil)
		}
		s.log("[INFO] query %s: %s (%s, %d rows)", params.Name, truncateSQL(params.SQL), result.Duration, result.RowCount)
		return s.makeResult(req.ID, result)
	case <-time.After(s.queryTimeout()):
		return s.makeError(req.ID, ErrQueryTimeout, "query timed out after 30s", nil)
	}
}

func isReadOnly(sql string) bool {
	trimmed := strings.TrimSpace(sql)
	upper := strings.ToUpper(trimmed)

	// Skip leading WITH if it's a CTE that starts with SELECT
	upper = stripLeadingCTE(upper)

	if strings.HasPrefix(upper, "SELECT") ||
		strings.HasPrefix(upper, "WITH") ||
		strings.HasPrefix(upper, "SHOW") ||
		strings.HasPrefix(upper, "DESCRIBE") ||
		strings.HasPrefix(upper, "DESC") ||
		strings.HasPrefix(upper, "EXPLAIN") ||
		strings.HasPrefix(upper, "PRAGMA") {
		return true
	}
	return false
}

func stripLeadingCTE(upper string) string {
	// Simple: if starts with WITH, skip to the SELECT after the final CTE
	if !strings.HasPrefix(upper, "WITH") {
		return upper
	}
	// Check if it's WITH ... SELECT (read-only) or WITH ... INSERT/UPDATE/DELETE
	idx := strings.Index(upper, " SELECT ")
	if idx != -1 {
		upper = upper[idx+1:] // +1 to start at SELECT
	}
	return upper
}

func truncateSQL(sql string) string {
	cleaned := strings.ReplaceAll(sql, "\n", " ")
	if len(cleaned) > 60 {
		return cleaned[:60] + "..."
	}
	return cleaned
}

func errFromRecover(r interface{}) error {
	switch v := r.(type) {
	case error:
		return v
	default:
		return fmt.Errorf("%v", v)
	}
}
