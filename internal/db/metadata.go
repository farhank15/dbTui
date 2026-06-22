package db

import (
	"github.com/farhank15/dbTui/internal/model"
)

// MetadataExplorer provides methods to explore database metadata
type MetadataExplorer struct {
	connector Connector
}

// NewMetadataExplorer creates a new metadata explorer
func NewMetadataExplorer(connector Connector) *MetadataExplorer {
	return &MetadataExplorer{connector: connector}
}

// ExploreDatabase returns full schema information for a database
func (me *MetadataExplorer) ExploreDatabase(dbName string) (*DatabaseSchema, error) {
	tables, err := me.connector.GetTables(dbName)
	if err != nil {
		return nil, err
	}

	schema := &DatabaseSchema{
		Name:   dbName,
		Tables: make([]*model.TableDetail, 0),
	}

	for _, table := range tables {
		detail, err := me.connector.GetTableDetail(dbName, table.Name)
		if err != nil {
			// Skip tables we can't inspect
			continue
		}
		schema.Tables = append(schema.Tables, detail)
	}

	return schema, nil
}

// DatabaseSchema represents a complete database schema
type DatabaseSchema struct {
	Name   string               `json:"name"`
	Tables []*model.TableDetail `json:"tables"`
}
