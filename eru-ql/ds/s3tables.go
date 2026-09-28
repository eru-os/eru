package ds

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3tables"
	"github.com/aws/aws-sdk-go-v2/service/s3tables/types"
	"github.com/aws/smithy-go"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	common_types "github.com/eru-os/eru/eru-ql/common_types"
	"github.com/eru-os/eru/eru-ql/module_model"
)

const (
	AuthTypeSecret = "SECRET"
	AuthTypeIAM    = "IAM"
)

type IcebergMetadata struct {
	FormatVersion   int               `json:"format-version"`
	TableUUID       string            `json:"table-uuid"`
	Location        string            `json:"location"`
	LastUpdatedMS   int64             `json:"last-updated-ms"`
	LastColumnID    int               `json:"last-column-id"`
	Schemas         []IcebergSchema   `json:"schemas"`
	CurrentSchemaID int               `json:"current-schema-id"`
	Properties      map[string]string `json:"properties"`
}

type IcebergSchema struct {
	SchemaID int           `json:"schema-id"`
	Type     string        `json:"type"`
	Fields   []SchemaField `json:"fields"`
}

type SchemaField struct {
	ID       int     `json:"id"`
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	Required bool    `json:"required"`
	Doc      *string `json:"doc,omitempty"`
}

func (ib *IcebergSqlMaker) S3TablesInit(ctx context.Context, s3TablesConfig *module_model.S3TablesConfig) (err error) {
	logs.WithContext(ctx).Debug("S3TablesInit - Start")
	awsConf, awsConfErr := config.LoadDefaultConfig(ctx,
		config.WithRegion(s3TablesConfig.Region),
	)
	if awsConfErr != nil {
		err = logs.Err(ctx, awsConfErr, "error while loading AWS config")
		return
	}

	if s3TablesConfig.Authentication == AuthTypeSecret {
		awsConf.Credentials = credentials.NewStaticCredentialsProvider(
			s3TablesConfig.Key,
			s3TablesConfig.Secret,
			"", // a token will be created when the session is used.
		)
	} else if s3TablesConfig.Authentication == AuthTypeIAM {
		logs.WithContext(ctx).Info("connecting AWS S3tables with IAM role")
		// do nothing - no new attributes to set in config
	}
	s3TablesConfig.Session = s3tables.NewFromConfig(awsConf)
	s3TablesConfig.S3Session = s3.NewFromConfig(awsConf)

	return err

}

