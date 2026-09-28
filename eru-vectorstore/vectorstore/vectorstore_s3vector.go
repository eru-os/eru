package vectorstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3vectors"
	s3vdoc "github.com/aws/aws-sdk-go-v2/service/s3vectors/document"
	"github.com/aws/aws-sdk-go-v2/service/s3vectors/types"
	s3vt "github.com/aws/aws-sdk-go-v2/service/s3vectors/types"
	"github.com/aws/smithy-go"
	models "github.com/eru-os/eru/eru-ai/models"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

const (
	AuthTypeSecret = "SECRET"
	AuthTypeIAM    = "IAM"
)

type S3VectorStore struct {
	VectorStore
	Region         string `json:"region" eru:"required"`
	BucketName     string `json:"bucket_name" eru:"required"`
	Authentication string `json:"authentication" eru:"required"`
	Key            string `json:"key"`
	Secret         string `json:"secret"`
	session        *s3vectors.Client
	Index          S3VectorIndex `json:"index"`
}

type S3VectorIndex struct {
	Dimension             int                         `json:"dimension"`
	Metric                string                      `json:"metric"`
	DataType              string                      `json:"data_type"`
	MetadataConfiguration *s3vt.MetadataConfiguration `json:"metadata_configuration,omitempty"`
}

func (svs *S3VectorStore) Init(ctx context.Context) (err error) {
	logs.WithContext(ctx).Debug("Init - Start")
	awsConf, awsConfErr := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(svs.Region),
	)
	if awsConfErr != nil {
		err = awsConfErr
		logs.WithContext(ctx).Error(err.Error())
		return
	}

	if svs.Authentication == AuthTypeSecret {
		awsConf.Credentials = credentials.NewStaticCredentialsProvider(
			svs.Key,
			svs.Secret,
			"", // a token will be created when the session is used.
		)
	} else if svs.Authentication == AuthTypeIAM {
		logs.WithContext(ctx).Info("connecting AWS S3 with IAM role")
		// do nothing - no new attributes to set in config
	}
	svs.session = s3vectors.NewFromConfig(awsConf)

	return err
}

