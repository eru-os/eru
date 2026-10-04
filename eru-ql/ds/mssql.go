package ds

import (
	"context"
	"errors"
	"fmt"
	"strings"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/eru-os/eru/eru-ql/module_model"
  "github.com/eru-os/eru/eru-ql/derived"
)

type MssqlSqlMaker struct {
	SqlMaker
}

func (mr *MssqlSqlMaker) MakeUpsertQuery(ctx context.Context, tableName string, insertCols []string, colsPlaceholder string, conflictCols []string, updateCols []string, action string, returningStr string) (string, error) {
	logs.WithContext(ctx).Debug("MakeUpsertQuery - Start (mssql)")
	if len(conflictCols) == 0 {
		return "", errors.New("upsertOn must include at least one column")
	}
	conflictSet := make(map[string]bool)
	for _, c := range conflictCols {
		conflictSet[strings.TrimSpace(c)] = true
	}
	trimmedInsertCols := make([]string, 0, len(insertCols))
	for _, c := range insertCols {
		cc := strings.TrimSpace(c)
		if cc != "" {
			trimmedInsertCols = append(trimmedInsertCols, cc)
		}
	}
	colList := strings.Join(trimmedInsertCols, ",")

	var onParts []string
	for _, c := range conflictCols {
		cc := strings.TrimSpace(c)
		onParts = append(onParts, fmt.Sprint("T.", cc, " = S.", cc))
	}
	onCond := strings.Join(onParts, " and ")

	whenMatched := ""
	if action != "nothing" {
		sets := buildUpsertSets(trimmedInsertCols, conflictSet, updateCols, func(col string) string {
			return fmt.Sprint("T.", col, " = S.", col)
		})
		if len(sets) > 0 {
			whenMatched = fmt.Sprint(" when matched then update set ", strings.Join(sets, ","))
		}
	}

	var srcRefs []string
	for _, c := range trimmedInsertCols {
		srcRefs = append(srcRefs, fmt.Sprint("S.", c))
	}
	whenNotMatched := fmt.Sprint(" when not matched then insert (", colList, ") values (", strings.Join(srcRefs, ","), ")")

	output := ""
	if returningStr != "" {
		trimmed := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(returningStr), "RETURNING"))
		trimmed = strings.TrimPrefix(trimmed, "returning")
		trimmed = strings.TrimSpace(trimmed)
		if trimmed != "" && trimmed != "*" {
			var outCols []string
			for _, c := range strings.Split(trimmed, ",") {
				cc := strings.TrimSpace(c)
				if cc != "" {
					outCols = append(outCols, fmt.Sprint("inserted.", cc))
				}
			}
			output = fmt.Sprint(" output ", strings.Join(outCols, ","))
		} else if trimmed == "*" {
			output = " output inserted.*"
		}
	}

	q := fmt.Sprint(
		"merge ", tableName, " with (holdlock) as T using (values ", colsPlaceholder, ") as S (", colList, ") ",
		"on ", onCond,
		whenMatched,
		whenNotMatched,
		output,
		";",
	)
	return q, nil
}

func (pr *MssqlSqlMaker) GetTableMetaDataSQL(ctx context.Context, tableName string) string {
	return strings.Replace(mssqlTableMetaDataSQL, "$$tableCondition$$", fmt.Sprint("and c.table_name = '", tableName, "'"), 1)
}
func (mr *MssqlSqlMaker) CreateConn(ctx context.Context, dataSource *module_model.DataSource) error {
	return errors.New("CreateConn not implemented")
}
func (mr *MssqlSqlMaker) CheckMe(ctx context.Context) {
	logs.WithContext(ctx).Info("I am MssqlSqlMaker changed  removed")
	mr.ChildChange = "changed by MysqlRead"
}

func (mr *MssqlSqlMaker) AddLimitSkipClause(ctx context.Context, query string, limit int, skip int, globalLimit int) (newQuery string) {
	logs.WithContext(ctx).Debug("AddLimitSkipClause - Start")
	if limit == 0 {
		limit = globalLimit
	}

	if skip == 0 {
		if mr.DistinctResults {
			newQuery = strings.Replace(query, "distinct ", fmt.Sprint("distinct top ", limit, " "), 1)
		} else {
			newQuery = strings.Replace(query, "select ", fmt.Sprint("select top ", limit), 1)
		}

	} else {
		orderClause := ""
		if mr.SortClause == "" {
			orderClause = " order by (select null) "
		}
		newQuery = fmt.Sprint(query, orderClause, " offset ", skip, " rows fetch next ", limit, " rows only")
	}
	return newQuery
}

func (mr *MssqlSqlMaker) getDataTypeMapping(ctx context.Context, dataType string) string {
	logs.WithContext(ctx).Debug("getDataTypeMapping - Start")
	if mssqlDataTypeMapping[dataType] == "" {
		return "NotSupported"
	} else {
		return mssqlDataTypeMapping[dataType]
	}
}
func (mr *MssqlSqlMaker) getErutoDBDataTypeMapping(ctx context.Context, dataType string) string {
	logs.WithContext(ctx).Debug("getErutoDBDataTypeMapping - Start")
	return dataType
}

var mssqlTableMetaDataSQL = ""
var mssqlDataTypeMapping = map[string]string{}

func (mr *MssqlSqlMaker) GetCalcDialect(ctx context.Context) derived.Dialect {
	return derived.MssqlDialect()
}
