package ds

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"strings"

	"github.com/antlr4-go/antlr/v4"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	common_types "github.com/eru-os/eru/eru-ql/common_types"
	parser "github.com/eru-os/eru/eru-ql/ds/parser"
  "github.com/eru-os/eru/eru-ql/derived"

	//eru_utils "github.com/eru-os/eru/eru-utils"

	"github.com/eru-os/eru/eru-ql/module_model"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

var dbBlockedWords = []string{}
var dbBlockedRegex = []string{}

type PostgresSqlMaker struct {
	SqlMaker
}

// Listener to traverse the parse tree
type Listener struct {
	*parser.BasePostgreSQLParserListener
	Columns       []string
	inFinalSelect bool // Flag to track if we're in the final SELECT
}

/* func (l *Listener) EnterColumnref(ctx *parser.ColumnrefContext) {
	// Check if this is the final SELECT (not part of a CTE)
	if ctx.GetParent() != nil {
		logs.WithContext(context.Background()).Info(fmt.Sprint(ctx.GetText()))
		 if _, ok := ctx.GetParent().(*parser.Relation_expr_opt_aliasContext); ok {
			l.inFinalSelect = true

		}
	}
} */

//func (l *Listener) EnterEveryRule(ctx antlr.ParserRuleContext) {
//	fmt.Print(ctx.GetRuleIndex(), " : ", ctx.GetText())
//}

func (pr *PostgresSqlMaker) GetBlockedWords() []string {
	return append(blockedWords, dbBlockedWords...)
}
func (pr *PostgresSqlMaker) GetBlockedRegex() []string {
	return append(blockedRegex, dbBlockedRegex...)
}
func (pr *PostgresSqlMaker) GetMakeJsonArrayFn() (string, error) {
	return " ?| array ", nil
}
func (pr *PostgresSqlMaker) GetMakeJsonArrayFnStr() (string, error) {
	return " ? ", nil
}

func (pr *PostgresSqlMaker) ExtractTableNames(ctx context.Context, query string) (resTablesInQuery module_model.TablesInQuery) {
	logs.WithContext(ctx).Debug("ExtractTableNames - Start")
	// Create an input stream
	//query = "select * from abc a left join xyz b on 1=1"
	is := antlr.NewInputStream(query)

	// Create the lexer and token stream
	lexer := parser.NewPostgreSQLLexer(is)
	stream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)

	// Create the parser
	p := parser.NewPostgreSQLParser(stream)

	// Parse the input (starting rule depends on your grammar, e.g., statement)
	tree := p.Root()
	//fmt.Println(tree.ToStringTree(nil, p))

	//tables := extractTableNames(tree)
	aliases := extractAliasNames(tree)
	tablesInQuery := extractTableAliasNames(tree, query)
	reaTablesInQuery := module_model.TablesInQuery{}
	// Walk the parse tree with a custom listener
	//listener := &Listener{}
	//antlr.ParseTreeWalkerDefault.Walk(listener, tree)

	//var filteredArray []string
	// Only keep table names that don't exist in aliases
	for _, table := range tablesInQuery.Tables {
		aliasFound := false
		for _, alias := range aliases {
			if strings.TrimSpace(table.TableName) == strings.TrimSpace(alias) {
				aliasFound = true
				break
			}
		}
		if !aliasFound {
			reaTablesInQuery.Tables = append(reaTablesInQuery.Tables, table)
		}
	}
	//tables = eru_utils.UniqueStrings(filteredArray)
	//logs.WithContext(ctx).Info(fmt.Sprint("Tables : ", tables))
	return reaTablesInQuery
}
func (pr *PostgresSqlMaker) DefaultSchemaName() string {
	return "public."
}

// ExtractDMLTargetTables returns ONLY the table being mutated by an
// INSERT/UPDATE/DELETE — never the tables in subqueries or FROM clauses.
// Used for cache invalidation, where we only want to invalidate entries
// that actually depend on the row(s) that changed.
func (pr *PostgresSqlMaker) ExtractDMLTargetTables(ctx context.Context, query string) []string {
	logs.WithContext(ctx).Debug("ExtractDMLTargetTables - Start")
	is := antlr.NewInputStream(query)
	lexer := parser.NewPostgreSQLLexer(is)
	stream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	p := parser.NewPostgreSQLParser(stream)
	tree := p.Root()

	seen := map[string]struct{}{}
	var out []string
	collectDMLTargets(tree, seen, &out)
	return out
}

