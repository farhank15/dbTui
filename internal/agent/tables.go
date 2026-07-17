package agent

func (s *Server) handleTables(req Request) Response {
	params, err := parseParams[TablesParams](req.Params)
	if err != nil {
		return s.makeError(req.ID, ErrInvalidParams, "invalid params: "+err.Error(), nil)
	}

	if params.Name == "" {
		return s.makeError(req.ID, ErrInvalidParams, "name is required", nil)
	}
	if params.Database == "" {
		return s.makeError(req.ID, ErrInvalidParams, "database is required", nil)
	}

	conn, err := s.manager.GetConnector(params.Name)
	if err != nil {
		return s.makeError(req.ID, ErrNotConnected, err.Error(), nil)
	}

	tables, err := conn.GetTables(params.Database)
	if err != nil {
		return s.makeError(req.ID, ErrInternal, "failed to list tables: "+err.Error(), nil)
	}

	entries := make([]TableEntry, 0, len(tables))
	for _, tbl := range tables {
		entries = append(entries, TableEntry{
			Name:     tbl.Name,
			RowCount: tbl.RowCount,
		})
	}

	return s.makeResult(req.ID, TablesResult{Tables: entries})
}