func (svs *S3VectorStore) SearchVectors(ctx context.Context, vectorRecordsSearch VectorRecordsSearch) (vectorResults VectorResults, err error) {
	logs.WithContext(ctx).Debug("SearchVectors - Start")
	if svs.session == nil {
		logs.WithContext(ctx).Info("creating AWS session")
		err = svs.Init(ctx)
		if err != nil {
			return
		}
	}
	var float32Vector []float32
	if len(vectorRecordsSearch.Vector) == 0 {
		// make embedding based on text in meta data
		var embeddingInputs []models.EmbeddingInput
		strSearch := ""
		if vectorRecordsSearch.Inputs != nil {
			for key, value := range vectorRecordsSearch.Inputs {
				if key == "##text##" || key == "text" {
					strSearch = value
					break
				}
			}
			embeddingInputs = append(embeddingInputs, models.EmbeddingInput{
				Id:   "text",
				Text: strSearch,
			})
		}
		if strSearch != "" && svs.Embed.Model != nil {
			embeddingOutputs, err := svs.Embed.Model.GenerateEmbeddings(ctx, embeddingInputs, svs.Embed.ChunkingConfig, svs.Embed.Dimension)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return vectorResults, logs.Err(ctx, err, "")
			}
			vectorRecordsSearch.Vector = embeddingOutputs[0].Vector
		}
	}
	float32Vector = make([]float32, len(vectorRecordsSearch.Vector))
	for i, v := range vectorRecordsSearch.Vector {
		float32Vector[i] = float32(v)
	}
	if len(float32Vector) == 0 {
		err = fmt.Errorf("query vector is empty - provide 'vector' directly, or pass inputs {\"##text##\": \"<search text>\"} with embed model %q configured on the vectorstore", svs.Embed.ModelName)
		logs.WithContext(ctx).Error(err.Error())
		return vectorResults, err
	}
	filterAnd, filterAndOk := vectorRecordsSearch.Filter["$and"]
	if !filterAndOk {
		existingFilter := vectorRecordsSearch.Filter
		filterAndArray := make([]interface{}, 0)
		if existingFilter != nil {
			filterAndArray = append(filterAndArray, existingFilter)
		}
		filterAndArray = append(filterAndArray, map[string]interface{}{"namespace": map[string]interface{}{"$eq": vectorRecordsSearch.Namespace}})
		vectorRecordsSearch.Filter = map[string]interface{}{"$and": filterAndArray}
	} else {
		filterAndArray, filterAndArrayOk := filterAnd.([]interface{})
		if !filterAndArrayOk {
			filterAndArray = make([]interface{}, 0)
		}
		filterAndArray = append(filterAndArray, map[string]interface{}{"namespace": map[string]interface{}{"$eq": vectorRecordsSearch.Namespace}})
		vectorRecordsSearch.Filter["$and"] = filterAndArray
	}
	results, err := svs.session.QueryVectors(ctx, &s3vectors.QueryVectorsInput{
		VectorBucketName: aws.String(svs.BucketName),
		IndexName:        aws.String(svs.VectorName),
		QueryVector:      &types.VectorDataMemberFloat32{Value: float32Vector},
		TopK:             aws.Int32(int32(vectorRecordsSearch.TopK)),
		ReturnDistance:   vectorRecordsSearch.ReturnDistance,
		ReturnMetadata:   vectorRecordsSearch.ReturnMetadata,
		Filter:           s3vdoc.NewLazyDocument(vectorRecordsSearch.Filter),
	})
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return vectorResults, logs.Err(ctx, err, "")
	}
	vectorResults.Records = make([]VectorResult, len(results.Vectors))
	for i, result := range results.Vectors {
		// Convert document interface to map if metadata exists
		metadataMap := make(map[string]interface{})
		if result.Metadata != nil {
			// Extract metadata from document interface
			metadataBytes, err := result.Metadata.MarshalSmithyDocument()
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return vectorResults, logs.Err(ctx, err, "")
			}
			err = json.Unmarshal(metadataBytes, &metadataMap)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return vectorResults, logs.Err(ctx, err, "")
			}
		}
		metadataMap["distance"] = result.Distance
		// Convert vector data to []float64 if available
		var values []float64
		if result.Data != nil {
			// Handle different vector data types
			switch v := result.Data.(type) {
			case *types.VectorDataMemberFloat32:
				values = make([]float64, len(v.Value))
				for j, f32 := range v.Value {
					values[j] = float64(f32)
				}
			}
		}
		vectorResults.Records[i] = VectorResult{
			Id:       *result.Key,
			Values:   values,
			Metadata: metadataMap,
		}
	}

	return vectorResults, nil
}

func (svs *S3VectorStore) SaveVectors(ctx context.Context, vectorRecords VectorRecords) (err error) {
	logs.WithContext(ctx).Debug("SaveVectors - Start")
	if svs.session == nil {
		logs.WithContext(ctx).Info("creating AWS session")
		err = svs.Init(ctx)
		if err != nil {
			return
		}
	}
	if svs.Embed.Model != nil {
		var embeddingInputs []models.EmbeddingInput
		for _, vectorRecord := range vectorRecords.Vectors {
			if vectorRecord.Metadata[svs.Embed.Field] == nil {
				logs.WithContext(ctx).Error(fmt.Sprintf("metadata for vector %s is nil", vectorRecord.Id))
				continue
			}
			strText, ok := vectorRecord.Metadata[svs.Embed.Field].(string)
			if !ok {
				logs.WithContext(ctx).Error(fmt.Sprintf("metadata for vector %s is not a string", vectorRecord.Id))
				continue
			}
			embeddingInputs = append(embeddingInputs, models.EmbeddingInput{
				Id:   vectorRecord.Id,
				Text: strText,
			})
		}
		var embeddingOutputs []models.EmbeddingOutput
		embeddingOutputs, err = svs.Embed.Model.GenerateEmbeddings(ctx, embeddingInputs, svs.Embed.ChunkingConfig, svs.Embed.Dimension)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return
		}
		for _, embeddingOutput := range embeddingOutputs {
			for j, v := range vectorRecords.Vectors {
				if v.Id == embeddingOutput.Id {
					vectorRecords.Vectors[j].Values = embeddingOutput.Vector
					break
				}
			}
		}
	}
	var vectors []s3vt.PutInputVector

	for _, vectorRecord := range vectorRecords.Vectors {
		if len(vectorRecord.Values) == 0 {
			err = fmt.Errorf("vector %s has no embedding values - provide 'values' directly, or set metadata field %q (the configured embed field) to the text to embed with embed model %q configured on the vectorstore", vectorRecord.Id, svs.Embed.Field, svs.Embed.ModelName)
			logs.WithContext(ctx).Error(err.Error())
			return err
		}
		// Convert []float64 to []float32 for AWS SDK
		float32Values := make([]float32, len(vectorRecord.Values))
		for i, v := range vectorRecord.Values {
			float32Values[i] = float32(v)
		}
		metadataMap := make(map[string]string)
		for key, value := range vectorRecord.Metadata {
			metadataMap[key] = fmt.Sprintf("%v", value)
		}
		metadataMap["namespace"] = vectorRecords.Namespace
		vectors = append(vectors, s3vt.PutInputVector{
			Key: aws.String(vectorRecord.Id),
			Data: &s3vt.VectorDataMemberFloat32{
				Value: float32Values,
			},
			Metadata: s3vdoc.NewLazyDocument(metadataMap),
		})
	}

	result, err := svs.session.PutVectors(ctx, &s3vectors.PutVectorsInput{
		VectorBucketName: aws.String(svs.BucketName),
		IndexName:        aws.String(svs.VectorName),
		Vectors:          vectors,
	})
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	logs.WithContext(ctx).Info(fmt.Sprint("result = ", result))
	return nil
}