func collectDMLTargets(tree antlr.Tree, seen map[string]struct{}, out *[]string) {
	switch tree.(type) {
	case *parser.Insert_targetContext, *parser.Relation_expr_opt_aliasContext:
		if name := firstQualifiedName(tree); name != "" {
			if _, ok := seen[name]; !ok {
				seen[name] = struct{}{}
				*out = append(*out, name)
			}
		}
		return
	}
	for i := 0; i < tree.GetChildCount(); i++ {
		collectDMLTargets(tree.GetChild(i), seen, out)
	}
}

func firstQualifiedName(tree antlr.Tree) string {
	if q, ok := tree.(*parser.Qualified_nameContext); ok {
		return q.GetText()
	}
	for i := 0; i < tree.GetChildCount(); i++ {
		if name := firstQualifiedName(tree.GetChild(i)); name != "" {
			return name
		}
	}
	return ""
}

func (pr *PostgresSqlMaker) GetPreparedQueryPlaceholder(ctx context.Context, rowCount int, colCount int, single bool) string {
	logs.WithContext(ctx).Debug("GetPreparedQueryPlaceholder - Start")
	var rowArray []string
	startNo := 0
	if single {
		return fmt.Sprint("$", colCount)
	}
	for r := 1; r <= rowCount; r++ {
		var colArray []string
		for c := 1; c <= colCount; c++ {
			startNo++
			colArray = append(colArray, fmt.Sprint("$", startNo))
		}
		rowArray = append(rowArray, fmt.Sprint("(", strings.Join(colArray, " , "), ")"))
	}
	return strings.Join(rowArray, " , ")
}

func (pr *PostgresSqlMaker) MakeUpsertQuery(ctx context.Context, tableName string, insertCols []string, colsPlaceholder string, conflictCols []string, updateCols []string, action string, returningStr string) (string, error) {
	logs.WithContext(ctx).Debug("MakeUpsertQuery - Start (postgres)")
	if len(conflictCols) == 0 {
		return "", errors.New("upsertOn must include at least one column")
	}
	conflictSet := make(map[string]bool)
	for _, c := range conflictCols {
		conflictSet[strings.TrimSpace(c)] = true
	}
	base := fmt.Sprint("insert into ", tableName, " (", strings.Join(insertCols, ","), ") values ", colsPlaceholder)
	conflictClause := fmt.Sprint(" on conflict (", strings.Join(conflictCols, ","), ")")
	if action == "nothing" {
		return fmt.Sprint(base, conflictClause, " do nothing", returningStr), nil
	}
	sets := buildUpsertSets(insertCols, conflictSet, updateCols, func(col string) string {
		return fmt.Sprint(col, " = excluded.", col)
	})
	if len(sets) == 0 {
		return fmt.Sprint(base, conflictClause, " do nothing", returningStr), nil
	}
	return fmt.Sprint(base, conflictClause, " do update set ", strings.Join(sets, ","), returningStr), nil
}

func (pr *PostgresSqlMaker) GetTableMetaDataSQL(ctx context.Context, tableName string) string {
	logs.WithContext(ctx).Debug("GetTableMetaDataSQL - Start")
	stringToReplace := ""
	if tableName != "" {
		stringToReplace = fmt.Sprint("and c.table_name = '", tableName, "'")
	}
	return strings.Replace(postgresTableMetaDataSQL, "$$tableCondition$$", stringToReplace, 1)
}

