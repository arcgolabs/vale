package runtime

import "github.com/hashicorp/go-memdb"

func catalogSchema() *memdb.DBSchema {
	return &memdb.DBSchema{
		Tables: map[string]*memdb.TableSchema{
			catalogTableRoute: {
				Name: catalogTableRoute,
				Indexes: map[string]*memdb.IndexSchema{
					"id":                          stringIndex("id", "Name", true),
					"entrypoint":                  stringIndex("entrypoint", "Entrypoint", false),
					"service":                     stringIndex("service", "Service", false),
					"host":                        stringIndex("host", "Host", false),
					"path_prefix":                 stringIndex("path_prefix", "PathPrefix", false),
					"entrypoint_service":          compoundStringIndex("entrypoint_service", "Entrypoint", "Service"),
					"entrypoint_host":             compoundStringIndex("entrypoint_host", "Entrypoint", "Host"),
					"entrypoint_path_prefix":      compoundStringIndex("entrypoint_path_prefix", "Entrypoint", "PathPrefix"),
					"entrypoint_host_path_prefix": compoundStringIndex("entrypoint_host_path_prefix", "Entrypoint", "Host", "PathPrefix"),
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

func compoundStringIndex(name string, fields ...string) *memdb.IndexSchema {
	indexes := make([]memdb.Indexer, 0, len(fields))
	for _, field := range fields {
		indexes = append(indexes, &memdb.StringFieldIndex{Field: field})
	}
	return &memdb.IndexSchema{
		Name:   name,
		Unique: false,
		Indexer: &memdb.CompoundIndex{
			Indexes: indexes,
		},
	}
}
