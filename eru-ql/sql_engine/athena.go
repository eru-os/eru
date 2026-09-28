package sqlengine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/athena"
	"github.com/aws/aws-sdk-go-v2/service/athena/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	common_types "github.com/eru-os/eru/eru-ql/common_types"
)

const (
	AuthTypeSecret = "SECRET"
	AuthTypeIAM    = "IAM"
)

type AthenaSQLEngine struct {
	SQLEngine
	session        *athena.Client
	s3session      *s3.Client
	Region         string `json:"region" eru:"required"`
	Authentication string `json:"authentication" eru:"required"`
	Key            string `json:"key"`
	Secret         string `json:"secret"`
	Workgroup      string `json:"workgroup"`
	OutputS3Bucket string `json:"output_s3_bucket" eru:"required"`
}

func (athenaSQLEngine *AthenaSQLEngine) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	err := json.Unmarshal(*rj, &athenaSQLEngine)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}
func (athenaSQLEngine *AthenaSQLEngine) Init(ctx context.Context) (err error) {
	logs.WithContext(ctx).Debug("Init - Start")
	awsConf, awsConfErr := config.LoadDefaultConfig(ctx,
		config.WithRegion(athenaSQLEngine.Region),
	)
	if awsConfErr != nil {
		err = awsConfErr
		logs.WithContext(ctx).Error(err.Error())
		return
	}

	if athenaSQLEngine.Authentication == AuthTypeSecret {
		awsConf.Credentials = credentials.NewStaticCredentialsProvider(
			athenaSQLEngine.Key,
			athenaSQLEngine.Secret,
			"", // a token will be created when the session is used.
		)
	} else if athenaSQLEngine.Authentication == AuthTypeIAM {
		logs.WithContext(ctx).Info("connecting AWS Athena with IAM role")
		// do nothing - no new attributes to set in config
	}
	athenaSQLEngine.session = athena.NewFromConfig(awsConf)
	athenaSQLEngine.s3session = s3.NewFromConfig(awsConf)
	return err

}

func (athenaSQLEngine *AthenaSQLEngine) SetUp(ctx context.Context) (err error) {
	logs.WithContext(ctx).Debug("SetUp - Start")
	if athenaSQLEngine.session == nil || athenaSQLEngine.s3session == nil {
		err = athenaSQLEngine.Init(ctx)
		if err != nil {
			return err
		}
	}
	if athenaSQLEngine.Workgroup == "" {
		err = logs.Err(ctx, errors.New("workgroup is required"), "workgroup is required")
		return err
	}
	if athenaSQLEngine.OutputS3Bucket == "" {
		err = logs.Err(ctx, errors.New("output_s3_bucket name is required"), "output_s3_bucket name is required")
		return err
	}
	bucketExits := true
	_, err = athenaSQLEngine.s3session.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(athenaSQLEngine.OutputS3Bucket),
	})

	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) {
			switch apiErr.ErrorCode() {
			case "NotFound", "NoSuchBucket":
				bucketExits = false
				err = nil
			default:
				err = logs.Err(ctx, err, "failed to check output s3 bucket")
				return err
			}
		}
	}

	if !bucketExits {
		_, err = athenaSQLEngine.s3session.CreateBucket(ctx, &s3.CreateBucketInput{
			Bucket: aws.String(athenaSQLEngine.OutputS3Bucket),
			CreateBucketConfiguration: &s3types.CreateBucketConfiguration{
				LocationConstraint: s3types.BucketLocationConstraint(athenaSQLEngine.Region),
			},
		})
		if err != nil {
			err = logs.Err(ctx, err, "failed to create output s3 bucket")
			return err
		}
	}

	workgroupExists := true
	_, err = athenaSQLEngine.session.GetWorkGroup(ctx, &athena.GetWorkGroupInput{
		WorkGroup: aws.String(athenaSQLEngine.Workgroup),
	})
	if err != nil {
		if err.Error() != "" && strings.Contains(err.Error(), "not found") {
			workgroupExists = false // workgroup does not exist
		} else {
			err = logs.Err(ctx, err, "failed to get workgroup")
			return err
		}
	}
	if !workgroupExists {
		_, err = athenaSQLEngine.session.CreateWorkGroup(ctx, &athena.CreateWorkGroupInput{
			Name: aws.String(athenaSQLEngine.Workgroup),
			Configuration: &types.WorkGroupConfiguration{
				ResultConfiguration: &types.ResultConfiguration{
					OutputLocation: aws.String(fmt.Sprintf("s3://%s/", athenaSQLEngine.OutputS3Bucket)),
				},
				EnforceWorkGroupConfiguration: aws.Bool(false),
			},
		})
		if err != nil {
			err = logs.Err(ctx, err, "failed to create workgroup")
			return err
		}
	}

	return nil
}
func (athenaSQLEngine *AthenaSQLEngine) ExecuteQuery(ctx context.Context, query string, database string, catalog string) (output []map[string]interface{}, err error) {
	if athenaSQLEngine.session == nil {
		err = athenaSQLEngine.Init(ctx)
		if err != nil {
			return
		}
	}
	start, err := athenaSQLEngine.session.StartQueryExecution(ctx, &athena.StartQueryExecutionInput{
		QueryString: aws.String(query),
		QueryExecutionContext: &types.QueryExecutionContext{
			Catalog:  aws.String(catalog),
			Database: aws.String(database),
		},
		ResultConfiguration: &types.ResultConfiguration{OutputLocation: aws.String(fmt.Sprintf("s3://%s/", athenaSQLEngine.OutputS3Bucket))},
		WorkGroup:           aws.String(athenaSQLEngine.Workgroup),
	})
	if err != nil {
		err = logs.Err(ctx, err, "failed to start query execution")
		return nil, err
	}

	// Poll
	var state string
	for {
		desc, err := athenaSQLEngine.session.GetQueryExecution(ctx, &athena.GetQueryExecutionInput{
			QueryExecutionId: start.QueryExecutionId,
		})
		if err != nil {
			return nil, err
		}
		state = string(desc.QueryExecution.Status.State)
		if state == "SUCCEEDED" {
			break
		}
		if state == "FAILED" || state == "CANCELLED" {
			err = logs.Err(ctx, fmt.Errorf("athena query failed: %s : %s %d", state, *desc.QueryExecution.Status.AthenaError.ErrorMessage, *desc.QueryExecution.Status.AthenaError.ErrorType), "athena query failed")
			return nil, err
		}
		time.Sleep(2 * time.Second)
	}

	// 3. Fetch results
	resOut, err := athenaSQLEngine.session.GetQueryResults(ctx, &athena.GetQueryResultsInput{
		QueryExecutionId: start.QueryExecutionId,
	})
	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprintf("failed to get query results: %v", err))
		return nil, err
	}

	for _, row := range resOut.ResultSet.Rows {
		for _, col := range row.Data {
			logs.WithContext(ctx).Info(fmt.Sprintf("%s\t", aws.ToString(col.VarCharValue)))
		}
	}
	return nil, nil
}