func (pr *PostgresSqlMaker) MakeCreateTableSQL(ctx context.Context, tableName string, tableObj map[string]common_types.TableColsMetaData) (string, error) {
	logs.WithContext(ctx).Debug("MakeCreateTableSQL - Start")
	var cols []string
	var fks []string
	pkCon := make(map[string][]string)
	uqCon := make(map[string][]string)
	pkConName := fmt.Sprint("pk_", strings.Replace(tableName, ".", "___", 1))
	for _, v := range tableObj {
		dt := "serial"
		//pk := ""
		//uk := ""
		nl := ""
		if !v.IsNullable {
			nl = " not null "
		}
		if v.IsUnique && !v.PrimaryKey {
			uqCon[v.UqConstraintName] = append(uqCon[v.UqConstraintName], v.ColName)
		}
		if !v.PrimaryKey {
			dt = pr.getErutoDBDataTypeMapping(ctx, v.OwnDataType)
			if dt == "NotSupported" {
				return "", errors.New(fmt.Sprint("Unsupported Datatype : ", v.OwnDataType))
			}
		} else {
			pkCon[pkConName] = append(pkCon[pkConName], v.ColName)
			//pk = " primary key "
			nl = ""
		}

		switch dt {
		case "numeric":
			dt = fmt.Sprint(dt, " (", v.NumericPrecision, ")")
		case "character", "character varying":
			dt = fmt.Sprint(dt, " (", v.CharMaxLength, ")")
		case "timestamp without time zone", "timestamp with time zone":
			dt = strings.Replace(dt, "timestamp", fmt.Sprint("timestamp (", v.DatetimePrecision, ")"), 1)
		case "time with time zone":
			dt = strings.Replace(dt, "time", fmt.Sprint("time (", v.DatetimePrecision, ")"), 1)
		}

		if v.FkTblName != "" {
			fks = append(fks, fmt.Sprint("constraint fk_", v.TblName, v.ColName, " foreign key (", v.ColName, ") references ", v.FkTblSchema, ".", v.FkTblName, "(", v.FkColName, ")"))
		}
		cols = append(cols, fmt.Sprint(v.ColName, " ", dt, nl))
	}
	var pk []string
	for k, v := range pkCon {
		pk = append(pk, fmt.Sprint("constraint ", k, " primary key (", strings.Join(v, " , "), ")"))
	}
	if len(pk) > 0 {
		cols = append(cols, strings.Join(pk, " , "))
	}
	var uq []string
	for k, v := range uqCon {
		uq = append(uq, fmt.Sprint("constraint ", k, " unique (", strings.Join(v, " , "), ")"))
	}
	if len(uq) > 0 {
		cols = append(cols, strings.Join(uq, " , "))
	}
	if len(fks) > 0 {
		cols = append(cols, strings.Join(fks, " , "))
	}

	query := fmt.Sprint("create table ", tableName, " (", strings.Join(cols, " , "), " )")
	return query, nil
}

func dialPostgres(ctx context.Context, cfg module_model.DbConfig) (*sqlx.DB, error) {
	connString := fmt.Sprint("postgres://", cfg.User, ":", cfg.Password, "@", cfg.Host, ":", cfg.Port, "/", cfg.DefaultDB, "?sslmode=disable")
	db, err := sqlx.Open("postgres", connString)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
	if _, err = db.Queryx("select 1"); err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
	return db, nil
}

func (pr *PostgresSqlMaker) CreateConn(ctx context.Context, dataSource *module_model.DataSource) error {
	logs.WithContext(ctx).Debug("CreateConn - Start")
	db, err := dialPostgres(ctx, dataSource.DbConfig)
	if err != nil {
		dataSource.ConStatus = false
		return err
	}
	logs.WithContext(ctx).Info("dummy query success - setting con as true")
	dataSource.Con = db
	dataSource.ConStatus = true

	for i := range dataSource.ReadDbConfigs {
		if dataSource.ReadPolicy.Disabled || dataSource.ReadDbConfigs[i].Disabled {
			if dataSource.ReadDbConfigs[i].Con != nil {
				_ = dataSource.ReadDbConfigs[i].Con.Close()
				dataSource.ReadDbConfigs[i].Con = nil
			}
			dataSource.ReadDbConfigs[i].ConStatus = false
			continue
		}
		if rerr := pr.ConnectReadReplica(ctx, dataSource, i); rerr != nil {
			logs.WithContext(ctx).Error(fmt.Sprint("read replica ", dataSource.ReadDbConfigs[i].Name, " connect failed: ", rerr.Error()))
		}
	}
	return nil
}

func (pr *PostgresSqlMaker) ConnectReadReplica(ctx context.Context, dataSource *module_model.DataSource, idx int) error {
	if idx < 0 || idx >= len(dataSource.ReadDbConfigs) {
		return errors.New("read replica index out of range")
	}
	replica := dataSource.ReadDbConfigs[idx]
	cfg := replica.DbConfig
	if replica.ResolvedDbConfig.Host != "" {
		cfg = replica.ResolvedDbConfig
	}
	db, err := dialPostgres(ctx, cfg)
	if err != nil {
		replica.ConStatus = false
		replica.Con = nil
		return err
	}
	replica.Con = db
	replica.ConStatus = true
	return nil
}

