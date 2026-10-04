package ql

import (
	"context"
	"errors"
	"fmt"
	"strings"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/eru-os/eru/eru-ql/derived"
	"github.com/eru-os/eru/eru-ql/ds"
	"github.com/eru-os/eru/eru-ql/module_model"
	"github.com/eru-os/eru/eru-ql/module_store"
)

// securedSourceFor hands the compiler the FROM-clause source for a hop target
// with that table's own row-level security rule already applied, built the same
// way secureSQL builds it. A hop can never read rows the caller could not have
// selected directly.
func (sqlObj *SQLObjectQ) securedSourceFor(ctx context.Context, datasource *module_model.DataSource, s module_store.ModuleStoreI) derived.SecuredSource {
	return securedSourceFor(ctx, sqlObj.ProjectId, sqlObj.TenantId, sqlObj.FinalVariables, datasource, s)
}

func securedSourceFor(ctx context.Context, projectId string, tenantId string, vars map[string]interface{}, datasource *module_model.DataSource, s module_store.ModuleStoreI) derived.SecuredSource {
	return func(table string) (source string, denied bool, err error) {
		rule, ruleJoins, rErr := getTableSecurityRule(ctx, projectId, tenantId, datasource.DbAlias, table, s, "query", vars, table)
		if rErr != nil {
			if strings.HasPrefix(rErr.Error(), "TableSecurityRule not defined for") {
				return table, false, nil
			}
			if strings.Contains(rErr.Error(), "access denied for table") {
				return "", true, nil
			}
			return "", false, rErr
		}
		if strings.TrimSpace(rule) == "" {
			return table, false, nil
		}
		q := fmt.Sprint("select ", table, ".* from ", table)
		for _, ruleJoin := range ruleJoins {
			tj, jErr := datasource.GetTableJoins(ctx, table, ruleJoin, make(map[string]string))
			if jErr != nil {
				return "", false, jErr
			}
			onClause, oErr := processMapVariable(ctx, tj.GetOnClause(ctx), vars)
			if oErr != nil {
				return "", false, oErr
			}
			oc, _ := processWhereClause(ctx, onClause, "", table, true, false, nil)
			q = fmt.Sprint(q, " left join ", ruleJoin, " on ", oc)
		}
		q = fmt.Sprint(q, " where ", rule)
		return fmt.Sprint("(", q, ")"), false, nil
	}
}

// derivedExprResolver returns the compiled expression for a derived field on a
// table, if the name is one. Used to substitute a derived field into the inner
// WHERE and ORDER BY, so the database filters and sorts on the expression
// itself rather than requiring the caller to wrap the query.
type derivedExprResolver func(colName string) (expr string, ok bool)

// newDerivedExprResolver builds the resolver for one table. A compile failure is
// recorded on the object rather than returned, because the clause walkers it
// feeds have no way to carry an error; MakeQuery reports it.
func (sqlObj *SQLObjectQ) newDerivedExprResolver(ctx context.Context, table string, alias string, datasource *module_model.DataSource, sqlMaker ds.SqlMakerI, s module_store.ModuleStoreI) derivedExprResolver {
	return func(colName string) (string, bool) {
		if _, isRealColumn := datasource.SchemaTables[table][colName]; isRealColumn {
			return "", false
		}
		df, isDerived := datasource.DerivedFields[table][colName]
		if !isDerived {
			return "", false
		}
		compiled, err := sqlObj.compileDerivedField(ctx, table, derived.CompileRequest{
			Schema: derived.Schema{
				Tables:  datasource.SchemaTables,
				Derived: datasource.DerivedFields,
				Joins:   datasource.DerivedHopLookup(ctx),
			},
			Table:          table,
			Field:          df,
			Dialect:        sqlMaker.GetCalcDialect(ctx),
			OuterAlias:     alias,
			AllowHops:      true,
			MaxInlineDepth: sqlObj.derivedLimits(ctx, s).MaxInlineDepth,
			Security:       sqlObj.securedSourceFor(ctx, datasource, s),
		}, s)
		if err != nil {
			if sqlObj.derivedErr == "" {
				sqlObj.derivedErr = err.Error()
			}
			return "", false
		}
		return compiled.Expr, true
	}
}

// resolveDerived is the entry point the clause walkers use.
func (sqlObj *SQLObjectQ) resolveDerived(colName string) (string, bool) {
	if sqlObj.derivedResolve == nil {
		return "", false
	}
	return sqlObj.derivedResolve(colName)
}

// compileDerivedField compiles one derived field and enforces the per-query
// caps as it goes.
func (sqlObj *SQLObjectQ) compileDerivedField(ctx context.Context, table string, field derived.CompileRequest, s module_store.ModuleStoreI) (derived.CompiledField, error) {
	compiled, err := derived.Compile(field)
	if err != nil {
		return compiled, err
	}

	limits := sqlObj.derivedLimits(ctx, s)
	sqlObj.derivedCount = sqlObj.derivedCount + 1
	sqlObj.derivedAggHops = sqlObj.derivedAggHops + compiled.AggregateHops

	if sqlObj.derivedCount > limits.MaxFieldsPerQuery {
		return compiled, errors.New(fmt.Sprint("this query selects ", sqlObj.derivedCount, " derived fields, the limit is ", limits.MaxFieldsPerQuery))
	}
	if sqlObj.derivedAggHops > limits.MaxAggregateHops {
		return compiled, errors.New(fmt.Sprint("this query needs ", sqlObj.derivedAggHops, " aggregate subqueries, the limit is ", limits.MaxAggregateHops, " - select fewer aggregated derived fields at once"))
	}
	for _, w := range compiled.Warnings {
		logs.WithContext(ctx).Warn(fmt.Sprint(table, ".", field.Field.ColName, ": ", w))
	}
	return compiled, nil
}

func (sqlObj *SQLObjectQ) derivedLimits(ctx context.Context, s module_store.ModuleStoreI) module_model.DerivedFieldLimits {
	if sqlObj.derivedLimitsCache != nil {
		return *sqlObj.derivedLimitsCache
	}
	limits := module_model.DefaultDerivedFieldLimits()
	if s != nil {
		if ps, err := s.GetProjectSettingsObject(ctx, sqlObj.ProjectId); err == nil {
			limits = ps.DerivedFieldLimits.WithDefaults()
		}
	}
	sqlObj.derivedLimitsCache = &limits
	return limits
}
