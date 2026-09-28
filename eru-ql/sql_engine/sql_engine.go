package sqlengine

import (
	"context"
	"encoding/json"
	"errors"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	common_types "github.com/eru-os/eru/eru-ql/common_types"
)

type SQLEngineI interface {
	ExecuteQuery(ctx context.Context, query string, database string, catalog string) (output []map[string]interface{}, err error)
	Init(ctx context.Context) (err error)
	MakeCreateTableSQL(ctx context.Context, tableName string, tableObj map[string]common_types.TableColsMetaData) (string, error)
	MakeDropTableSQL(ctx context.Context, tableName string) (string, error)
	MakeFromJson(ctx context.Context, rj *json.RawMessage) error
	SetUp(ctx context.Context) (err error)
}

type SQLEngine struct {
	SqlEngineName string `json:"sql_engine_name"`
	SqlEngineType string `json:"sql_engine_type"`
}

func GetSQLEngine(sqlEngineType string) SQLEngineI {
	switch sqlEngineType {
	case "athena":
		return new(AthenaSQLEngine)
	/* case "trino":
	return new(TrinoSQLEngine) */
	default:
		return new(SQLEngine)
	}
}

func (sqle *SQLEngine) ExecuteQuery(ctx context.Context, query string, database string, catalog string) (output []map[string]interface{}, err error) {
	err = logs.Err(ctx, errors.New("not implemented"), "not implemented")
	return nil, err
}

func (sqle *SQLEngine) SetUp(ctx context.Context) (err error) {
	err = logs.Err(ctx, errors.New("not implemented"), "not implemented")
	return err
}
func (sqle *SQLEngine) Init(ctx context.Context) (err error) {
	err = logs.Err(ctx, errors.New("not implemented"), "not implemented")
	return err
}

func (sqle *SQLEngine) MakeCreateTableSQL(ctx context.Context, tableName string, tableObj map[string]common_types.TableColsMetaData) (string, error) {
	err := logs.Err(ctx, errors.New("not implemented"), "not implemented")
	return "", err
}
func (sqle *SQLEngine) MakeDropTableSQL(ctx context.Context, tableName string) (string, error) {
	err := logs.Err(ctx, errors.New("not implemented"), "not implemented")
	return "", err
}

func (sqle *SQLEngine) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	err := logs.Err(ctx, errors.New("not implemented"), "not implemented")
	return err
}