func (pr *PostgresSqlMaker) CheckMe(ctx context.Context) {
	logs.WithContext(ctx).Info("I am PostgresSqlMaker")
	pr.ChildChange = "changed by PostgresSqlMaker"
}
func (pr *PostgresSqlMaker) MakeJsonColumn(jsonField string, jsonKey string) string {
	return fmt.Sprint(jsonField, "->>'", jsonKey, "'")
}
func (pr *PostgresSqlMaker) AddLimitSkipClause(ctx context.Context, query string, limit int, skip int, globalLimit int) (newQuery string) {
	logs.WithContext(ctx).Debug("AddLimitSkipClause - Start")
	if limit > 0 {
		newQuery = fmt.Sprint(query, " limit ", limit)
	} else {
		newQuery = fmt.Sprint(query, " limit ", globalLimit)
	}
	if skip > 0 {
		newQuery = fmt.Sprint(newQuery, " offset ", skip)
	}
	return newQuery
}

func (pr *PostgresSqlMaker) getDataTypeMapping(ctx context.Context, dataType string) string {
	logs.WithContext(ctx).Debug("getDataTypeMapping - Start")
	if postgresDataTypeMapping[strings.ToLower(dataType)] == "" {
		return "NotSupported"
	} else {
		return postgresDataTypeMapping[strings.ToLower(dataType)]
	}
}

func (pr *PostgresSqlMaker) getErutoDBDataTypeMapping(ctx context.Context, dataType string) string {
	logs.WithContext(ctx).Debug("getErutoDBDataTypeMapping - Start")
	if postgresErutoDBDataTypeMapping[strings.ToLower(dataType)] == "" {
		return "NotSupported"
	} else {
		return postgresErutoDBDataTypeMapping[strings.ToLower(dataType)]
	}
}

func (pr *PostgresSqlMaker) SaveTable(ctx context.Context, tableName string, tableStructure common_types.TableStructure, isEdit bool, dataSource *module_model.DataSource) (err error) {
	logs.WithContext(ctx).Debug("SaveTable - Start")
	query, err := pr.MakeCreateTableSQL(ctx, tableName, tableStructure.NewColumns)
	if err != nil {
		return err
	}
	_, err = pr.ExecutePreparedQuery(ctx, query, dataSource)
	if err != nil {
		return err
	}
	return nil
}

const postgresTableMetaDataSQL = `select CAST(c.table_schema as VARCHAR) TblSchema,
       CAST(c.table_name as VARCHAR) TblName,
       CAST(c.column_name as VARCHAR) ColName,
       CAST(c.data_type as VARCHAR) DataType,
	   '' OwnDataType,
	   CAST(CASE WHEN pk.constraint_type is not null then 'true' else 'false' end as VARCHAR) PrimaryKey,
		CAST(CASE WHEN uq.constraint_type is not null then 'true' else 'false' end as VARCHAR) IsUnique,
		CAST(COALESCE(pk.constraint_name,'') as VARCHAR) PkConstraintName,
		CAST(COALESCE(uq.constraint_name,'') as VARCHAR) UqConstraintName,
	   CAST(CASE WHEN CAST(c.is_nullable as VARCHAR) = 'YES' THEN 'true' ELSE 'false' END as VARCHAR) IsNullable,       
       c.ordinal_position ColPosition,
       CAST(SPLIT_PART(REPLACE(COALESCE(c.column_default,''),'''',''), '::', 1) as VARCHAR) DefaultValue,
       CAST(CASE WHEN UPPER(c.column_default) like 'NEXTVAL%' then 'true' else 'false' end as VARCHAR) AutoIncrement,
       COALESCE(c.character_maximum_length,-1) CharMaxLength,
       COALESCE(c.numeric_precision,0)||','||COALESCE(c.numeric_scale,0) NumericPrecision,
       COALESCE(c.numeric_scale,0) NumericScale,
       COALESCE(c.datetime_precision,0) DatetimePrecision,
       CAST(COALESCE(fk.constraint_name,'') as VARCHAR) FkConstraintName,
       CAST(COALESCE(fk.delete_rule,'') as VARCHAR) FkDeleteRule,
       CAST(COALESCE(fk.foreign_table_schema,'')  as VARCHAR) FkTblSchema,
       CAST(COALESCE(fk.foreign_table_name,'') as VARCHAR) FkTblName,
       CAST(COALESCE(fk.foreign_column_name,'') as VARCHAR) FkColName
FROM information_schema.columns c
LEFT JOIN (select tc.constraint_type, tc.constraint_name , tc.table_schema, tc.table_name, kcu.column_name from information_schema.table_constraints tc
INNER JOIN information_schema.key_column_usage kcu 
            	ON tc.constraint_name = kcu.constraint_name AND tc.table_schema = kcu.table_schema) pk on pk.constraint_type='PRIMARY KEY' and pk.table_schema=c.table_schema
				and pk.table_name=c.table_name and pk.column_name=c.column_name
LEFT JOIN (select tc.constraint_type, tc.constraint_name , tc.table_schema, tc.table_name, kcu.column_name from information_schema.table_constraints tc
INNER JOIN information_schema.key_column_usage kcu 
            	ON tc.constraint_name = kcu.constraint_name AND tc.table_schema = kcu.table_schema) uq on uq.constraint_type='UNIQUE' and uq.table_schema=c.table_schema
				and uq.table_name=c.table_name and uq.column_name=c.column_name				
LEFT JOIN (SELECT tc.table_schema, tc.constraint_name, tc.table_name, kcu.column_name,
			ccu.table_schema foreign_table_schema, ccu.table_name foreign_table_name,
			ccu.column_name foreign_column_name, rc.delete_rule delete_rule
            FROM information_schema.table_constraints tc
            INNER JOIN information_schema.key_column_usage kcu 
            	ON tc.constraint_name = kcu.constraint_name AND tc.table_schema = kcu.table_schema
            INNER JOIN information_schema.constraint_column_usage ccu
				ON ccu.constraint_name = tc.constraint_name AND ccu.table_schema = tc.table_schema
			INNER JOIN  information_schema.referential_constraints rc
            	ON rc.constraint_name = tc.constraint_name AND rc.constraint_schema = tc.table_schema
			WHERE tc.constraint_type = 'FOREIGN KEY' ) fk
ON fk.table_name = c.table_name AND fk.column_name = c.column_name AND fk.table_schema = c.table_schema
WHERE  c.table_schema not in ('information_schema','pg_catalog') $$tableCondition$$
ORDER BY c.ordinal_position`

