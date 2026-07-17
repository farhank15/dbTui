package agent

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/farhank15/dbTui/internal/db"
)

func (s *Server) handleConnect(req Request) Response {
	params, err := parseParams[ConnectParams](req.Params)
	if err != nil {
		return s.makeError(req.ID, ErrInvalidParams, "invalid params: "+err.Error(), nil)
	}

	if params.Name == "" {
		return s.makeError(req.ID, ErrInvalidParams, "name is required", nil)
	}
	if params.URL == "" {
		return s.makeError(req.ID, ErrInvalidParams, "url is required", nil)
	}

	connModel, err := db.ParseDSN(params.URL)
	if err != nil {
		return s.makeError(req.ID, ErrUnsupportedType, err.Error(), nil)
	}

	connModel.Name = params.Name
	connModel.ID = params.Name

	if connModel.SSLMode == "" {
		connModel.SSLMode = "disable"
	}

	start := time.Now()
	if err := s.manager.Connect(connModel); err != nil {
		return s.makeError(req.ID, ErrConnectionFailed, err.Error(), nil)
	}
	elapsed := time.Since(start)

	s.log("[INFO] connect %s -> %s (%s)", params.Name, s.redactURL(params.URL), elapsed)

	version := s.detectVersion(params.Name)
	s.manager.RefreshDatabases(params.Name)

	state := s.manager.GetConnectionState(params.Name)
	dbName := connModel.Database
	if state != nil && len(state.Databases) > 0 {
		dbName = state.Databases[0]
	}

	return s.makeResult(req.ID, ConnectResult{
		Connected: true,
		Type:      string(connModel.Type),
		Version:   version,
		Database:  dbName,
	})
}

func (s *Server) detectVersion(name string) string {
	conn, err := s.manager.GetConnector(name)
	if err != nil {
		return ""
	}

	state := s.manager.GetConnectionState(name)
	if state != nil && state.Connection.Type == "sqlite" {
		res, err := conn.ExecuteQuery("SELECT sqlite_version()")
		if err == nil && len(res.Rows) > 0 && len(res.Rows[0]) > 0 {
			return res.Rows[0][0]
		}
		return ""
	}

	res, err := conn.ExecuteQuery("SELECT version()")
	if err != nil || len(res.Rows) == 0 || len(res.Rows[0]) == 0 {
		return ""
	}
	return res.Rows[0][0]
}

func (s *Server) handleDisconnect(req Request) Response {
	params, err := parseParams[DisconnectParams](req.Params)
	if err != nil {
		return s.makeError(req.ID, ErrInvalidParams, "invalid params: "+err.Error(), nil)
	}

	if !s.manager.IsConnected(params.Name) {
		return s.makeError(req.ID, ErrNotConnected, "not connected: "+params.Name, nil)
	}

	if err := s.manager.Disconnect(params.Name); err != nil {
		return s.makeError(req.ID, ErrConnectionFailed, "disconnect failed: "+err.Error(), nil)
	}

	s.log("[INFO] disconnect %s", params.Name)
	return s.makeResult(req.ID, DisconnectResult{Disconnected: true})
}

func (s *Server) handleList(req Request) Response {
	states := s.manager.GetActiveConnections()
	entries := make([]ListConnEntry, 0, len(states))
	for _, st := range states {
		entry := ListConnEntry{
			Name:      st.Connection.ID,
			Type:      string(st.Connection.Type),
			Host:      st.Connection.Host,
			Port:      st.Connection.Port,
			Database:  st.Connection.Database,
			File:      st.Connection.File,
			Connected: st.Connected,
		}
		entries = append(entries, entry)
	}
	return s.makeResult(req.ID, ListResult{Connections: entries})
}

func parseParams[T any](raw interface{}) (T, error) {
	var zero T
	if raw == nil {
		return zero, nil
	}
	var params T
	switch v := raw.(type) {
	case map[string]interface{}:
		b, err := json.Marshal(v)
		if err != nil {
			return zero, err
		}
		if err := json.Unmarshal(b, &params); err != nil {
			return zero, err
		}
		return params, nil
	default:
		return zero, fmt.Errorf("unexpected params type %T", raw)
	}
}
