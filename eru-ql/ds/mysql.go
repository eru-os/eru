package ds

import (
	"context"
	"errors"
	"fmt"
	"strings"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/eru-os/eru/eru-ql/module_model"
  "github.com/eru-os/eru/eru-ql/derived"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

type MysqlSqlMaker struct {
	SqlMaker
}

func (mr *MysqlSqlMaker) MakeUpsertQuery(ctx context.Context, tableName string, insertCols []string, colsPlaceholder string, conflictCols []string, updateCols []string, action string, returningStr string) (string, error) {
	logs.WithContext(ctx).Debug("MakeUpsertQuery - Start (mysql)")
	if len(conflictCols) == 0 {
		return "", errors.New("upsertOn must include at least one column")
	}
	if returningStr != "" {
		logs.WithContext(ctx).Warn("mysql upsert does not support RETURNING - clause will be dropped")
	}
	conflictSet := make(map[string]bool)
	for _, c := range conflictCols {
		conflictSet[strings.TrimSpace(c)] = true
	}
	base := fmt.Sprint("insert into ", tableName, " (", strings.Join(insertCols, ","), ") values ", colsPlaceholder)
	if action == "nothing" {
		firstCol := strings.TrimSpace(conflictCols[0])
		return fmt.Sprint(base, " on duplicate key update ", firstCol, " = ", firstCol), nil
	}
	sets := buildUpsertSets(insertCols, conflictSet, updateCols, func(col string) string {
		return fmt.Sprint(col, " = values(", col, ")")
	})
	if len(sets) == 0 {
		firstCol := strings.TrimSpace(conflictCols[0])
		return fmt.Sprint(base, " on duplicate key update ", firstCol, " = ", firstCol), nil
	}
	return fmt.Sprint(base, " on duplicate key update ", strings.Join(sets, ",")), nil
}

func (mr *MysqlSqlMaker) GetTableMetaDataSQL(ctx context.Context, tableName string) string {
	return strings.Replace(mysqlTableMetaDataSQL, "$$tableCondition$$", fmt.Sprint("and c.table_name = '", tableName, "'"), 1)
}

func dialMysql(ctx context.Context, cfg module_model.DbConfig) (*sqlx.DB, error) {
	connString := fmt.Sprint(cfg.User, ":", cfg.Password, "@tcp(", cfg.Host, ":", cfg.Port, ")/", cfg.DefaultSchema)
	db, err := sqlx.Open("mysql", connString)
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

func (mr *MysqlSqlMaker) CreateConn(ctx context.Context, dataSource *module_model.DataSource) error {
	logs.WithContext(ctx).Debug("CreateConn - Start")
	db, err := dialMysql(ctx, dataSource.DbConfig)
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
		if rerr := mr.ConnectReadReplica(ctx, dataSource, i); rerr != nil {
			logs.WithContext(ctx).Error(fmt.Sprint("read replica ", dataSource.ReadDbConfigs[i].Name, " connect failed: ", rerr.Error()))
		}
	}
	return nil
}

func (mr *MysqlSqlMaker) ConnectReadReplica(ctx context.Context, dataSource *module_model.DataSource, idx int) error {
	if idx < 0 || idx >= len(dataSource.ReadDbConfigs) {
		return errors.New("read replica index out of range")
	}
	replica := dataSource.ReadDbConfigs[idx]
	cfg := replica.DbConfig
	if replica.ResolvedDbConfig.Host != "" {
		cfg = replica.ResolvedDbConfig
	}
	db, err := dialMysql(ctx, cfg)
	if err != nil {
		replica.ConStatus = false
		replica.Con = nil
		return err
	}
	replica.Con = db
	replica.ConStatus = true
	return nil
}
func (mr *MysqlSqlMaker) CheckMe(ctx context.Context) {
	logs.WithContext(ctx).Info("I am MysqlSqlMaker changed  removed")
	mr.ChildChange = "changed by MysqlSqlMaker"
}

func (pr *MysqlSqlMaker) AddLimitSkipClause(ctx context.Context, query string, limit int, skip int, globalLimit int) (newQuery string) {
	logs.WithContext(ctx).Debug("AddLimitSkipClause - Start")
	strSkip := ""
	if skip > 0 {
		strSkip = fmt.Sprint(skip, " , ")
	}
	if limit > 0 {
		newQuery = fmt.Sprint(query, " limit ", strSkip, limit)
	} else {
		newQuery = fmt.Sprint(query, " limit ", strSkip, globalLimit)
	}
	return newQuery
}

func (mr *MysqlSqlMaker) executeQuery(ctx context.Context, query string, datasource *module_model.DataSource) (res map[string]interface{}, err error) {
	logs.WithContext(ctx).Debug("executeQuery - Start")
	//rows, e := datasource.Con.Query(query)
	rows, e := datasource.Con.Queryx(query)
	if e != nil {
		logs.WithContext(ctx).Error(e.Error())
	}
	defer rows.Close()
	mapping := make(map[string]interface{})
	for rows.Next() {
		e = rows.MapScan(mapping)
		if e != nil {
			logs.WithContext(ctx).Error(e.Error())
		}
		colsType, ee := rows.ColumnTypes()
		if ee != nil {
			logs.WithContext(ctx).Error(e.Error())
		}
		_ = colsType
	}
	return nil, nil
}
func (mr *MysqlSqlMaker) getDataTypeMapping(ctx context.Context, dataType string) string {
	logs.WithContext(ctx).Debug("getDataTypeMapping - Start")
	fmt.Printf("getDataTypeMapping - Start dataType = %s\n", dataType)
	if mysqlDataTypeMapping[dataType] == "" {
		return "NotSupported"
	} else {
		fmt.Printf("mysqlDataTypeMapping[dataType] = %s\n", mysqlDataTypeMapping[dataType])
		return mysqlDataTypeMapping[dataType]
	}
}

func (mr *MysqlSqlMaker) getErutoDBDataTypeMapping(ctx context.Context, dataType string) string {
	logs.WithContext(ctx).Debug("getErutoDBDataTypeMapping - Start")
	return dataType
}

var mysqlTableMetaDataSQL = `SELECT 
    c.TABLE_SCHEMA AS tblschema,
    c.TABLE_NAME AS tblname,
    c.COLUMN_NAME AS colname,
    c.DATA_TYPE AS datatype,
    '' AS owndatatype,
    CAST(CASE WHEN pk.CONSTRAINT_TYPE IS NOT NULL THEN 'true' ELSE 'false' END AS CHAR) AS primarykey,
    CAST(CASE WHEN uq.CONSTRAINT_TYPE IS NOT NULL THEN 'true' ELSE 'false' END AS CHAR) AS isunique,
    COALESCE(pk.CONSTRAINT_NAME, '') AS pkconstraintname,
    COALESCE(uq.CONSTRAINT_NAME, '') AS uqconstraintname,
    CAST(CASE WHEN c.IS_NULLABLE = 'YES' THEN 'true' ELSE 'false' END AS CHAR) AS isnullable,
    c.ORDINAL_POSITION AS colposition,
    CAST(COALESCE(c.COLUMN_DEFAULT, '') AS CHAR) AS defaultvalue,
    CAST(CASE WHEN c.EXTRA LIKE '%auto_increment%' THEN 'true' ELSE 'false' END AS CHAR) AS autoincrement,
    COALESCE(c.CHARACTER_MAXIMUM_LENGTH, -1) AS charmaxlength,
    CONCAT(COALESCE(c.NUMERIC_PRECISION, 0), ',', COALESCE(c.NUMERIC_SCALE, 0)) AS numericprecision,
    COALESCE(c.NUMERIC_SCALE, 0) AS numericscale,
    COALESCE(c.DATETIME_PRECISION, 0) AS datetimeprecision,
    COALESCE(fk.CONSTRAINT_NAME, '') AS fkconstraintname,
    COALESCE(fk.DELETE_RULE, '') AS fkdeleterule,
    COALESCE(fk.REFERENCED_TABLE_SCHEMA, '') AS fktblschema,
    COALESCE(fk.REFERENCED_TABLE_NAME, '') AS fktblname,
    COALESCE(fk.REFERENCED_COLUMN_NAME, '') AS fkcolname
FROM 
    information_schema.COLUMNS c
LEFT JOIN (
    SELECT 
        kcu.TABLE_SCHEMA, 
        kcu.TABLE_NAME, 
        kcu.COLUMN_NAME, 
        tc.CONSTRAINT_NAME, 
        tc.CONSTRAINT_TYPE
    FROM information_schema.TABLE_CONSTRAINTS tc
    JOIN information_schema.KEY_COLUMN_USAGE kcu 
        ON tc.CONSTRAINT_NAME = kcu.CONSTRAINT_NAME 
        AND tc.TABLE_SCHEMA = kcu.TABLE_SCHEMA 
        AND tc.TABLE_NAME = kcu.TABLE_NAME
    WHERE tc.CONSTRAINT_TYPE = 'PRIMARY KEY'
) pk 
    ON pk.TABLE_SCHEMA = c.TABLE_SCHEMA 
    AND pk.TABLE_NAME = c.TABLE_NAME 
    AND pk.COLUMN_NAME = c.COLUMN_NAME
LEFT JOIN (
    SELECT 
        kcu.TABLE_SCHEMA, 
        kcu.TABLE_NAME, 
        kcu.COLUMN_NAME, 
        tc.CONSTRAINT_NAME, 
        tc.CONSTRAINT_TYPE
    FROM information_schema.TABLE_CONSTRAINTS tc
    JOIN information_schema.KEY_COLUMN_USAGE kcu 
        ON tc.CONSTRAINT_NAME = kcu.CONSTRAINT_NAME 
        AND tc.TABLE_SCHEMA = kcu.TABLE_SCHEMA 
        AND tc.TABLE_NAME = kcu.TABLE_NAME
    WHERE tc.CONSTRAINT_TYPE = 'UNIQUE'
) uq 
    ON uq.TABLE_SCHEMA = c.TABLE_SCHEMA 
    AND uq.TABLE_NAME = c.TABLE_NAME 
    AND uq.COLUMN_NAME = c.COLUMN_NAME
LEFT JOIN (
    SELECT 
        kcu.TABLE_SCHEMA, 
        kcu.TABLE_NAME, 
        kcu.COLUMN_NAME, 
        kcu.CONSTRAINT_NAME, 
        kcu.REFERENCED_TABLE_SCHEMA, 
        kcu.REFERENCED_TABLE_NAME, 
        kcu.REFERENCED_COLUMN_NAME, 
        rc.DELETE_RULE
    FROM information_schema.KEY_COLUMN_USAGE kcu
    JOIN information_schema.REFERENTIAL_CONSTRAINTS rc 
        ON kcu.CONSTRAINT_NAME = rc.CONSTRAINT_NAME 
        AND kcu.CONSTRAINT_SCHEMA = rc.CONSTRAINT_SCHEMA
    WHERE kcu.REFERENCED_TABLE_NAME IS NOT NULL
) fk 
    ON fk.TABLE_SCHEMA = c.TABLE_SCHEMA 
    AND fk.TABLE_NAME = c.TABLE_NAME 
    AND fk.COLUMN_NAME = c.COLUMN_NAME
WHERE 
    c.TABLE_SCHEMA NOT IN ('information_schema', 'mysql', 'performance_schema', 'sys') $$tableCondition$$
ORDER BY c.TABLE_SCHEMA, c.TABLE_NAME, c.ORDINAL_POSITION;
`

var mysqlDataTypeMapping = map[string]string{
	"tinyint":          "SmallInteger",
	"smallint":         "SmallInteger",
	"mediumint":        "Integer",
	"int":              "Integer",
	"integer":          "Integer",
	"bigint":           "BigInteger",
	"decimal":          "Decimal",
	"numeric":          "Decimal",
	"float":            "Float",
	"double":           "Float",
	"double precision": "Float",
	"real":             "Integer",
	"bit":              "Integer",
	"BOOL":             "Boolean",
	"boolean":          "Boolean",
	"date":             "Date",
	"datetime":         "DateTime",
	"timestamp":        "DateTimeWithZone",
	"time":             "Time",
	"year":             "Varchar",
	"char":             "Varchar",
	"varchar":          "Varchar",
	"tinytext":         "Char",
	"text":             "Char",
	"mediumtext":       "Varchar",
	"longtext":         "Varchar",
	"json":             "JSON",
}

func (mr *MysqlSqlMaker) GetCalcDialect(ctx context.Context) derived.Dialect {
	return derived.MysqlDialect()
}