//erudevsh

var postgresDataTypeMapping = map[string]string{
	"smallint":                    "SmallInteger",
	"integer":                     "Integer",
	"int4":                        "Integer",
	"int8":                        "BigInteger",
	"int16":                       "Integer",
	"int32":                       "Integer",
	"int64":                       "Integer",
	"int":                         "Integer",
	"bigint":                      "BigInteger",
	"numeric":                     "Decimal",
	"real":                        "Float",
	"double precision":            "Float",
	"float8":                      "Float",
	"float4":                      "Float",
	"varchar":                     "Varchar",
	"character varying":           "Varchar",
	"character":                   "Char",
	"text":                        "String",
	"timestamp":                   "DateTime",
	"timestamp without time zone": "DateTime",
	"timestamp with time zone":    "DateTimeWithZone",
	"date":                        "Date",
	"time without time zone":      "Time",
	"time with time zone":         "TimeWithZone",
	"boolean":                     "Boolean",
	"bool":                        "Boolean",
	"json":                        "JSON",
	"jsonb":                       "JSON"}

var postgresErutoDBDataTypeMapping = map[string]string{
	"smallinteger":     "smallint",
	"integer":          "integer",
	"biginteger":       "bigint",
	"decimal":          "numeric",
	"float":            "double precision",
	"varchar":          "character varying",
	"char":             "character",
	"string":           "text",
	"datetime":         "timestamp without time zone",
	"datetimewithzone": "timestamp with time zone",
	"date":             "date",
	"time":             "time with time zone",
	"timewithzone":     "time with time zone",
	"boolean":          "boolean",
	"json":             "jsonb",
}