func (svs *S3VectorStore) DeleteVectors(ctx context.Context, vectorRecordsDelete VectorRecordsDelete) (err error) {
	logs.WithContext(ctx).Debug("DeleteVectors - Start")
	if svs.session == nil {
		logs.WithContext(ctx).Info("creating AWS session")
		err = svs.Init(ctx)
		if err != nil {
			return
		}
	}

	_, err = svs.session.DeleteVectors(ctx, &s3vectors.DeleteVectorsInput{
		VectorBucketName: aws.String(svs.BucketName),
		IndexName:        aws.String(svs.VectorName),
		Keys:             vectorRecordsDelete.Ids,
	})
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	return nil
}
func (svs *S3VectorStore) EditIndex(ctx context.Context, cloneVectorStore VectorStoreI) (err error) {
	return svs.CreateIndex(ctx, cloneVectorStore)
}

func (svs *S3VectorStore) CreateIndex(ctx context.Context, cloneVectorStore VectorStoreI) (err error) {
	logs.WithContext(ctx).Debug("CreateIndex - Start")
	svsClone, svsCloneOk := cloneVectorStore.(*S3VectorStore)
	if !svsCloneOk {
		return logs.Err(ctx, fmt.Errorf("invalid vector store type"), "")
	}
	if svs.session == nil {
		logs.WithContext(ctx).Info("creating AWS session")

		err = svsClone.Init(ctx)
		if err != nil {
			return
		}
		svs.session = svsClone.session
	}

	be := false

	be, err = cloneVectorStore.CheckRemoteStoreExists(ctx)
	if err != nil {

		return
	}

	logs.WithContext(ctx).Info(fmt.Sprint("bucket exists = ", be))

	if !be {

		_, err = svs.session.CreateVectorBucket(ctx, &s3vectors.CreateVectorBucketInput{
			VectorBucketName: aws.String(svsClone.BucketName),
			/* EncryptionConfiguration: &s3vt.EncryptionConfiguration{
				SseType:   aws.String("aws:kms"),
				KmsKeyArn: aws.String("arn:aws:kms:us-east-1:123456789012:key/abcd-1234-efgh-5678"),
			}, */
		})
		if err != nil {
			logs.WithContext(ctx).Info(err.Error())
			err = errors.New("error while creating new AWS bucket")
			return
		}
	} else {
		logs.WithContext(ctx).Info("skipping bucket creation in AWS as it already exists")
	}

	createIndexInput := &s3vectors.CreateIndexInput{
		VectorBucketName: aws.String(svsClone.BucketName),
		IndexName:        aws.String(svsClone.VectorName),
		Dimension:        aws.Int32(int32(svsClone.Index.Dimension)),
		DistanceMetric:   s3vt.DistanceMetric(svsClone.Index.Metric),
		DataType:         s3vt.DataType(svsClone.Index.DataType),
	}

	// Only set MetadataConfiguration if it's provided and not empty
	if svsClone.Index.MetadataConfiguration != nil {
		// Check if the NonFilterableMetadataKeys array has valid content
		if len(svsClone.Index.MetadataConfiguration.NonFilterableMetadataKeys) > 0 {
			createIndexInput.MetadataConfiguration = svsClone.Index.MetadataConfiguration
		}
	}

	resp, err := svs.session.CreateIndex(ctx, createIndexInput)
	if err != nil {
		logs.WithContext(ctx).Info(err.Error())
		err = errors.New("error while creating new AWS vector index")
		return
	}
	logs.WithContext(ctx).Info(fmt.Sprint("response = ", resp))

	return nil
}

