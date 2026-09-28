package ds

import (
	"context"
	"errors"
	"fmt"
	"strings"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	common_types "github.com/eru-os/eru/eru-ql/common_types"
	"github.com/eru-os/eru/eru-ql/module_model"
)

const (
	CatalogTypeS3Tables = "s3tables"
)

type IcebergSqlMaker struct {
	SqlMaker
}

func (ib *IcebergSqlMaker) CreateConn(ctx context.Context, dataSource *module_model.DataSource) error {
	logs.WithContext(ctx).Debug("CreateConn - Start")
	// Load a REST catalog (S3 Tables endpoint)

	if dataSource.IcebergConfig.CatalogType != CatalogTypeS3Tables {
		return errors.New("currently only s3tables is supported")
	}
	if dataSource.IcebergConfig.CatalogType == CatalogTypeS3Tables {
		return ib.S3TablesCreateConn(ctx, dataSource)
	}

	return nil
}

func (ib *IcebergSqlMaker) getDataTypeMapping(ctx context.Context, dataType string) string {
	logs.WithContext(ctx).Debug("getDataTypeMapping - Start")
	if icebergDataTypeMapping[strings.ToUpper(dataType)] == "" {
		return "NotSupported"
	} else {
		return icebergDataTypeMapping[strings.ToUpper(dataType)]
	}
}

func (ib *IcebergSqlMaker) getErutoDBDataTypeMapping(ctx context.Context, dataType string) string {
	logs.WithContext(ctx).Debug("getErutoDBDataTypeMapping - Start")
	if icebergErutoDBDataTypeMapping[dataType] == "" {
		return "NotSupported"
	} else {
		return icebergErutoDBDataTypeMapping[dataType]
	}
}
func (ib *IcebergSqlMaker) SaveTable(ctx context.Context, tableName string, tableStructure common_types.TableStructure, isEdit bool, dataSource *module_model.DataSource) (err error) {
	if isEdit {
		// alter table
	} else {
		if dataSource.IcebergConfig.S3TablesConfig.UseForDDL {
			namespace := fmt.Sprintf("%s_%s", dataSource.IcebergConfig.TenantId, dataSource.IcebergConfig.Database)
			return ib.S3TablesSaveTable(ctx, tableName, namespace, tableStructure, isEdit, dataSource)
		}
		if dataSource.SqlEngine == nil {
			err = logs.Err(ctx, errors.New("sql engine is nil"), "sql engine is not initialized")
			return err
		}
		ns := fmt.Sprintf("%s_%s", dataSource.IcebergConfig.TenantId, dataSource.IcebergConfig.Database)
		tn := fmt.Sprintf("%s.%s", ns, tableName)
		query, err := dataSource.SqlEngine.MakeCreateTableSQL(ctx, tn, tableStructure.NewColumns)
		if err != nil {
			return err
		}
		query = fmt.Sprintf("%s TBLPROPERTIES ('table_type' = 'iceberg')", query)
		_, err = ib.ExecutePreparedQuery(ctx, query, dataSource)
		if err != nil {
			return err
		}
	}
	return nil
}
func (ib *IcebergSqlMaker) DropTable(ctx context.Context, tableName string, dataSource *module_model.DataSource) (err error) {
	if dataSource.IcebergConfig.S3TablesConfig.UseForDDL {
		namespace := fmt.Sprintf("%s_%s", dataSource.IcebergConfig.TenantId, dataSource.IcebergConfig.Database)
		return ib.S3TablesDropTable(ctx, tableName, namespace, dataSource)
	}
	if dataSource.SqlEngine == nil {
		err = logs.Err(ctx, errors.New("sql engine is nil"), "sql engine is not initialized")
		return err
	}
	tn := fmt.Sprintf("%s_%s.%s", dataSource.IcebergConfig.TenantId, dataSource.IcebergConfig.Database, tableName)
	query, err := dataSource.SqlEngine.MakeDropTableSQL(ctx, tn)
	if err != nil {
		return err
	}
	_, err = ib.ExecutePreparedQuery(ctx, query, dataSource)
	if err != nil {
		return err
	}
	return nil
}
func (ib *IcebergSqlMaker) EditTable(ctx context.Context, tableName string, tableStructure common_types.TableStructure) (err error) {

	return nil
}
func (ib *IcebergSqlMaker) MakeCreateTableSQL(ctx context.Context, tableName string, tableObj map[string]common_types.TableColsMetaData) (string, error) {
	logs.WithContext(ctx).Debug("MakeCreateTableSQL - Start")
	err := logs.Err(ctx, errors.New("not implemented"), "not implemented")
	return "", err
}

func (ib *IcebergSqlMaker) ExecutePreparedQuery(ctx context.Context, query string, datasource *module_model.DataSource) (res map[string]interface{}, err error) {
	logs.WithContext(ctx).Debug("ExecutePreparedQuery - Start")
	logs.WithContext(ctx).Info(query)
	dn := fmt.Sprintf("%s_%s", datasource.IcebergConfig.TenantId, datasource.IcebergConfig.Database)
	catalog := fmt.Sprintf("s3tablescatalog/%s", datasource.IcebergConfig.S3TablesConfig.BucketName)
	output, err := datasource.SqlEngine.ExecuteQuery(ctx, query, dn, catalog)
	if err != nil {
		return nil, err
	}
	logs.WithContext(ctx).Info(fmt.Sprint(output))
	if len(output) == 0 {
		return nil, nil
	}
	return output[0], nil
}

func (ib *IcebergSqlMaker) GetTableList(ctx context.Context, datasource *module_model.DataSource, tableName string, myself SqlMakerI) (err error) {
	logs.WithContext(ctx).Debug("GetTableList - Start")
	tableList := make(map[string]map[string]common_types.TableColsMetaData)
	if datasource.IcebergConfig.S3TablesConfig.UseForDDL {
		namespace := fmt.Sprintf("%s_%s", datasource.IcebergConfig.TenantId, datasource.IcebergConfig.Database)
		return ib.S3TablesGetTableList(ctx, namespace, datasource, tableName, myself)
	}
	datasource.OtherTables = tableList
	return nil
}

var icebergDataTypeMapping = map[string]string{
	"BOOLEAN":   "Boolean",
	"INT":       "Integer",
	"BIGINT":    "BigInteger",
	"FLOAT":     "Float",
	"DOUBLE":    "Float",
	"DECIMAL":   "Decimal",
	"DATE":      "Date",
	"TIME":      "Time",
	"TIMESTAMP": "TimeWithZone",
	"STRING":    "Varchar",
	"VARCHAR":   "Varchar",
	"ARRAY":     "Array",
	"MAP":       "Map",
	"STRUCT":    "Struct",
	"BINARY":    "Binary",
	"UUID":      "UUID",
}
var icebergErutoDBDataTypeMapping = map[string]string{
	"Boolean":      "BOOLEAN",
	"Integer":      "INT",
	"BigInteger":   "BIGINT",
	"Float":        "FLOAT",
	"Decimal":      "DECIMAL",
	"Date":         "DATE",
	"Time":         "TIME",
	"TimeWithZone": "TIMESTAMP",
	"Varchar":      "STRING",
	"Array":        "ARRAY",
	"Map":          "MAP",
	"Struct":       "STRUCT",
	"Binary":       "BINARY",
	"UUID":         "UUID",
}
