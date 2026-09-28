package qlcache

import (
	"strings"

	"github.com/eru-os/eru/eru-ql/module_model"
)

// TablesToTags converts a TablesInQuery (as returned by SqlMakerI.ExtractTableNames)
// into the normalized tag list used by the cache. The schema prefix (e.g. "public.")
// comes from SqlMakerI.DefaultSchemaName — tables without an explicit schema are
// qualified so that "orders" and "public.orders" produce the same tag.
func TablesToTags(projectId, dsAlias string, tables module_model.TablesInQuery, schemaPrefix string) []string {
	if len(tables.Tables) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(tables.Tables))
	tags := make([]string, 0, len(tables.Tables))
	for _, t := range tables.Tables {
		name := NormalizeTableName(t.TableName)
		if name == "" {
			continue
		}
		if !strings.Contains(name, ".") && schemaPrefix != "" {
			name = NormalizeTableName(schemaPrefix) + name
		}
		tag := TableTag(projectId, dsAlias, name)
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		tags = append(tags, tag)
	}
	return tags
}
