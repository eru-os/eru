package derived

import (
	"fmt"
	"strings"

	common_types "github.com/eru-os/eru/eru-ql/common_types"
)

// Hop describes one resolved relationship step. Keys always come from the
// datasource's configured joins, never from a client payload.
type Hop struct {
	TargetTable string
	OwnCols     []string
	TargetCols  []string
	Cardinality string
}

const (
	CardinalityOne  = "one"
	CardinalityMany = "many"
)

// HopLookup resolves the configured join between two tables. The caller supplies
// it so this package stays free of the datasource model.
type HopLookup func(ownTable string, targetTable string) (Hop, error)

// Schema is the metadata the compiler is allowed to resolve against.
type Schema struct {
	Tables  map[string]map[string]common_types.TableColsMetaData
	Derived map[string]map[string]common_types.DerivedFieldMetaData
	Joins   HopLookup
}

type resolvedRef struct {
	node     *Node
	hop      *Hop
	table    string
	column   string
	dataType string
	valType  string
	// derivedRef names another derived field on the same table, to be inlined
	derivedRef string
}

func (s Schema) column(table string, column string) (common_types.TableColsMetaData, bool) {
	col, ok := s.Tables[table][column]
	return col, ok
}

// resolveRef resolves one {…} reference against the schema. Every identifier
// that reaches SQL comes out of this lookup.
func (c *compiler) resolveRef(n *Node) (resolvedRef, error) {
	parts := strings.Split(n.Path, ".")
	switch len(parts) {
	case 1:
		return c.resolveOwnRef(n, parts[0])
	case 2:
		return c.resolveHopRef(n, parts[0], parts[1])
	}
	return resolvedRef{}, fmt.Errorf("field reference {%s} reaches more than one table away - only an immediately related table is allowed", n.Path)
}

func (c *compiler) resolveOwnRef(n *Node, column string) (resolvedRef, error) {
	if col, ok := c.schema.column(c.table, column); ok {
		if masking := strings.ToLower(col.ColumnMasking.MaskingType); masking == common_types.ColumnMaskingEncrypt || masking == common_types.ColumnMaskingHash {
			return resolvedRef{}, fmt.Errorf("column %s on %s is %sed - a derived field cannot be built on it", column, c.table, masking)
		}
		return resolvedRef{node: n, table: c.table, column: column, dataType: col.OwnDataType, valType: ValueTypeOf(pickType(col))}, nil
	}
	if _, ok := c.schema.Derived[c.table][column]; ok {
		if column == c.field && len(c.inlining) == 0 {
			return resolvedRef{}, fmt.Errorf("derived field %s refers to itself", column)
		}
		return resolvedRef{node: n, table: c.table, column: column, derivedRef: column}, nil
	}
	return resolvedRef{}, fmt.Errorf("column %s does not exist on %s", column, c.table)
}

func (c *compiler) resolveHopRef(n *Node, target string, column string) (resolvedRef, error) {
	if !c.allowHops {
		return resolvedRef{}, fmt.Errorf("field reference {%s} reaches another table - cross-table references are not supported yet", n.Path)
	}
	if c.schema.Joins == nil {
		return resolvedRef{}, fmt.Errorf("field reference {%s} reaches another table but no join lookup is configured", n.Path)
	}
	targetTable := c.qualify(target)
	if _, ok := c.schema.Tables[targetTable]; !ok {
		return resolvedRef{}, fmt.Errorf("table %s is not managed by this datasource", targetTable)
	}
	hop, err := c.schema.Joins(c.table, targetTable)
	if err != nil {
		return resolvedRef{}, fmt.Errorf("no join is defined between %s and %s - add the join before using {%s}", c.table, targetTable, n.Path)
	}
	col, ok := c.schema.column(targetTable, column)
	if !ok {
		return resolvedRef{}, fmt.Errorf("column %s does not exist on %s", column, targetTable)
	}
	if masking := strings.ToLower(col.ColumnMasking.MaskingType); masking == common_types.ColumnMaskingEncrypt || masking == common_types.ColumnMaskingHash {
		return resolvedRef{}, fmt.Errorf("column %s on %s is %sed - a derived field cannot be built on it", column, targetTable, masking)
	}
	hop.TargetTable = targetTable
	hop.Cardinality = c.hopCardinality(hop)
	c.refTables[targetTable] = true
	c.recordHop(hop)
	return resolvedRef{node: n, hop: &hop, table: targetTable, column: column, dataType: col.OwnDataType, valType: ValueTypeOf(pickType(col))}, nil
}

func (c *compiler) qualify(table string) string {
	if strings.Contains(table, ".") {
		return table
	}
	if i := strings.LastIndex(c.table, "."); i > 0 {
		return fmt.Sprint(c.table[:i], ".", table)
	}
	return table
}

func pickType(col common_types.TableColsMetaData) string {
	if col.OwnDataType != "" {
		return col.OwnDataType
	}
	return col.DataType
}
