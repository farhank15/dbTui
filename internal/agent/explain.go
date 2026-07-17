package agent

import (
	"strings"
	"time"

	"github.com/farhank15/dbTui/internal/db"
)

func (s *Server) handleExplain(req Request) Response {
	params, err := parseParams[ExplainParams](req.Params)
	if err != nil {
		return s.makeError(req.ID, ErrInvalidParams, "invalid params: "+err.Error(), nil)
	}

	if params.Name == "" {
		return s.makeError(req.ID, ErrInvalidParams, "name is required", nil)
	}
	if params.SQL == "" {
		return s.makeError(req.ID, ErrInvalidParams, "sql is required", nil)
	}

	conn, err := s.manager.GetConnector(params.Name)
	if err != nil {
		return s.makeError(req.ID, ErrNotConnected, err.Error(), nil)
	}

	explainSQL := buildExplainSQL(params.SQL, conn)

	start := time.Now()
	result, err := conn.ExecuteQuery(explainSQL)
	elapsed := time.Since(start)

	if err != nil {
		return s.makeError(req.ID, ErrInternal, "explain failed: "+err.Error(), nil)
	}

	// Format explain output into a single string
	var planParts []string
	for _, row := range result.Rows {
		planParts = append(planParts, strings.Join(row, " "))
	}
	plan := strings.Join(planParts, "\n")

	return s.makeResult(req.ID, ExplainResult{
		Plan:     plan,
		Duration: elapsed.Round(time.Millisecond).String(),
	})
}

func buildExplainSQL(sql string, conn db.Connector) string {
	// Determine DB type from the connector
	switch conn.(type) {
	case *db.PostgresConnector:
		return "EXPLAIN (FORMAT JSON) " + sql
	case *db.MySQLConnector:
		return "EXPLAIN FORMAT=JSON " + sql
	case *db.SQLiteConnector:
		return "EXPLAIN QUERY PLAN " + sql
	default:
		return "EXPLAIN " + sql
	}
}
