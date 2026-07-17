package agent

import (
	"strconv"
	"time"

	"github.com/farhank15/dbTui/internal/db"
)

func (s *Server) handleStats(req Request) Response {
	params, err := parseParams[StatsParams](req.Params)
	if err != nil {
		return s.makeError(req.ID, ErrInvalidParams, "invalid params: "+err.Error(), nil)
	}

	if params.Name == "" {
		return s.makeError(req.ID, ErrInvalidParams, "name is required", nil)
	}

	conn, err := s.manager.GetConnector(params.Name)
	if err != nil {
		return s.makeError(req.ID, ErrNotConnected, err.Error(), nil)
	}

	state := s.manager.GetConnectionState(params.Name)
	if state == nil {
		return s.makeError(req.ID, ErrNotConnected, "not connected", nil)
	}

	dbName := state.Connection.Database
	tableCount := 0
	if len(state.Databases) > 0 {
		tables, err := conn.GetTables(state.Databases[0])
		if err == nil {
			tableCount = len(tables)
		}
		dbName = state.Databases[0]
	}

	sizeMB := getDBSize(conn, state)
	activeConns := len(s.manager.GetActiveConnections())
	uptime := time.Since(s.startAt).Round(time.Second).String()

	return s.makeResult(req.ID, StatsResult{
		Database:          dbName,
		SizeMB:            sizeMB,
		TableCount:        tableCount,
		ActiveConnections: activeConns,
		Uptime:            uptime,
	})
}

func getDBSize(conn db.Connector, state *db.ConnectionState) float64 {
	if state == nil || len(state.Databases) == 0 {
		return 0
	}

	switch c := conn.(type) {
	case *db.PostgresConnector:
		res, err := c.ExecuteQuery("SELECT pg_database_size(current_database())")
		if err == nil && len(res.Rows) > 0 && len(res.Rows[0]) > 0 {
			if val, err := strconv.ParseFloat(res.Rows[0][0], 64); err == nil {
				return val / 1024 / 1024
			}
		}
	case *db.MySQLConnector:
		q := "SELECT SUM(data_length + index_length) / 1024 / 1024 FROM information_schema.tables WHERE table_schema = DATABASE()"
		res, err := c.ExecuteQuery(q)
		if err == nil && len(res.Rows) > 0 && len(res.Rows[0]) > 0 {
			if val, err := strconv.ParseFloat(res.Rows[0][0], 64); err == nil {
				return val
			}
		}
	case *db.SQLiteConnector:
		res, err := c.ExecuteQuery("PRAGMA page_count")
		if err != nil || len(res.Rows) == 0 || len(res.Rows[0]) == 0 {
			return 0
		}
		pageCount, err := strconv.ParseFloat(res.Rows[0][0], 64)
		if err != nil {
			return 0
		}
		res2, err := c.ExecuteQuery("PRAGMA page_size")
		if err != nil || len(res2.Rows) == 0 || len(res2.Rows[0]) == 0 {
			return 0
		}
		pageSize, err := strconv.ParseFloat(res2.Rows[0][0], 64)
		if err != nil {
			return 0
		}
		return (pageCount * pageSize) / 1024 / 1024
	}
	return 0
}