func (ib *IcebergSqlMaker) S3TablesCreateConn(ctx context.Context, dataSource *module_model.DataSource) (err error) {
	logs.WithContext(ctx).Debug("S3TablesCreateConn - Start")
	if dataSource.IcebergConfig.S3TablesConfig.BucketName == "" {
		err = logs.Err(ctx, errors.New("S3TablesConfig.BucketName is nil"), "bucket name cannot be blank")
		return err
	}
	if dataSource.IcebergConfig.S3TablesConfig.Session == nil {
		err = ib.S3TablesInit(ctx, &dataSource.IcebergConfig.S3TablesConfig)
		if err != nil {
			return err
		}
	}
	// Step 1. Create a bucket once (infra bootstrap)
	bucketExists := false
	buckets, bucketsErr := dataSource.IcebergConfig.S3TablesConfig.Session.ListTableBuckets(ctx, &s3tables.ListTableBucketsInput{})
	if bucketsErr != nil {
		err = logs.Err(ctx, bucketsErr, "error while listing S3Tables buckets")
		return err
	}
	for _, bucket := range buckets.TableBuckets {
		if *bucket.Name == dataSource.IcebergConfig.S3TablesConfig.BucketName {
			dataSource.IcebergConfig.S3TablesConfig.BucketArn = *bucket.Arn
			bucketExists = true
			break
		}
	}

	if !bucketExists {
		bucketOut, bucketOutErr := dataSource.IcebergConfig.S3TablesConfig.Session.CreateTableBucket(ctx, &s3tables.CreateTableBucketInput{
			Name: aws.String(dataSource.IcebergConfig.S3TablesConfig.BucketName),
		})
		if bucketOutErr != nil {
			err = logs.Err(ctx, bucketOutErr, "error while creating new S3Tables bucket")
			return err
		}
		bucketArn := *bucketOut.Arn
		dataSource.IcebergConfig.S3TablesConfig.BucketArn = bucketArn
	}
	// Step 2. Create a namespace (database)
	namespace := fmt.Sprintf("%s_%s", dataSource.IcebergConfig.TenantId, dataSource.IcebergConfig.Database)

	namespaceExists := false
	namespaces, namespaceErr := dataSource.IcebergConfig.S3TablesConfig.Session.ListNamespaces(ctx, &s3tables.ListNamespacesInput{
		TableBucketARN: aws.String(dataSource.IcebergConfig.S3TablesConfig.BucketArn),
	})
	if namespaceErr != nil {
		err = logs.Err(ctx, namespaceErr, "error while listing S3Tables namespaces")
		return err
	}

	for _, ns := range namespaces.Namespaces {
		// ns is []string, flatten with dot for comparison
		joined := strings.Join(ns.Namespace, ".")
		if joined == namespace {
			namespaceExists = true
			break
		}
	}

	if !namespaceExists {
		namespaceOut, namespaceOutErr := dataSource.IcebergConfig.S3TablesConfig.Session.CreateNamespace(ctx, &s3tables.CreateNamespaceInput{
			TableBucketARN: aws.String(dataSource.IcebergConfig.S3TablesConfig.BucketArn),
			Namespace:      []string{namespace},
		})
		if namespaceOutErr != nil {
			err = logs.Err(ctx, namespaceOutErr, "error while creating new S3Tables namespace")
			return err
		}
		logs.WithContext(ctx).Info(fmt.Sprintf("namespace created successfully with arn: %s", namespaceOut.Namespace))
	}

	if dataSource.SqlEngine == nil {
		err = logs.Err(ctx, errors.New("sql engine is not initialized"), "sql engine is not initialized")
		return err
	}

	err = dataSource.SqlEngine.SetUp(ctx)
	if err != nil {
		return err
	}
	err = dataSource.SqlEngine.Init(ctx)
	if err != nil {
		return err
	}
	return nil
}

