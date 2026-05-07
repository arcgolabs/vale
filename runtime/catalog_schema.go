package runtime

import "github.com/hashicorp/go-memdb"

func catalogSchema() *memdb.DBSchema {
	return &memdb.DBSchema{
		Tables: map[string]*memdb.TableSchema{
			catalogTableRoute: {
				Name: catalogTableRoute,
				Indexes: map[string]*memdb.IndexSchema{
					"id":          stringIndex("id", "Name", true),
					"entrypoint":  stringIndex("entrypoint", "Entrypoint", false),
					"service":     stringIndex("service", "Service", false),
					"host":        stringIndex("host", "Host", false),
					"path_prefix": stringIndex("path_prefix", "PathPrefix", false),
				},
			},
			catalogTableService: {
				Name: catalogTableService,
				Indexes: map[string]*memdb.IndexSchema{
					"id": stringIndex("id", "Name", true),
				},
			},
			catalogTableEndpoint: {
				Name: catalogTableEndpoint,
				Indexes: map[string]*memdb.IndexSchema{
					"id":      stringIndex("id", "ID", true),
					"service": stringIndex("service", "Service", false),
				},
			},
			catalogTableMiddleware: {
				Name: catalogTableMiddleware,
				Indexes: map[string]*memdb.IndexSchema{
					"id":   stringIndex("id", "Name", true),
					"type": stringIndex("type", "Type", false),
				},
			},
		},
	}
}

func stringIndex(name, field string, unique bool) *memdb.IndexSchema {
	return &memdb.IndexSchema{
		Name:    name,
		Unique:  unique,
		Indexer: &memdb.StringFieldIndex{Field: field},
	}
}
