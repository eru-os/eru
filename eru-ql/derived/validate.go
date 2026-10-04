package derived

import (
	"fmt"
	"strings"

	common_types "github.com/eru-os/eru/eru-ql/common_types"
)

// ValidationResult is what the validate endpoint returns: exactly what the
// database will run, plus everything we resolved on the way there.
type ValidationResult struct {
	Formula       string                            `json:"formula"`
	Expr          string                            `json:"expr"`
	ResultType    string                            `json:"calc_result_type"`
	Deps          []string                          `json:"calc_deps"`
	RefTables     []string                          `json:"ref_tables"`
	HasAggregate  bool                              `json:"calc_has_aggregate"`
	AggregateHops int                               `json:"aggregate_hops"`
	Warnings      []string                          `json:"warnings"`
	AST           *common_types.CalcNode            `json:"calc_ast"`
	Field         common_types.DerivedFieldMetaData `json:"field"`
}

// Validate re-parses the formula, resolves it against the live schema and emits
// the SQL, without persisting anything. Everything a client sends beyond the
// formula itself is treated as a claim and re-derived here.
func Validate(req CompileRequest) (ValidationResult, error) {
	if err := ValidateName(req.Field.ColName); err != nil {
		return ValidationResult{}, err
	}
	if _, ok := req.Schema.Tables[req.Table]; !ok {
		return ValidationResult{}, fmt.Errorf("table %s is not managed by this datasource", req.Table)
	}
	if _, ok := req.Schema.Tables[req.Table][req.Field.ColName]; ok {
		return ValidationResult{}, fmt.Errorf("derived field %s collides with an existing column on %s", req.Field.ColName, req.Table)
	}

	node, err := Parse(req.Field.Formula)
	if err != nil {
		return ValidationResult{}, err
	}

	req.SkipCache = true
	req.ValidateOnly = true
	compiled, err := Compile(req)
	if err != nil {
		return ValidationResult{}, err
	}

	field := req.Field
	field.IsCalc = true
	field.IsNullable = true
	field.CalcMode = common_types.CalcModeVirtual
	field.CalcAST = node
	field.CalcDeps = compiled.Deps
	field.CalcRefs = compiled.Refs
	field.CalcJoins = nil
	for _, hop := range compiled.Hops {
		field.CalcJoins = append(field.CalcJoins, common_types.CalcJoin{
			TargetTable: hop.TargetTable,
			OwnCols:     hop.OwnCols,
			TargetCols:  hop.TargetCols,
			Cardinality: hop.Cardinality,
		})
	}
	field.CalcHasAggregate = compiled.HasAggregate
	field.CalcResultType = compiled.ResultType
	if strings.TrimSpace(field.DataType) == "" {
		// nothing declared, so record what the formula produces
		field.DataType = compiled.ResultType
	}
	field.Invalid = false
	field.InvalidReason = ""
	field.TblName = req.Table
	if parts := strings.SplitN(req.Table, ".", 2); len(parts) == 2 {
		field.TblSchema = parts[0]
		field.TblName = parts[1]
	}

	return ValidationResult{
		Formula:       req.Field.Formula,
		Expr:          compiled.Expr,
		ResultType:    compiled.ResultType,
		Deps:          compiled.Deps,
		RefTables:     compiled.RefTables,
		HasAggregate:  compiled.HasAggregate,
		AggregateHops: compiled.AggregateHops,
		Warnings:      compiled.Warnings,
		AST:           node,
		Field:         field,
	}, nil
}

// ValidateName is the only thing standing between a field name and an identifier
// in emitted SQL.
func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("derived field name is mandatory")
	}
	if !isIdentStart(name[0]) || name[0] < 'a' || name[0] > 'z' {
		if name[0] != '_' {
			return fmt.Errorf("derived field name %s is invalid - expected %s", name, NamePattern)
		}
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c == '_' || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			continue
		}
		return fmt.Errorf("derived field name %s is invalid - expected %s", name, NamePattern)
	}
	return nil
}

const NamePattern = "^[a-z_][a-z0-9_]*$"

// Revalidate re-resolves a stored derived field against a refreshed schema and
// reports why it is broken, if it is. Used after the schema moves underneath.
func Revalidate(req CompileRequest) (invalidReason string) {
	if _, err := Compile(CompileRequest{
		Schema:       req.Schema,
		Table:        req.Table,
		Field:        clearInvalid(req.Field),
		Dialect:      req.Dialect,
		OuterAlias:   req.OuterAlias,
		AllowHops:    req.AllowHops,
		Security:     req.Security,
		ValidateOnly: req.ValidateOnly,
		SkipCache:    true,
	}); err != nil {
		return err.Error()
	}
	return ""
}

func clearInvalid(f common_types.DerivedFieldMetaData) common_types.DerivedFieldMetaData {
	f.Invalid = false
	f.InvalidReason = ""
	return f
}