func (athenaSQLEngine *AthenaSQLEngine) MakeCreateTableSQL(ctx context.Context, tableName string, tableObj map[string]common_types.TableColsMetaData) (string, error) {
	logs.WithContext(ctx).Debug("MakeCreateTableSQL - Start")
	var cols []string
	for _, v := range tableObj {
		dt := athenaSQLEngine.getErutoDBDataTypeMapping(ctx, v.OwnDataType)

		//pk := ""
		//uk := ""

		//Athena does not support NOT NULL constraint
		/* nl := ""
		if !v.IsNullable {
			nl = " NOT NULL "
		} */

		switch dt {
		case "Decimal":
			dt = fmt.Sprint(dt, " (", v.NumericPrecision, ")")
			/* case "Varchar":
			dt = fmt.Sprint(dt, " (", v.CharMaxLength, ")") */
		}

		//Athena does not support DEFAULT values
		/* df := v.DefaultValue
		if df != "" {
			df = fmt.Sprint(" default ", df)
		} */
		cols = append(cols, fmt.Sprint(v.ColName, " ", strings.ToUpper(dt)))
	}

	query := fmt.Sprint("create table ", tableName, " (", strings.Join(cols, " , "), " )")
	logs.WithContext(ctx).Info(query)
	return query, nil
}
func (athenaSQLEngine *AthenaSQLEngine) getDataTypeMapping(ctx context.Context, dataType string) string {
	logs.WithContext(ctx).Debug("getDataTypeMapping - Start")
	if athenaDataTypeMapping[strings.ToLower(dataType)] == "" {
		return "NotSupported"
	} else {
		return athenaDataTypeMapping[strings.ToLower(dataType)]
	}
}

func (athenaSQLEngine *AthenaSQLEngine) getErutoDBDataTypeMapping(ctx context.Context, dataType string) string {
	logs.WithContext(ctx).Debug("getErutoDBDataTypeMapping - Start")
	if athenaErutoDBDataTypeMapping[dataType] == "" {
		return "NotSupported"
	} else {
		return athenaErutoDBDataTypeMapping[dataType]
	}
}

var athenaDataTypeMapping = map[string]string{
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
var athenaErutoDBDataTypeMapping = map[string]string{
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

/*
| Iceberg type   | Athena / Trino         | Spark SQL         | Flink SQL         | DuckDB         |
| -------------- | ---------------------- | ----------------- | ----------------- | -------------- |
| `boolean`      | `BOOLEAN`              | `BOOLEAN`         | `BOOLEAN`         | `BOOLEAN`      |
| `int`          | `INT`                  | `INT`             | `INT`             | `INTEGER`      |
| `long`         | `BIGINT`               | `BIGINT`          | `BIGINT`          | `BIGINT`       |
| `float`        | `FLOAT`                | `FLOAT`           | `FLOAT`           | `REAL`         |
| `double`       | `DOUBLE`               | `DOUBLE`          | `DOUBLE`          | `DOUBLE`       |
| `decimal(p,s)` | `DECIMAL(p,s)`         | `DECIMAL(p,s)`    | `DECIMAL(p,s)`    | `DECIMAL(p,s)` |
| `date`         | `DATE`                 | `DATE`            | `DATE`            | `DATE`         |
| `time`         | `TIME`                 | `TIME`            | `TIME`            | `TIME`         |
| `timestamp`    | `TIMESTAMP`            | `TIMESTAMP`       | `TIMESTAMP(3)`    | `TIMESTAMP`    |
| `string`       | `STRING`/`VARCHAR`     | `STRING`          | `STRING`          | `TEXT`         |
| `binary`       | `BINARY`               | `BINARY`          | `BYTES`           | `BLOB`         |
| `uuid`         | `UUID`                 | `STRING` (mapped) | `STRING` (mapped) | `UUID`         |
| `fixed(N)`     | ❌ not exposed directly | `BINARY(N)`       | `BYTES(N)`        | ❌              |
| `list<T>`      | `ARRAY<T>`             | `ARRAY<T>`        | `ARRAY<T>`        | `LIST<T>`      |
| `map<K,V>`     | `MAP<K,V>`             | `MAP<K,V>`        | `MAP<K,V>`        | `MAP<K,V>`     |
| `struct<...>`  | `STRUCT<...>`          | `STRUCT<...>`     | `ROW<...>`        | `STRUCT<...>`  |
*/
