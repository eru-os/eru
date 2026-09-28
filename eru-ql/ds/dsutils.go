package ds

import (
	"strings"

	"github.com/antlr4-go/antlr/v4"
	parser "github.com/eru-os/eru/eru-ql/ds/parser"
	"github.com/eru-os/eru/eru-ql/module_model"
)

func GetSqlMaker(dbName string) SqlMakerI {
	switch dbName {
	case "mysql":
		return new(MysqlSqlMaker)
	case "postgres":
		return new(PostgresSqlMaker)
	case "mssql":
		return new(MssqlSqlMaker)
	case "iceberg":
		return new(IcebergSqlMaker)
	default:
		return nil
		//do nothing
	}
}

func GetDbType(dbName string) string {
	switch dbName {
	case "postgres", "mysql", "mssql", "iceberg":
		return "sql"
	case "mongo":
		return "mongo"
	default:
		return "unknown"
	}
}
func extractTableAliasNames(tree antlr.Tree, query string) (tablesInQuery module_model.TablesInQuery) {

	tablesInQuery = module_model.TablesInQuery{}
	switch ctx := tree.(type) {
	case *parser.Table_refContext:
		startIndex := 0
		stopIndex := 0
		tn := ""
		alias := ""
		for _, v := range ctx.GetChildren() {
			switch ctx1 := v.(type) {
			case *parser.Relation_exprContext:
				startIndex = ctx1.GetStart().GetStart()
				stopIndex = ctx1.GetStop().GetStop() + 1
				tn = ctx1.GetText()
			case *parser.Alias_clauseContext:
				stopIndex = ctx1.GetStop().GetStop() + 1
				alias = ctx1.GetText()

			default:
				for i := 0; i < tree.GetChildCount(); i++ {
					childTablesInQuery := extractTableAliasNames(tree.GetChild(i), query)
					for _, v := range childTablesInQuery.Tables {
						aliasFound := false
						for _, vv := range tablesInQuery.Tables {
							if vv.TableKey == v.TableKey {
								aliasFound = true
								break
							}
						}
						if !aliasFound {
							tablesInQuery.Tables = append(tablesInQuery.Tables, v)
						}
					}
				}
			}
		}
		if tn != "" {
			if alias == "" {
				alias = tn
			}
			tmpStopIndex := stopIndex + 5
			if tmpStopIndex > len(query) {
				tmpStopIndex = len(query)
			}

			tablesInQuery.Tables = append(tablesInQuery.Tables, module_model.TableInQuery{AliasName: alias, TableName: tn, TableKey: query[startIndex:stopIndex], TableKeyPrefix: query[startIndex-5 : startIndex], TableKeySuffix: query[stopIndex:tmpStopIndex]})
		}
	default:
		for i := 0; i < tree.GetChildCount(); i++ {
			childTablesInQuery := extractTableAliasNames(tree.GetChild(i), query)
			for _, v := range childTablesInQuery.Tables {
				aliasFound := false
				for _, vv := range tablesInQuery.Tables {
					if vv.TableKey == v.TableKey {
						aliasFound = true
						break
					}
				}
				if !aliasFound {
					tablesInQuery.Tables = append(tablesInQuery.Tables, v)
				}
			}
		}
	}
	return
}
func extractAliasNames(tree antlr.Tree) (aliases []string) {
	switch ctx := tree.(type) {
	case *parser.Common_table_exprContext:
		aliases = append(aliases, strings.Split(ctx.GetText(), "as(")[0])
	default:
		for i := 0; i < tree.GetChildCount(); i++ {
			aliases = append(aliases, extractAliasNames(tree.GetChild(i))...)
		}
	}
	return aliases
}
func extractTableNames(tree antlr.Tree) (tableNames []string) {
	switch ctx := tree.(type) {
	case *parser.Relation_exprContext:
		tableNames = append(tableNames, ctx.GetText())
	default:
		for i := 0; i < tree.GetChildCount(); i++ {
			tableNames = append(tableNames, extractTableNames(tree.GetChild(i))...)
		}
	}
	return tableNames
}
