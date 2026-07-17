package agent

import (
	"github.com/farhank15/dbTui/internal/model"
)

func (s *Server) handleSchema(req Request) Response {
	params, err := parseParams[SchemaParams](req.Params)
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

	var databases []string
	if params.Database != "" {
		databases = []string{params.Database}
	} else {
		databases = state.Databases
	}

	schemaDBs := make([]SchemaDBEntry, 0, len(databases))
	for _, dbName := range databases {
		entry := SchemaDBEntry{Name: dbName}

		tables, err := conn.GetTables(dbName)
		if err != nil {
			continue
		}

		schemaTables := make([]SchemaTable, 0, len(tables))
		for _, tbl := range tables {
			detail, err := conn.GetTableDetail(dbName, tbl.Name)
			if err != nil {
				continue
			}
			schemaTables = append(schemaTables, tableDetailToSchema(detail))
		}
		entry.Tables = schemaTables
		schemaDBs = append(schemaDBs, entry)
	}

	return s.makeResult(req.ID, SchemaResult{Databases: schemaDBs})
}

func tableDetailToSchema(detail *model.TableDetail) SchemaTable {
	cols := make([]SchemaColumn, len(detail.Table.Columns))
	for i, c := range detail.Table.Columns {
		cols[i] = SchemaColumn{
			Name:     c.Name,
			Type:     c.Type,
			Nullable: c.Nullable,
			Key:      c.Key,
			Default:  c.Default,
			Extra:    c.Extra,
		}
	}

	idxs := make([]SchemaIndex, len(detail.Indexes))
	for i, idx := range detail.Indexes {
		idxs[i] = SchemaIndex{
			Name:    idx.Name,
			Columns: idx.Columns,
			Unique:  idx.Unique,
		}
	}

	fks := make([]SchemaFK, len(detail.ForeignKeys))
	for i, fk := range detail.ForeignKeys {
		fks[i] = SchemaFK{
			Name:      fk.Name,
			Column:    fk.Column,
			RefTable:  fk.RefTable,
			RefColumn: fk.RefColumn,
			OnDelete:  fk.OnDelete,
			OnUpdate:  fk.OnUpdate,
		}
	}

	return SchemaTable{
		Name:        detail.Table.Name,
		Columns:     cols,
		Indexes:     idxs,
		ForeignKeys: fks,
		RowCount:    detail.Table.RowCount,
	}
}