func (ib *IcebergSqlMaker) S3TablesSaveTable(ctx context.Context, tableName string, namespace string, tableStructure common_types.TableStructure, isEdit bool, dataSource *module_model.DataSource) (err error) {
	if isEdit {
		// alter table
	} else {
		var columns []types.SchemaField
		for _, col := range tableStructure.NewColumns {
			columns = append(columns, types.SchemaField{
				Name:     aws.String(col.ColName),
				Type:     aws.String(ib.getErutoDBDataTypeMapping(ctx, col.OwnDataType)),
				Required: !col.IsNullable,
			})
		}
		meta := types.TableMetadataMemberIceberg{
			Value: types.IcebergMetadata{
				Schema: &types.IcebergSchema{
					Fields: columns,
				},
			},
		}
		_, err = dataSource.IcebergConfig.S3TablesConfig.Session.CreateTable(ctx, &s3tables.CreateTableInput{
			Format:         types.OpenTableFormatIceberg,
			Name:           aws.String(tableName),
			Namespace:      aws.String(namespace),
			TableBucketARN: aws.String(dataSource.IcebergConfig.S3TablesConfig.BucketArn),
			Metadata:       &meta,
		})
		if err != nil {
			var apiErr smithy.APIError
			if errors.As(err, &apiErr) && apiErr.ErrorCode() == "ConflictException" {
				err = logs.Err(ctx, err, "table already exists")
				return err
			}
			err = logs.Err(ctx, err, "error while creating S3Tables table")
			return err
		}
	}
	return nil
}
func (ib *IcebergSqlMaker) S3TablesDropTable(ctx context.Context, tableName string, namespace string, dataSource *module_model.DataSource) (err error) {
	dropInput := &s3tables.DeleteTableInput{
		TableBucketARN: aws.String(dataSource.IcebergConfig.S3TablesConfig.BucketArn),
		Namespace:      aws.String(namespace),
		Name:           aws.String(tableName),
	}

	_, err = dataSource.IcebergConfig.S3TablesConfig.Session.DeleteTable(ctx, dropInput)
	if err != nil {
		err = logs.Err(ctx, err, "error while deleting S3Tables table")
		return err
	}
	return nil
}
func (ib *IcebergSqlMaker) S3TablesGetTableList(ctx context.Context, namespace string, datasource *module_model.DataSource, tableName string, myself SqlMakerI) (err error) {
	logs.WithContext(ctx).Debug("GetTableList - Start")
	tableList := make(map[string]map[string]common_types.TableColsMetaData)
	colList := make(map[string]common_types.TableColsMetaData)

	if tableName != "" {
		colList, err = ib.S3TablesGetTable(ctx, namespace, datasource, tableName, myself)
		if err != nil {
			return err
		}
		tableList[tableName] = colList
	} else {

		input := &s3tables.ListTablesInput{
			TableBucketARN: aws.String(datasource.IcebergConfig.S3TablesConfig.BucketArn),
			Namespace:      aws.String(namespace),
		}

		out, err := datasource.IcebergConfig.S3TablesConfig.Session.ListTables(ctx, input)
		if err != nil {
			err = logs.Err(ctx, err, "error while listing S3Tables tables")
			return err
		}

		for _, t := range out.Tables {
			colList, err = ib.S3TablesGetTable(ctx, namespace, datasource, *t.Name, myself)
			if err != nil {
				return err
			}
			tableList[*t.Name] = colList
		}
	}
	datasource.OtherTables = tableList
	return nil
}

func (ib *IcebergSqlMaker) S3TablesGetTable(ctx context.Context, namespace string, datasource *module_model.DataSource, tableName string, myself SqlMakerI) (colList map[string]common_types.TableColsMetaData, err error) {
	logs.WithContext(ctx).Debug("GetTableList - Start")
	colList = make(map[string]common_types.TableColsMetaData)

	resp, respErr := datasource.IcebergConfig.S3TablesConfig.Session.GetTable(ctx, &s3tables.GetTableInput{
		TableBucketARN: aws.String(datasource.IcebergConfig.S3TablesConfig.BucketArn),
		Namespace:      aws.String(namespace),
		Name:           aws.String(tableName),
	})
	if respErr != nil {
		err = logs.Err(ctx, respErr, "error while getting S3Tables table")
		return colList, err
	}
	metadataLocation := *resp.MetadataLocation
	metadataLocationSplit := strings.SplitN(strings.TrimPrefix(metadataLocation, "s3://"), "/", 2)
	bucketName := metadataLocationSplit[0]
	objectKey := strings.Join(metadataLocationSplit[1:], "/")
	obj, err := datasource.IcebergConfig.S3TablesConfig.S3Session.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		err = logs.Err(ctx, err, "error while getting S3Tables table")
		return colList, err
	}
	defer obj.Body.Close()

	body, err := io.ReadAll(obj.Body)
	if err != nil {
		err = logs.Err(ctx, err, "error while reading S3Tables table")
		return colList, err
	}

	var meta IcebergMetadata
	if err := json.Unmarshal(body, &meta); err != nil {
		err = logs.Err(ctx, err, "error while unmarshalling S3Tables table")
		return colList, err
	}
	for _, schema := range meta.Schemas {
		for _, field := range schema.Fields {
			colList[field.Name] = common_types.TableColsMetaData{
				ColName:     field.Name,
				DataType:    field.Type,
				IsNullable:  !field.Required,
				OwnDataType: ib.getDataTypeMapping(ctx, field.Type),
			}
		}
	}
	return colList, nil
}