// RewriteDerivedFields replaces table-qualified references to derived fields
// with their compiled expressions, in place, by offset. It runs before
// secureSQL so the security wrapper sees the final table set - hop targets
// included - and so cache tags taken from the executed SQL come out right.
func (pr *PostgresSqlMaker) RewriteDerivedFields(ctx context.Context, req DerivedRewriteRequest) (string, error) {
	logs.WithContext(ctx).Debug("RewriteDerivedFields - Start")
	if len(req.DataSource.DerivedFields) == 0 {
		return req.Query, nil
	}

	is := antlr.NewInputStream(req.Query)
	lexer := parser.NewPostgreSQLLexer(is)
	stream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	p := parser.NewPostgreSQLParser(stream)
	tree := p.Root()

	refs := extractColumnRefs(tree)
	if len(refs) == 0 {
		return req.Query, nil
	}

	scope, err := derivedScope(ctx, tree, req.Query, pr)
	if err != nil {
		return "", err
	}

	dialect := pr.GetCalcDialect(ctx)
	limits := req.Limits.WithDefaults()
	fieldCount := 0
	aggHops := 0

	// Splice right-to-left so earlier offsets stay valid.
	sort.Slice(refs, func(i int, j int) bool { return refs[i].Start > refs[j].Start })

	query := req.Query
	for _, ref := range refs {
		table, resolved := scope[strings.ToLower(ref.Qualifier)]
		if !resolved {
			// a CTE, a derived table, or a qualifier we do not manage
			continue
		}
		if table == "" {
			return "", errors.New(fmt.Sprint("qualifier ", ref.Qualifier, " refers to more than one table in this statement - derived field ", ref.Qualifier, ".", ref.ColName, " cannot be resolved here"))
		}
		// a real column of the same name always wins
		if _, isColumn := req.DataSource.SchemaTables[table][ref.ColName]; isColumn {
			continue
		}
		df, isDerived := req.DataSource.DerivedFields[table][ref.ColName]
		if !isDerived {
			continue
		}

		compiled, cErr := derived.Compile(derived.CompileRequest{
			Schema: derived.Schema{
				Tables:  req.DataSource.SchemaTables,
				Derived: req.DataSource.DerivedFields,
				Joins:   req.DataSource.DerivedHopLookup(ctx),
			},
			Table:          table,
			Field:          df,
			Dialect:        dialect,
			OuterAlias:     ref.Qualifier,
			AllowHops:      true,
			MaxInlineDepth: limits.MaxInlineDepth,
			Security:       req.Security,
		})
		if cErr != nil {
			return "", cErr
		}

		fieldCount = fieldCount + 1
		aggHops = aggHops + compiled.AggregateHops
		if fieldCount > limits.MaxFieldsPerQuery {
			return "", errors.New(fmt.Sprint("this query references ", fieldCount, " derived fields, the limit is ", limits.MaxFieldsPerQuery))
		}
		if aggHops > limits.MaxAggregateHops {
			return "", errors.New(fmt.Sprint("this query needs ", aggHops, " aggregate subqueries, the limit is ", limits.MaxAggregateHops))
		}
		for _, w := range compiled.Warnings {
			logs.WithContext(ctx).Warn(fmt.Sprint(table, ".", ref.ColName, ": ", w))
		}

		if ref.Start < 0 || ref.Stop > len(query) || ref.Start > ref.Stop {
			return "", errors.New(fmt.Sprint("cannot splice derived field ", ref.Qualifier, ".", ref.ColName, " - offsets out of range"))
		}
		query = fmt.Sprint(query[:ref.Start], compiled.Expr, query[ref.Stop:])
	}

	if fieldCount > 0 {
		logs.WithContext(ctx).Info(fmt.Sprint("rewrote ", fieldCount, " derived field reference(s)"))
	}
	return query, nil
}

// derivedScope maps each alias in the statement to the schema-qualified table it
// names. An alias used for two different tables maps to "" so the caller fails
// loudly rather than resolving it against the wrong one; a CTE name is left out
// entirely so references through it are passed through untouched.
func derivedScope(ctx context.Context, tree antlr.Tree, query string, pr *PostgresSqlMaker) (map[string]string, error) {
	ctes := make(map[string]bool)
	for _, alias := range extractAliasNames(tree) {
		ctes[strings.ToLower(strings.TrimSpace(alias))] = true
	}

	scope := make(map[string]string)
	for _, t := range extractTableAliasNames(tree, query).Tables {
		alias := strings.ToLower(strings.TrimSpace(t.AliasName))
		table := strings.TrimSpace(t.TableName)
		if alias == "" || table == "" {
			continue
		}
		if ctes[strings.ToLower(table)] || ctes[alias] {
			continue
		}
		if !strings.Contains(table, ".") {
			table = fmt.Sprint(pr.DefaultSchemaName(), table)
		}
		addAlias(scope, alias, table)
		// An unaliased schema-qualified table can also be referred to by its
		// bare name: "from public.inv" makes "inv.col" legal.
		if alias == strings.ToLower(strings.TrimSpace(t.TableName)) {
			if i := strings.LastIndex(alias, "."); i >= 0 {
				addAlias(scope, alias[i+1:], table)
			}
		}
	}
	return scope, nil
}

// addAlias records alias -> table, mapping to "" when one alias names two
// different tables so the caller fails loudly instead of guessing.
func addAlias(scope map[string]string, alias string, table string) {
	if alias == "" {
		return
	}
	if existing, seen := scope[alias]; seen && existing != table {
		scope[alias] = ""
		return
	}
	scope[alias] = table
}