func (svs *S3VectorStore) DeleteIndex(ctx context.Context, indexName string) (err error) {
	logs.WithContext(ctx).Debug("DeleteIndex - Start")
	if svs.session == nil {
		logs.WithContext(ctx).Info("creating AWS session")
		err = svs.Init(ctx)
		if err != nil {
			return
		}
	}

	be := false
	be, err = svs.CheckRemoteStoreExists(ctx)
	if err != nil {
		return
	}

	logs.WithContext(ctx).Info(fmt.Sprint("bucket exists = ", be))

	if be {
		deleteIndexOutput, deleteIndexOutputErr := svs.session.DeleteIndex(ctx, &s3vectors.DeleteIndexInput{
			VectorBucketName: aws.String(svs.BucketName),
			IndexName:        aws.String(svs.VectorName),
		})
		if deleteIndexOutputErr != nil {
			logs.WithContext(ctx).Info(deleteIndexOutputErr.Error())
			err = errors.New("error while deleting AWS vector index")
		}
		logs.WithContext(ctx).Info(fmt.Sprint("deleteIndexOutput = ", deleteIndexOutput))
		deleteVectorBucket, deleteVectorBucketErr := svs.session.DeleteVectorBucket(ctx, &s3vectors.DeleteVectorBucketInput{
			VectorBucketName: aws.String(svs.BucketName),
		})
		if deleteVectorBucketErr != nil {
			logs.WithContext(ctx).Info(deleteVectorBucketErr.Error())
			if deleteIndexOutputErr != nil {
				err = errors.New("error while deleting AWS vector index and bucket")
			} else {
				err = errors.New("error while deleting AWS vector bucket")
			}
			return
		}
		logs.WithContext(ctx).Info(fmt.Sprint("deleteVectorBucket = ", deleteVectorBucket))
	}
	return err
}

func (svs *S3VectorStore) GetStats(ctx context.Context) (VectorStats, error) {
	logs.WithContext(ctx).Debug("GetStats - Start")

	return VectorStats{
		//IndexName: svs.IndexName,
	}, nil
}

func (svs *S3VectorStore) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	err := json.Unmarshal(*rj, &svs)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}

func (svs *S3VectorStore) GetAttribute(ctx context.Context, attributeName string) string {
	switch attributeName {
	case "vector_name":
		return svs.VectorName
	case "vector_type":
		return svs.VectorType
	case "bucket_name":
		return svs.BucketName
	case "region":
		return svs.Region
	case "authentication":
		return svs.Authentication
	case "key":
		return svs.Key
	case "secret":
		return svs.Secret
	default:
		return ""
	}
}

func (svs *S3VectorStore) UpdateVectorStore(ctx context.Context, updatedVectorStore VectorStoreI) error {
	logs.WithContext(ctx).Debug("S3VectorStore UpdateVectorStore - Start")
	updatedS3VectorStore, ok := updatedVectorStore.(*S3VectorStore)
	if !ok {
		return logs.Err(ctx, fmt.Errorf("invalid vector store type"), "")
	}
	svs.BucketName = updatedS3VectorStore.BucketName
	svs.Region = updatedS3VectorStore.Region
	svs.Authentication = updatedS3VectorStore.Authentication
	svs.Key = updatedS3VectorStore.Key
	svs.Secret = updatedS3VectorStore.Secret
	svs.Index = updatedS3VectorStore.Index
	return nil
}

func (svs *S3VectorStore) GetBytes(ctx context.Context) ([]byte, error) {
	logs.WithContext(ctx).Debug("S3VectorStore GetBytes - Start")
	vectorStoreJson, err := json.Marshal(svs)
	if err != nil {
		err = logs.Err(ctx, err, "")
		return nil, err
	}
	return vectorStoreJson, nil
}

func (svs *S3VectorStore) BytesToVectorStore(ctx context.Context, vectorStoreObjJson []byte) (VectorStoreI, error) {
	logs.WithContext(ctx).Debug("S3VectorStore BytesToVectorStore - Start")
	iCloneI := reflect.New(reflect.TypeOf(svs))
	vectorStoreObjCloneErr := json.Unmarshal(vectorStoreObjJson, iCloneI.Interface())
	if vectorStoreObjCloneErr != nil {
		err := logs.Err(ctx, vectorStoreObjCloneErr, "error while cloning vectorStoreObj(unmarshal)")
		return nil, err
	}
	return iCloneI.Elem().Interface().(VectorStoreI), nil
}

func (svs *S3VectorStore) SyncIndexDefinition(ctx context.Context, cloneVectorStore VectorStoreI) error {
	logs.WithContext(ctx).Debug("S3VectorStore SyncIndexDefinition - Start")
	// S3 doesn't have index definitions to sync like Pinecone
	// This method is kept for interface compliance
	return nil
}
func (svs *S3VectorStore) CheckRemoteStoreExists(ctx context.Context) (exists bool, err error) {

	if svs.session == nil {
		logs.WithContext(ctx).Info("creating AWS session")
		err = svs.Init(ctx)
		if err != nil {
			return
		}
	}
	logs.WithContext(ctx).Info(svs.BucketName)
	_, err = svs.session.GetVectorBucket(ctx, &s3vectors.GetVectorBucketInput{
		VectorBucketName: aws.String(svs.BucketName),
	})

	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) {
			switch apiErr.ErrorCode() {
			case "NotFound", "NoSuchBucket", "NotFoundException":
				return false, nil // Bucket does not exist
			}
		}
		logs.WithContext(ctx).Info(err.Error())
		err = errors.New(fmt.Sprint("error occurred while checking bucket : ", svs.BucketName))
		return false, err
	}
	return true, err // Bucket exists
}
func (svs *S3VectorStore) ListVectors(ctx context.Context, vectorRecordsList VectorRecordsList) (vectorResults VectorResults, err error) {
	logs.WithContext(ctx).Debug("ListVectors - Start")

	if svs.session == nil {
		logs.WithContext(ctx).Info("creating AWS session")
		err = svs.Init(ctx)
		if err != nil {
			return
		}
	}
	results, err := svs.session.GetVectors(ctx, &s3vectors.GetVectorsInput{
		VectorBucketName: aws.String(svs.BucketName),
		IndexName:        aws.String(svs.VectorName),
		Keys:             vectorRecordsList.Ids,
		ReturnMetadata:   vectorRecordsList.ReturnMetadata,
		ReturnData:       vectorRecordsList.ReturnVector,
	})
	if err != nil {
		return vectorResults, logs.Err(ctx, err, "")
	}
	logs.WithContext(ctx).Info(fmt.Sprint("getOut = ", results))
	vectorResults.Records = make([]VectorResult, len(results.Vectors))
	for i, result := range results.Vectors {
		// Convert document interface to map if metadata exists
		metadataMap := make(map[string]interface{})
		if result.Metadata != nil {
			// Extract metadata from document interface
			metadataBytes, err := result.Metadata.MarshalSmithyDocument()
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return vectorResults, logs.Err(ctx, err, "")
			}
			err = json.Unmarshal(metadataBytes, &metadataMap)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return vectorResults, logs.Err(ctx, err, "")
			}
		}

		// Convert vector data to []float64 if available
		var values []float64
		if result.Data != nil {
			// Handle different vector data types
			switch v := result.Data.(type) {
			case *types.VectorDataMemberFloat32:
				values = make([]float64, len(v.Value))
				for j, f32 := range v.Value {
					values[j] = float64(f32)
				}
			}
		}

		vectorResults.Records[i] = VectorResult{
			Id:       *result.Key,
			Values:   values,
			Metadata: metadataMap,
		}
	}
	return vectorResults, nil
}
func (svs *S3VectorStore) SetAttribute(ctx context.Context, attributeName string, attributeValue string) error {
	switch attributeName {
	case "vector_name":
		svs.VectorName = attributeValue
	case "vector_type":
		svs.VectorType = attributeValue
	case "bucket_name":
		svs.BucketName = attributeValue
	case "region":
		svs.Region = attributeValue
	default:
		return fmt.Errorf("invalid attribute name")
	}
	return nil
}
